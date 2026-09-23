package cli_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/polyapi/polyglot/src/exitcode"
)

type webhookRecord struct {
	ID      string
	Name    string
	Context string
	Body    map[string]any
}

type webhookFake struct {
	mu        sync.Mutex
	byID      map[string]webhookRecord
	functions map[string]map[string]any
	posts     []map[string]any
	patches   []map[string]any
	tests     []any
	deleted   []string
}

func newWebhookFake() *webhookFake {
	return &webhookFake{byID: map[string]webhookRecord{}, functions: map[string]map[string]any{}}
}

func (f *webhookFake) put(id, context, name string, extra map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body := map[string]any{
		"id":         id,
		"name":       name,
		"context":    context,
		"visibility": "ENVIRONMENT",
		"method":     "POST",
		"url":        "https://example.test/webhooks/" + id,
		"uri":        "https://example.test/apis/" + id,
	}
	for k, v := range extra {
		body[k] = v
	}
	f.byID[id] = webhookRecord{ID: id, Name: name, Context: context, Body: body}
}

func (f *webhookFake) putFn(id, context, name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.functions[id] = map[string]any{"id": id, "name": name, "context": context}
}

func (f *webhookFake) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := strings.Trim(r.URL.Path, "/")
		parts := strings.Split(path, "/")
		raw, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case len(parts) >= 2 && parts[0] == "functions" && parts[1] == "server" && r.Method == http.MethodGet && len(parts) == 2:
			items := make([]any, 0, len(f.functions))
			for _, fn := range f.functions {
				items = append(items, fn)
			}
			_ = json.NewEncoder(w).Encode(items)
		case len(parts) == 0 || parts[0] != "webhooks":
			http.NotFound(w, r)
		case r.Method == http.MethodGet && len(parts) == 1:
			items := make([]any, 0, len(f.byID))
			for _, rec := range f.byID {
				items = append(items, rec.Body)
			}
			_ = json.NewEncoder(w).Encode(items)
		case r.Method == http.MethodPost && len(parts) == 1:
			var payload map[string]any
			_ = json.Unmarshal(raw, &payload)
			f.posts = append(f.posts, payload)
			id := "wh-1"
			payload["id"] = id
			payload["url"] = "https://example.test/webhooks/" + id
			name, _ := payload["name"].(string)
			ctx, _ := payload["context"].(string)
			f.byID[id] = webhookRecord{ID: id, Name: name, Context: ctx, Body: payload}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(payload)
		case r.Method == http.MethodGet && len(parts) == 2:
			rec, ok := f.byID[parts[1]]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(rec.Body)
		case r.Method == http.MethodPost && len(parts) == 2:
			if _, ok := f.byID[parts[1]]; !ok {
				http.NotFound(w, r)
				return
			}
			var payload any
			_ = json.Unmarshal(raw, &payload)
			f.tests = append(f.tests, payload)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "echo": payload})
		case r.Method == http.MethodPatch && len(parts) == 2:
			rec, ok := f.byID[parts[1]]
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
			f.byID[parts[1]] = rec
			_ = json.NewEncoder(w).Encode(rec.Body)
		case r.Method == http.MethodDelete && len(parts) == 2:
			if _, ok := f.byID[parts[1]]; !ok {
				http.NotFound(w, r)
				return
			}
			delete(f.byID, parts[1])
			f.deleted = append(f.deleted, parts[1])
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
}

func startWebhookAPI(t *testing.T, fake *webhookFake) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	return srv
}

func webhookEnv(t *testing.T, dir, baseURL string) {
	t.Helper()
	isolateCLI(t, dir)
	t.Setenv("POLY_API_KEY", "test-key-xxxx")
	t.Setenv("POLY_API_BASE_URL", baseURL)
}

func runWebhook(args ...string) (string, string, int) {
	all := append([]string{"--non-interactive"}, args...)
	return run(all...)
}

func TestWebhookListFiltersByContextAndShowsVisibility(t *testing.T) {
	dir := t.TempDir()
	fake := newWebhookFake()
	fake.put("w1", "billing", "hook", map[string]any{"visibility": "ENVIRONMENT"})
	fake.put("w2", "maps", "ingest", map[string]any{"visibility": "PUBLIC"})
	srv := startWebhookAPI(t, fake)
	webhookEnv(t, dir, srv.URL)

	stdout, stderr, code := runWebhook("webhook", "list", "--context", "billing")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout)
	if !strings.Contains(got, "NAME") || !strings.Contains(got, "VISIBILITY") || !strings.Contains(got, "ID") {
		t.Fatalf("expected column header:\n%s", got)
	}
	if !strings.Contains(got, "billing.hook") || !strings.Contains(got, "ENVIRONMENT") {
		t.Fatalf("expected billing webhook:\n%s", got)
	}
	if strings.Contains(got, "ingest") {
		t.Fatalf("maps leaked:\n%s", got)
	}
}

func TestWebhookGetByIDAndName(t *testing.T) {
	dir := t.TempDir()
	fake := newWebhookFake()
	fake.put("abc123", "billing", "hook", map[string]any{"description": "Orders"})
	srv := startWebhookAPI(t, fake)
	webhookEnv(t, dir, srv.URL)

	stdout, stderr, code := runWebhook("webhook", "get", "abc123")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"name": "hook"`) {
		t.Fatalf("get by id:\n%s", stdout)
	}
	stdout, stderr, code = runWebhook("webhook", "get", "hook", "--context", "billing")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"id": "abc123"`) {
		t.Fatalf("get by name:\n%s", stdout)
	}
	stdout, stderr, code = runWebhook("webhook", "get", "billing.hook")
	if code != 0 {
		t.Fatalf("context.name exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestWebhookCreateResolvesSecurityFunctionAndDefaults(t *testing.T) {
	dir := t.TempDir()
	fake := newWebhookFake()
	fake.putFn("fn-1", "billing", "hasValidCode")
	srv := startWebhookAPI(t, fake)
	webhookEnv(t, dir, srv.URL)

	stdout, stderr, code := runWebhook("webhook", "create",
		"--name", "hook",
		"--context", "billing",
		"--event-payload", `{"n":3}`,
		"--security-function", "billing.hasValidCode",
	)
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(plain(stdout), "created") || !strings.Contains(plain(stdout), "billing.hook") {
		t.Fatalf("stdout=%s", stdout)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.posts) != 1 {
		t.Fatalf("posts=%d", len(fake.posts))
	}
	post := fake.posts[0]
	if post["method"] != "POST" || post["visibility"] != "ENVIRONMENT" {
		t.Fatalf("post=%v", post)
	}
	if post["requirePolyApiKey"] != false {
		t.Fatalf("requirePolyApiKey=%v", post["requirePolyApiKey"])
	}
	fns, _ := post["securityFunctions"].([]any)
	fn, _ := fns[0].(map[string]any)
	if fn["id"] != "fn-1" {
		t.Fatalf("securityFunctions=%v", post["securityFunctions"])
	}
}

func TestWebhookUpdateDeleteURLAndTest(t *testing.T) {
	dir := t.TempDir()
	fake := newWebhookFake()
	fake.put("abc123", "billing", "hook", nil)
	srv := startWebhookAPI(t, fake)
	webhookEnv(t, dir, srv.URL)

	stdout, stderr, code := runWebhook("webhook", "update", "hook", "--context", "billing", "--description", "new")
	if code != 0 {
		t.Fatalf("update exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	fake.mu.Lock()
	if len(fake.patches) != 1 || fake.patches[0]["description"] != "new" {
		t.Fatalf("patches=%v", fake.patches)
	}
	fake.mu.Unlock()

	stdout, stderr, code = runWebhook("webhook", "url", "hook", "--context", "billing")
	if code != 0 {
		t.Fatalf("url exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "https://example.test/webhooks/abc123") {
		t.Fatalf("url=%s", stdout)
	}

	stdout, stderr, code = runWebhook("webhook", "test", "hook", "--context", "billing", "--data", `{"n":5}`)
	if code != 0 {
		t.Fatalf("test exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"ok": true`) {
		t.Fatalf("test stdout=%s", stdout)
	}
	fake.mu.Lock()
	if len(fake.tests) != 1 {
		t.Fatalf("tests=%v", fake.tests)
	}
	fake.mu.Unlock()

	stdout, stderr, code = runWebhook("webhook", "delete", "HOOK", "--context", "Billing")
	if code != 0 {
		t.Fatalf("delete exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.deleted) != 1 || fake.deleted[0] != "abc123" {
		t.Fatalf("deleted=%v", fake.deleted)
	}
}

func TestWebhookCreateRequiresNameAndContext(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	_, stderr, code := runWebhook("webhook", "create")
	if code != exitcode.Usage {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
}
