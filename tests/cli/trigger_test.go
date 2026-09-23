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

type triggerRecord struct {
	ID   string
	Name string
	Body map[string]any
}

type triggerFake struct {
	mu        sync.Mutex
	byID      map[string]triggerRecord
	webhooks  map[string]map[string]any
	functions map[string]map[string]any
	posts     []map[string]any
	patches   []map[string]any
	deleted   []string
}

func newTriggerFake() *triggerFake {
	return &triggerFake{
		byID:      map[string]triggerRecord{},
		webhooks:  map[string]map[string]any{},
		functions: map[string]map[string]any{},
	}
}

func (f *triggerFake) putTrigger(id, name string, extra map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body := map[string]any{"id": id, "name": name, "enabled": true, "waitForResponse": true}
	for k, v := range extra {
		body[k] = v
	}
	f.byID[id] = triggerRecord{ID: id, Name: name, Body: body}
}

func (f *triggerFake) putWebhook(id, context, name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.webhooks[id] = map[string]any{"id": id, "name": name, "context": context, "visibility": "ENVIRONMENT"}
}

func (f *triggerFake) putFn(id, context, name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.functions[id] = map[string]any{"id": id, "name": name, "context": context}
}

func (f *triggerFake) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := strings.Trim(r.URL.Path, "/")
		parts := strings.Split(path, "/")
		raw, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case len(parts) >= 2 && parts[0] == "functions" && parts[1] == "server" && r.Method == http.MethodGet:
			items := make([]any, 0, len(f.functions))
			for _, fn := range f.functions {
				items = append(items, fn)
			}
			_ = json.NewEncoder(w).Encode(items)
		case len(parts) >= 1 && parts[0] == "webhooks" && r.Method == http.MethodGet && len(parts) == 1:
			items := make([]any, 0, len(f.webhooks))
			for _, wh := range f.webhooks {
				items = append(items, wh)
			}
			_ = json.NewEncoder(w).Encode(items)
		case len(parts) >= 1 && parts[0] == "webhooks" && r.Method == http.MethodGet && len(parts) == 2:
			for _, wh := range f.webhooks {
				if wh["id"] == parts[1] {
					_ = json.NewEncoder(w).Encode(wh)
					return
				}
			}
			http.NotFound(w, r)
		case len(parts) == 0 || parts[0] != "triggers":
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
			id := "tr-1"
			payload["id"] = id
			if payload["name"] == nil || payload["name"] == "" {
				payload["name"] = "generated"
			}
			name, _ := payload["name"].(string)
			f.byID[id] = triggerRecord{ID: id, Name: name, Body: payload}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(payload)
		case r.Method == http.MethodGet && len(parts) == 2:
			rec, ok := f.byID[parts[1]]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(rec.Body)
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

func startTriggerAPI(t *testing.T, fake *triggerFake) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	return srv
}

func runTrigger(args ...string) (string, string, int) {
	all := append([]string{"--non-interactive"}, args...)
	return run(all...)
}

func TestTriggerListShowsEnabled(t *testing.T) {
	dir := t.TempDir()
	fake := newTriggerFake()
	fake.putTrigger("t1", "weekly", map[string]any{"enabled": true})
	fake.putTrigger("t2", "nightly", map[string]any{"enabled": false})
	srv := startTriggerAPI(t, fake)
	webhookEnv(t, dir, srv.URL)

	stdout, stderr, code := runTrigger("trigger", "list")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout)
	if !strings.Contains(got, "NAME") || !strings.Contains(got, "ENABLED") || !strings.Contains(got, "ID") {
		t.Fatalf("expected header:\n%s", got)
	}
	if !strings.Contains(got, "weekly") || !strings.Contains(got, "true") {
		t.Fatalf("expected weekly:\n%s", got)
	}
}

func TestTriggerCreateLinksWebhookToFunction(t *testing.T) {
	dir := t.TempDir()
	fake := newTriggerFake()
	fake.putWebhook("wh-1", "billing", "hook")
	fake.putFn("fn-1", "billing", "weeklyReport")
	srv := startTriggerAPI(t, fake)
	webhookEnv(t, dir, srv.URL)

	stdout, stderr, code := runTrigger("trigger", "create",
		"--name", "weekly",
		"--webhook", "billing.hook",
		"--function", "billing.weeklyReport",
	)
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(plain(stdout), "created") || !strings.Contains(plain(stdout), "weekly") {
		t.Fatalf("stdout=%s", stdout)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.posts) != 1 {
		t.Fatalf("posts=%d", len(fake.posts))
	}
	post := fake.posts[0]
	src, _ := post["source"].(map[string]any)
	dest, _ := post["destination"].(map[string]any)
	if src["webhookHandleId"] != "wh-1" {
		t.Fatalf("source=%v", src)
	}
	if dest["serverFunctionId"] != "fn-1" {
		t.Fatalf("destination=%v", dest)
	}
	if post["waitForResponse"] != true || post["enabled"] != true {
		t.Fatalf("post=%v", post)
	}
}

func TestTriggerCreateErrorHandler(t *testing.T) {
	dir := t.TempDir()
	fake := newTriggerFake()
	fake.putFn("fn-1", "billing", "weeklyReport")
	srv := startTriggerAPI(t, fake)
	webhookEnv(t, dir, srv.URL)

	stdout, stderr, code := runTrigger("trigger", "create",
		"--name", "on-error",
		"--error-handler-path", "billing.handle",
		"--function", "billing.weeklyReport",
	)
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(plain(stdout), "created") || !strings.Contains(plain(stdout), "on-error") {
		t.Fatalf("stdout=%s", stdout)
	}
	fake.mu.Lock()
	if len(fake.posts) != 1 {
		fake.mu.Unlock()
		t.Fatalf("posts=%d", len(fake.posts))
	}
	post := fake.posts[0]
	src, _ := post["source"].(map[string]any)
	eh, _ := src["errorHandler"].(map[string]any)
	if eh["path"] != "billing.handle" {
		fake.mu.Unlock()
		t.Fatalf("source=%v", src)
	}
	if _, ok := src["webhookHandleId"]; ok {
		fake.mu.Unlock()
		t.Fatalf("error-handler should not set webhookHandleId: %v", src)
	}
	if _, ok := post["waitForResponse"]; ok {
		fake.mu.Unlock()
		t.Fatalf("error-handler should omit waitForResponse: %v", post)
	}
	dest, _ := post["destination"].(map[string]any)
	if dest["serverFunctionId"] != "fn-1" {
		fake.mu.Unlock()
		t.Fatalf("destination=%v", dest)
	}
	fake.mu.Unlock()

	_, stderr, code = runTrigger("trigger", "create",
		"--name", "on-error-wait",
		"--error-handler-path", "billing.handle",
		"--function", "billing.weeklyReport",
		"--wait-for-response=false",
	)
	if code != 0 {
		t.Fatalf("explicit wait exit %d stderr=%s", code, stderr)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.posts) != 2 || fake.posts[1]["waitForResponse"] != false {
		t.Fatalf("explicit wait posts=%v", fake.posts)
	}
}

func TestTriggerCreateRequiresWebhookAndFunction(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	_, stderr, code := runTrigger("trigger", "create", "--function", "billing.fn")
	if code != exitcode.Usage {
		t.Fatalf("missing webhook exit %d stderr=%s", code, stderr)
	}
	_, stderr, code = runTrigger("trigger", "create", "--webhook", "billing.hook")
	if code != exitcode.Usage {
		t.Fatalf("missing function exit %d stderr=%s", code, stderr)
	}
}

func TestTriggerUpdateOnlyAllowedFieldsAndDelete(t *testing.T) {
	dir := t.TempDir()
	fake := newTriggerFake()
	fake.putTrigger("abc123", "weekly", nil)
	srv := startTriggerAPI(t, fake)
	webhookEnv(t, dir, srv.URL)

	stdout, stderr, code := runTrigger("trigger", "update", "weekly", "--wait-for-response=false", "--enabled=false")
	if code != 0 {
		t.Fatalf("update exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	fake.mu.Lock()
	if len(fake.patches) != 1 {
		t.Fatalf("patches=%v", fake.patches)
	}
	if fake.patches[0]["waitForResponse"] != false || fake.patches[0]["enabled"] != false {
		t.Fatalf("patch=%v", fake.patches[0])
	}
	if _, ok := fake.patches[0]["source"]; ok {
		t.Fatalf("update must not send source: %v", fake.patches[0])
	}
	fake.mu.Unlock()

	stdout, stderr, code = runTrigger("trigger", "delete", "WEEKLY")
	if code != 0 {
		t.Fatalf("delete exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.deleted) != 1 || fake.deleted[0] != "abc123" {
		t.Fatalf("deleted=%v", fake.deleted)
	}
}
