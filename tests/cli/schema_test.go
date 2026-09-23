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

type schemaRecord struct {
	ID      string
	Name    string
	Context string
	Body    map[string]any
}

type schemaFake struct {
	mu      sync.Mutex
	byID    map[string]schemaRecord
	posts   []map[string]any
	patches []map[string]any
	deleted []string
}

func newSchemaFake() *schemaFake {
	return &schemaFake{byID: map[string]schemaRecord{}}
}

func (f *schemaFake) put(id, context, name string, extra map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body := map[string]any{
		"id":         id,
		"name":       name,
		"context":    context,
		"visibility": "ENVIRONMENT",
		"definition": map[string]any{"type": "object"},
	}
	for k, v := range extra {
		body[k] = v
	}
	f.byID[id] = schemaRecord{ID: id, Name: name, Context: context, Body: body}
}

func (f *schemaFake) listItem(rec schemaRecord) map[string]any {
	return map[string]any{
		"id": rec.ID, "name": rec.Name, "context": rec.Context,
		"visibility": rec.Body["visibility"],
	}
}

func (f *schemaFake) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := strings.Trim(r.URL.Path, "/")
		parts := strings.Split(path, "/")
		if len(parts) == 0 || parts[0] != "schemas" {
			http.NotFound(w, r)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && len(parts) == 1:
			items := make([]any, 0, len(f.byID))
			for _, rec := range f.byID {
				items = append(items, f.listItem(rec))
			}
			_ = json.NewEncoder(w).Encode(items)
		case r.Method == http.MethodPost && len(parts) == 1:
			var payload map[string]any
			_ = json.Unmarshal(raw, &payload)
			f.posts = append(f.posts, payload)
			id := "sch-1"
			payload["id"] = id
			name, _ := payload["name"].(string)
			ctx, _ := payload["context"].(string)
			f.byID[id] = schemaRecord{ID: id, Name: name, Context: ctx, Body: payload}
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

func startSchemaAPI(t *testing.T, fake *schemaFake) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	return srv
}

func schemaEnv(t *testing.T, dir, baseURL string) {
	t.Helper()
	isolateCLI(t, dir)
	t.Setenv("POLY_API_KEY", "test-key-xxxx")
	t.Setenv("POLY_API_BASE_URL", baseURL)
}

func runSchema(args ...string) (string, string, int) {
	all := append([]string{"--non-interactive"}, args...)
	return run(all...)
}

func TestSchemaListFiltersByContextAndShowsVisibility(t *testing.T) {
	dir := t.TempDir()
	fake := newSchemaFake()
	fake.put("s1", "billing", "Order", map[string]any{"visibility": "ENVIRONMENT"})
	fake.put("s2", "maps", "Place", map[string]any{"visibility": "PUBLIC"})
	srv := startSchemaAPI(t, fake)
	schemaEnv(t, dir, srv.URL)

	stdout, stderr, code := runSchema("schema", "list", "--context", "billing")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout)
	if !strings.Contains(got, "NAME") || !strings.Contains(got, "VISIBILITY") || !strings.Contains(got, "ID") {
		t.Fatalf("expected column header:\n%s", got)
	}
	if !strings.Contains(got, "billing.Order") || !strings.Contains(got, "ENVIRONMENT") {
		t.Fatalf("expected billing schema:\n%s", got)
	}
	if strings.Contains(got, "Place") {
		t.Fatalf("maps leaked:\n%s", got)
	}
	if strings.Contains(got, `"type"`) {
		t.Fatalf("list should omit definition:\n%s", got)
	}
}

func TestSchemaGetByIDNameAndContextName(t *testing.T) {
	dir := t.TempDir()
	fake := newSchemaFake()
	fake.put("abc123", "billing", "Order", map[string]any{
		"definition": map[string]any{"type": "object", "properties": map[string]any{"sku": map[string]any{"type": "string"}}},
	})
	srv := startSchemaAPI(t, fake)
	schemaEnv(t, dir, srv.URL)

	stdout, stderr, code := runSchema("schema", "get", "abc123")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"name": "Order"`) || !strings.Contains(stdout, "sku") {
		t.Fatalf("get by id:\n%s", stdout)
	}

	stdout, stderr, code = runSchema("schema", "get", "Order", "--context", "billing")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"id": "abc123"`) {
		t.Fatalf("get by name:\n%s", stdout)
	}

	stdout, stderr, code = runSchema("schema", "get", "billing.Order")
	if code != 0 {
		t.Fatalf("context.name exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"id": "abc123"`) {
		t.Fatalf("get by context.name:\n%s", stdout)
	}
}

func TestSchemaCreatePostsDefinitionAndDefaultVisibility(t *testing.T) {
	dir := t.TempDir()
	fake := newSchemaFake()
	srv := startSchemaAPI(t, fake)
	schemaEnv(t, dir, srv.URL)

	stdout, stderr, code := runSchema("schema", "create",
		"--name", "Order",
		"--context", "billing",
		"--definition", `{"type":"object","properties":{"sku":{"type":"string"}}}`,
	)
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout)
	if !strings.Contains(got, "created") || !strings.Contains(got, "billing.Order") {
		t.Fatalf("stdout=%s", stdout)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.posts) != 1 {
		t.Fatalf("posts=%d", len(fake.posts))
	}
	post := fake.posts[0]
	if post["name"] != "Order" || post["context"] != "billing" || post["visibility"] != "ENVIRONMENT" {
		t.Fatalf("post=%v", post)
	}
	def, _ := post["definition"].(map[string]any)
	if def["type"] != "object" {
		t.Fatalf("definition=%v", post["definition"])
	}
}

func TestSchemaCreateFromDefinitionFileWrapper(t *testing.T) {
	dir := t.TempDir()
	fake := newSchemaFake()
	srv := startSchemaAPI(t, fake)
	schemaEnv(t, dir, srv.URL)
	path := filepath.Join(dir, "order.json")
	if err := os.WriteFile(path, []byte(`{"definition":{"type":"object","required":["sku"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runSchema("schema", "create", "--name", "Order", "--context", "billing", "--definition-file", path)
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	def, _ := fake.posts[0]["definition"].(map[string]any)
	req, _ := def["required"].([]any)
	if len(req) != 1 || req[0] != "sku" {
		t.Fatalf("definition=%v", fake.posts[0]["definition"])
	}
}

func TestSchemaCreateRequiresDefinition(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	_, stderr, code := runSchema("schema", "create", "--name", "Order", "--context", "billing")
	if code != exitcode.Usage {
		t.Fatalf("missing definition exit %d stderr=%s", code, stderr)
	}
}

func TestSchemaUpdateAndDelete(t *testing.T) {
	dir := t.TempDir()
	fake := newSchemaFake()
	fake.put("abc123", "billing", "Order", nil)
	srv := startSchemaAPI(t, fake)
	schemaEnv(t, dir, srv.URL)

	stdout, stderr, code := runSchema("schema", "update", "Order", "--context", "billing",
		"--definition", `{"type":"object"}`, "--visibility", "TENANT")
	if code != 0 {
		t.Fatalf("update exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	fake.mu.Lock()
	if len(fake.patches) != 1 {
		fake.mu.Unlock()
		t.Fatalf("patches=%v", fake.patches)
	}
	if fake.patches[0]["visibility"] != "TENANT" {
		fake.mu.Unlock()
		t.Fatalf("patch=%v", fake.patches[0])
	}
	fake.mu.Unlock()

	stdout, stderr, code = runSchema("schema", "delete", "billing.Order")
	if code != 0 {
		t.Fatalf("delete exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.deleted) != 1 || fake.deleted[0] != "abc123" {
		t.Fatalf("deleted=%v", fake.deleted)
	}
}
