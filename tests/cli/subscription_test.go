package cli_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/polyapi/polyglot/src/exitcode"
)

type subscriptionRecord struct {
	ID      string
	Name    string
	Context string
	Body    map[string]any
}

type subscriptionFake struct {
	mu        sync.Mutex
	byID      map[string]subscriptionRecord
	functions map[string]map[string]any
	variables map[string]map[string]any
	posts     []map[string]any
	patches   []map[string]any
	recovers  []map[string]any
	deleted   []string
	otps      []string
}

func newSubscriptionFake() *subscriptionFake {
	return &subscriptionFake{
		byID:      map[string]subscriptionRecord{},
		functions: map[string]map[string]any{},
		variables: map[string]map[string]any{},
	}
}

func (f *subscriptionFake) put(id, context, name string, extra map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body := map[string]any{
		"id": id, "name": name, "context": context,
		"enabled": true, "type": "CUSTOM",
		"websocketUrl": "wss://example.com/graphql",
		"query":        "subscription { x }",
		"functionId":   "fn-1",
	}
	for k, v := range extra {
		body[k] = v
	}
	f.byID[id] = subscriptionRecord{ID: id, Name: name, Context: context, Body: body}
}

func (f *subscriptionFake) putFn(id, context, name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.functions[id] = map[string]any{"id": id, "name": name, "context": context}
}

func (f *subscriptionFake) listItem(rec subscriptionRecord) map[string]any {
	return map[string]any{
		"id": rec.ID, "name": rec.Name, "context": rec.Context,
		"enabled": rec.Body["enabled"],
	}
}

func (f *subscriptionFake) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := strings.Trim(r.URL.Path, "/")
		parts := strings.Split(path, "/")
		raw, _ := io.ReadAll(r.Body)
		otp := r.Header.Get("x-otp")
		f.mu.Lock()
		defer f.mu.Unlock()
		if otp != "" {
			f.otps = append(f.otps, otp)
		}
		switch {
		case len(parts) >= 2 && parts[0] == "functions" && parts[1] == "server" && r.Method == http.MethodGet:
			items := make([]any, 0, len(f.functions))
			for _, fn := range f.functions {
				items = append(items, fn)
			}
			_ = json.NewEncoder(w).Encode(items)
		case len(parts) >= 1 && parts[0] == "variables" && r.Method == http.MethodGet && len(parts) == 1:
			items := make([]any, 0, len(f.variables))
			for _, v := range f.variables {
				items = append(items, v)
			}
			_ = json.NewEncoder(w).Encode(items)
		case len(parts) >= 2 && parts[0] == "subscriptions" && parts[1] == "graphql":
			f.handleGraphQL(w, r, parts, raw)
		default:
			http.NotFound(w, r)
		}
	})
}

func (f *subscriptionFake) handleGraphQL(w http.ResponseWriter, r *http.Request, parts []string, raw []byte) {
	switch {
	case r.Method == http.MethodGet && len(parts) == 2:
		items := make([]any, 0, len(f.byID))
		for _, rec := range f.byID {
			items = append(items, f.listItem(rec))
		}
		_ = json.NewEncoder(w).Encode(items)
	case r.Method == http.MethodPost && len(parts) == 2:
		var payload map[string]any
		_ = json.Unmarshal(raw, &payload)
		f.posts = append(f.posts, payload)
		id := "sub-1"
		payload["id"] = id
		name, _ := payload["name"].(string)
		ctx, _ := payload["context"].(string)
		f.byID[id] = subscriptionRecord{ID: id, Name: name, Context: ctx, Body: payload}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(payload)
	case r.Method == http.MethodGet && len(parts) == 3:
		rec, ok := f.byID[parts[2]]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(rec.Body)
	case r.Method == http.MethodPatch && len(parts) == 3:
		rec, ok := f.byID[parts[2]]
		if !ok {
			http.NotFound(w, r)
			return
		}
		var payload map[string]any
		_ = json.Unmarshal(raw, &payload)
		f.patches = append(f.patches, payload)
		for k, v := range payload {
			rec.Body[k] = v
		}
		if name, _ := payload["name"].(string); name != "" {
			rec.Name = name
		}
		if ctx, _ := payload["context"].(string); ctx != "" {
			rec.Context = ctx
		}
		f.byID[parts[2]] = rec
		_ = json.NewEncoder(w).Encode(rec.Body)
	case r.Method == http.MethodDelete && len(parts) == 3:
		if _, ok := f.byID[parts[2]]; !ok {
			http.NotFound(w, r)
			return
		}
		delete(f.byID, parts[2])
		f.deleted = append(f.deleted, parts[2])
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && len(parts) == 4 && parts[3] == "recover":
		if _, ok := f.byID[parts[2]]; !ok {
			http.NotFound(w, r)
			return
		}
		var payload map[string]any
		_ = json.Unmarshal(raw, &payload)
		f.recovers = append(f.recovers, payload)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"requestId": "req-1", "subscriptionId": parts[2],
			"forceCloseRequested": true, "closed": true, "restarted": true,
			"message": "restarted",
		})
	default:
		http.NotFound(w, r)
	}
}

func startSubscriptionAPI(t *testing.T, fake *subscriptionFake) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	return srv
}

func subscriptionEnv(t *testing.T, dir, baseURL string) {
	t.Helper()
	isolateCLI(t, dir)
	t.Setenv("POLY_API_KEY", "test-key-xxxx")
	t.Setenv("POLY_API_BASE_URL", baseURL)
}

func runSubscription(args ...string) (string, string, int) {
	all := append([]string{"--non-interactive"}, args...)
	return run(all...)
}

func TestSubscriptionListShowsContextNameEnabled(t *testing.T) {
	dir := t.TempDir()
	fake := newSubscriptionFake()
	fake.put("s1", "shopify", "ordersStream", map[string]any{"enabled": true})
	fake.put("s2", "opera", "events", map[string]any{"enabled": false})
	srv := startSubscriptionAPI(t, fake)
	subscriptionEnv(t, dir, srv.URL)

	stdout, stderr, code := runSubscription("subscription", "list", "--context", "shopify")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout)
	if !strings.Contains(got, "NAME") || !strings.Contains(got, "ENABLED") || !strings.Contains(got, "ID") {
		t.Fatalf("header:\n%s", got)
	}
	if !strings.Contains(got, "shopify.ordersStream") || !strings.Contains(got, "true") {
		t.Fatalf("expected shopify row:\n%s", got)
	}
	if strings.Contains(got, "events") {
		t.Fatalf("opera leaked:\n%s", got)
	}
}

func TestSubscriptionCreateResolvesFunctionAndSendsOTP(t *testing.T) {
	dir := t.TempDir()
	fake := newSubscriptionFake()
	fake.putFn("fn-9", "shopify", "handleOrder")
	srv := startSubscriptionAPI(t, fake)
	subscriptionEnv(t, dir, srv.URL)

	stdout, stderr, code := runSubscription(
		"subscription", "create",
		"--name", "ordersStream", "--context", "shopify", "--type", "CUSTOM",
		"--websocket-url", "wss://example.com/graphql",
		"--query", "subscription { orderUpdated { id } }",
		"--function", "shopify.handleOrder",
		"--otp", "123456",
	)
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(plain(stdout), "created") {
		t.Fatalf("stdout=%q", stdout)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.posts) != 1 {
		t.Fatalf("posts=%v", fake.posts)
	}
	p := fake.posts[0]
	if p["name"] != "ordersStream" || p["context"] != "shopify" || p["type"] != "CUSTOM" {
		t.Fatalf("%v", p)
	}
	if p["functionId"] != "fn-9" {
		t.Fatalf("functionId=%v", p["functionId"])
	}
	if p["enabled"] != true {
		t.Fatalf("enabled=%v", p["enabled"])
	}
	if _, ok := p["ohipOffsetSelection"]; ok {
		t.Fatalf("CUSTOM should omit OHIP keys: %v", p)
	}
	if len(fake.otps) == 0 || fake.otps[len(fake.otps)-1] != "123456" {
		t.Fatalf("otps=%v", fake.otps)
	}
}

func TestSubscriptionCreateOHIPRequiresOffsetWithSpecific(t *testing.T) {
	dir := t.TempDir()
	fake := newSubscriptionFake()
	fake.putFn("fn-1", "opera", "handleEvent")
	srv := startSubscriptionAPI(t, fake)
	subscriptionEnv(t, dir, srv.URL)

	_, stderr, code := runSubscription(
		"subscription", "create",
		"--name", "operaEvents", "--context", "opera", "--type", "OHIP",
		"--websocket-url", "wss://ohip.example/graphql",
		"--query", "subscription { newEvent { metadata { offset } } }",
		"--function", "opera.handleEvent",
		"--ohip-offset-selection", "SPECIFIC",
		"--params-object", `{"ohip":{"hostName":"h","appKey":"a","enterpriseId":"e","clientId":"c","clientSecret":"s"}}`,
	)
	if code != exitcode.Usage {
		t.Fatalf("missing offset exit %d stderr=%q", code, stderr)
	}

	_, stderr, code = runSubscription(
		"subscription", "create",
		"--name", "ordersStream", "--context", "shopify", "--type", "CUSTOM",
		"--websocket-url", "wss://example.com/graphql",
		"--query", "subscription { x }",
		"--function", "opera.handleEvent",
		"--ohip-maintain-offset",
	)
	if code != exitcode.Usage {
		t.Fatalf("CUSTOM+OHIP exit %d stderr=%q", code, stderr)
	}

	_, stderr, code = runSubscription(
		"subscription", "create",
		"--name", "ordersStream", "--context", "shopify", "--type", "CUSTOM",
		"--websocket-url", "wss://example.com/graphql",
		"--query", "subscription { x }",
		"--function", "opera.handleEvent",
		"--params-variable", "shopify.creds",
		"--params-object", `{"token":"x"}`,
	)
	if code != exitcode.Usage {
		t.Fatalf("two params exit %d stderr=%q", code, stderr)
	}
}

func TestSubscriptionCreateOHIPSucceeds(t *testing.T) {
	dir := t.TempDir()
	fake := newSubscriptionFake()
	fake.putFn("fn-1", "opera", "handleEvent")
	srv := startSubscriptionAPI(t, fake)
	subscriptionEnv(t, dir, srv.URL)
	params := filepath.Join(dir, "ohip.json")
	if err := os.WriteFile(params, []byte(`{"ohip":{"hostName":"h","appKey":"a","enterpriseId":"e","clientId":"c","clientSecret":"s"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runSubscription(
		"subscription", "create",
		"--name", "operaEvents", "--context", "opera", "--type", "ohip",
		"--websocket-url", "wss://ohip.example/graphql",
		"--query", "subscription { newEvent { metadata { offset } } }",
		"--function", "opera.handleEvent",
		"--params-object-file", params,
		"--ohip-offset-selection", "SPECIFIC",
		"--ohip-offset", "4815",
		"--ohip-maintain-offset",
	)
	if code != 0 {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	p := fake.posts[0]
	if p["type"] != "OHIP" || p["ohipOffset"] != "4815" || p["ohipOffsetSelection"] != "SPECIFIC" {
		t.Fatalf("%v", p)
	}
	if p["ohipMaintainOffset"] != true {
		t.Fatalf("maintain=%v", p["ohipMaintainOffset"])
	}
}

func TestSubscriptionGetByContextNameAndUpdatePatchIsPartial(t *testing.T) {
	dir := t.TempDir()
	fake := newSubscriptionFake()
	fake.put("s1", "shopify", "ordersStream", map[string]any{
		"query": "subscription { old }", "description": "old",
		"paramsObject": map[string]any{"ohip": map[string]any{"clientSecret": "supersecretvalue"}},
	})
	srv := startSubscriptionAPI(t, fake)
	subscriptionEnv(t, dir, srv.URL)

	stdout, stderr, code := runSubscription("subscription", "get", "shopify.ordersStream")
	if code != 0 {
		t.Fatalf("get exit %d stderr=%s", code, stderr)
	}
	if strings.Contains(stdout, "supersecretvalue") {
		t.Fatalf("secret leaked:\n%s", stdout)
	}
	if !strings.Contains(stdout, "********") {
		t.Fatalf("expected redaction:\n%s", stdout)
	}

	_, stderr, code = runSubscription("subscription", "update", "ordersStream", "--context", "shopify", "--description", "new desc", "--otp", "999")
	if code != 0 {
		t.Fatalf("update exit %d stderr=%s", code, stderr)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.patches) != 1 {
		t.Fatalf("patches=%v", fake.patches)
	}
	p := fake.patches[0]
	if p["description"] != "new desc" {
		t.Fatalf("%v", p)
	}
	if _, ok := p["query"]; ok {
		t.Fatalf("query should not be patched: %v", p)
	}
	if _, ok := p["websocketUrl"]; ok {
		t.Fatalf("url should not be patched: %v", p)
	}
	if len(fake.otps) == 0 || fake.otps[len(fake.otps)-1] != "999" {
		t.Fatalf("otps=%v", fake.otps)
	}
}

func TestSubscriptionDeleteAndRecover(t *testing.T) {
	dir := t.TempDir()
	fake := newSubscriptionFake()
	fake.put("s1", "opera", "events", nil)
	srv := startSubscriptionAPI(t, fake)
	subscriptionEnv(t, dir, srv.URL)

	_, stderr, code := runSubscription("subscription", "recover", "opera.events", "--reason", "stuck", "--ohip-offset-selection", "PERSISTED", "--otp", "otp")
	if code != 0 {
		t.Fatalf("recover exit %d stderr=%s", code, stderr)
	}
	fake.mu.Lock()
	if len(fake.recovers) != 1 {
		fake.mu.Unlock()
		t.Fatalf("recovers=%v", fake.recovers)
	}
	if fake.recovers[0]["reason"] != "stuck" || fake.recovers[0]["ohipOffsetSelection"] != "PERSISTED" {
		fake.mu.Unlock()
		t.Fatalf("%v", fake.recovers[0])
	}
	fake.mu.Unlock()

	_, stderr, code = runSubscription("subscription", "delete", "opera.events", "--otp", "del")
	if code != 0 {
		t.Fatalf("delete exit %d stderr=%s", code, stderr)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.deleted) != 1 || fake.deleted[0] != "s1" {
		t.Fatalf("deleted=%v", fake.deleted)
	}
}

func TestSubscriptionAmbiguousName(t *testing.T) {
	dir := t.TempDir()
	fake := newSubscriptionFake()
	fake.put("s1", "a", "dup", nil)
	fake.put("s2", "b", "dup", nil)
	srv := startSubscriptionAPI(t, fake)
	subscriptionEnv(t, dir, srv.URL)
	_, stderr, code := runSubscription("subscription", "get", "dup")
	if code != exitcode.Usage {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
	if !strings.Contains(strings.ToLower(stderr), "unclear") {
		t.Fatalf("stderr=%q", stderr)
	}
}

func TestSubscriptionHelpListsRoot(t *testing.T) {
	stdout, _, code := run("--non-interactive", "--help")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(plain(stdout), "subscription") {
		t.Fatalf("help missing subscription:\n%s", stdout)
	}
}
