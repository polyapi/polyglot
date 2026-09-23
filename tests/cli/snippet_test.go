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

type snippetRecord struct {
	ID      string
	Name    string
	Context string
	Body    map[string]any
}

type snippetFake struct {
	mu      sync.Mutex
	byID    map[string]snippetRecord
	puts    []map[string]any
	patches []map[string]any
	deleted []string
	seq     int
}

func newSnippetFake() *snippetFake {
	return &snippetFake{byID: map[string]snippetRecord{}}
}

func (f *snippetFake) put(id, context, name string, extra map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body := map[string]any{
		"id":          id,
		"name":        name,
		"context":     context,
		"language":    "typescript",
		"visibility":  "ENVIRONMENT",
		"description": "",
		"code":        "export const n = 1;\n",
	}
	for k, v := range extra {
		body[k] = v
	}
	f.byID[id] = snippetRecord{ID: id, Name: name, Context: context, Body: body}
}

func (f *snippetFake) listItem(rec snippetRecord) map[string]any {
	return map[string]any{
		"id": rec.ID, "name": rec.Name, "context": rec.Context,
		"language": rec.Body["language"], "visibility": rec.Body["visibility"],
		"description": rec.Body["description"],
	}
}

func (f *snippetFake) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := strings.Trim(r.URL.Path, "/")
		parts := strings.Split(path, "/")
		if len(parts) == 0 || parts[0] != "snippets" {
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
		case r.Method == http.MethodPut && len(parts) == 1:
			var payload map[string]any
			_ = json.Unmarshal(raw, &payload)
			f.puts = append(f.puts, payload)
			name, _ := payload["name"].(string)
			ctx, _ := payload["context"].(string)
			for id, rec := range f.byID {
				if rec.Name == name && rec.Context == ctx {
					for k, v := range payload {
						rec.Body[k] = v
					}
					rec.Body["id"] = id
					f.byID[id] = rec
					_ = json.NewEncoder(w).Encode(rec.Body)
					return
				}
			}
			f.seq++
			id := "snip-1"
			if f.seq > 1 {
				id = "snip-2"
			}
			payload["id"] = id
			f.byID[id] = snippetRecord{ID: id, Name: name, Context: ctx, Body: payload}
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

func startSnippetAPI(t *testing.T, fake *snippetFake) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	return srv
}

func snippetEnv(t *testing.T, dir, baseURL string) {
	t.Helper()
	isolateCLI(t, dir)
	t.Setenv("POLY_API_KEY", "test-key-xxxx")
	t.Setenv("POLY_API_BASE_URL", baseURL)
}

func runSnippet(args ...string) (string, string, int) {
	all := append([]string{"--non-interactive"}, args...)
	return run(all...)
}

func TestSnippetListFiltersByContextAndShowsLanguage(t *testing.T) {
	dir := t.TempDir()
	fake := newSnippetFake()
	fake.put("s1", "billing", "header", map[string]any{"language": "typescript", "visibility": "ENVIRONMENT"})
	fake.put("s2", "maps", "note", map[string]any{"language": "markdown", "visibility": "PUBLIC"})
	srv := startSnippetAPI(t, fake)
	snippetEnv(t, dir, srv.URL)

	stdout, stderr, code := runSnippet("snippet", "list", "--context", "billing")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout)
	if !strings.Contains(got, "NAME") || !strings.Contains(got, "LANGUAGE") || !strings.Contains(got, "VISIBILITY") {
		t.Fatalf("expected column header:\n%s", got)
	}
	if !strings.Contains(got, "billing.header") || !strings.Contains(got, "typescript") {
		t.Fatalf("expected billing snippet:\n%s", got)
	}
	if strings.Contains(got, "note") {
		t.Fatalf("maps leaked:\n%s", got)
	}
}

func TestSnippetGetJSONAndCodeOnly(t *testing.T) {
	dir := t.TempDir()
	fake := newSnippetFake()
	fake.put("abc123", "billing", "header", map[string]any{"code": "export const n = 3;\n"})
	srv := startSnippetAPI(t, fake)
	snippetEnv(t, dir, srv.URL)

	stdout, stderr, code := runSnippet("snippet", "get", "abc123")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"name": "header"`) || !strings.Contains(stdout, "export const n = 3") {
		t.Fatalf("get by id:\n%s", stdout)
	}

	stdout, stderr, code = runSnippet("snippet", "get", "billing.header", "--code")
	if code != 0 {
		t.Fatalf("code exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if strings.TrimSpace(stdout) != "export const n = 3;" {
		t.Fatalf("code stdout=%q", stdout)
	}
}

func TestSnippetAddPutsFileAndInfersTypeScript(t *testing.T) {
	dir := t.TempDir()
	fake := newSnippetFake()
	srv := startSnippetAPI(t, fake)
	snippetEnv(t, dir, srv.URL)
	path := filepath.Join(dir, "header.ts")
	if err := os.WriteFile(path, []byte("export const n = 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runSnippet("snippet", "add", "header", path, "--context", "billing", "--description", "Shared header")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout)
	if !strings.Contains(got, "added") || !strings.Contains(got, "billing.header") {
		t.Fatalf("stdout=%s", stdout)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.puts) != 1 {
		t.Fatalf("puts=%d", len(fake.puts))
	}
	put := fake.puts[0]
	if put["name"] != "header" || put["context"] != "billing" || put["language"] != "typescript" {
		t.Fatalf("put=%v", put)
	}
	if put["visibility"] != "ENVIRONMENT" || put["code"] != "export const n = 1;\n" {
		t.Fatalf("put=%v", put)
	}
	if put["description"] != "Shared header" {
		t.Fatalf("description=%v", put["description"])
	}
}

func TestSnippetAddInfersPythonAndRequiresLanguageForUnknown(t *testing.T) {
	dir := t.TempDir()
	fake := newSnippetFake()
	srv := startSnippetAPI(t, fake)
	snippetEnv(t, dir, srv.URL)
	py := filepath.Join(dir, "util.py")
	if err := os.WriteFile(py, []byte("n = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runSnippet("snippet", "add", "util", py, "--context", "billing")
	if code != 0 {
		t.Fatalf("py exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	fake.mu.Lock()
	if fake.puts[0]["language"] != "python" {
		fake.mu.Unlock()
		t.Fatalf("language=%v", fake.puts[0]["language"])
	}
	fake.mu.Unlock()

	unknown := filepath.Join(dir, "blob.xyz")
	if err := os.WriteFile(unknown, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code = runSnippet("snippet", "add", "blob", unknown, "--context", "billing")
	if code != exitcode.Usage || !strings.Contains(strings.ToLower(stderr), "language") {
		t.Fatalf("unknown ext exit %d stderr=%s", code, stderr)
	}

	stdout, stderr, code = runSnippet("snippet", "add", "blob", unknown, "--context", "billing", "--language", "text")
	if code != 0 {
		t.Fatalf("override exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestSnippetAddRequiresContext(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	path := filepath.Join(dir, "header.ts")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := runSnippet("snippet", "add", "header", path)
	if code != exitcode.Usage {
		t.Fatalf("missing context exit %d stderr=%s", code, stderr)
	}
}

func TestSnippetUpdateAndDelete(t *testing.T) {
	dir := t.TempDir()
	fake := newSnippetFake()
	fake.put("abc123", "billing", "header", nil)
	srv := startSnippetAPI(t, fake)
	snippetEnv(t, dir, srv.URL)
	path := filepath.Join(dir, "header.js")
	if err := os.WriteFile(path, []byte("module.exports = 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runSnippet("snippet", "update", "header", "--context", "billing",
		"--code-file", path, "--description", "v2")
	if code != 0 {
		t.Fatalf("update exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	fake.mu.Lock()
	if len(fake.patches) != 1 {
		fake.mu.Unlock()
		t.Fatalf("patches=%v", fake.patches)
	}
	if fake.patches[0]["language"] != "javascript" || fake.patches[0]["code"] != "module.exports = 1;\n" {
		fake.mu.Unlock()
		t.Fatalf("patch=%v", fake.patches[0])
	}
	fake.mu.Unlock()

	stdout, stderr, code = runSnippet("snippet", "delete", "billing.header")
	if code != 0 {
		t.Fatalf("delete exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.deleted) != 1 || fake.deleted[0] != "abc123" {
		t.Fatalf("deleted=%v", fake.deleted)
	}
}
