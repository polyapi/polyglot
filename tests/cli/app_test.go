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

type appRecord struct {
	ID   string
	Name string
	Body map[string]any
}

type appFake struct {
	mu         sync.Mutex
	byID       map[string]appRecord
	posts      []map[string]any
	patches    []map[string]any
	deleted    []string
	configured []string
}

func newAppFake() *appFake {
	return &appFake{byID: map[string]appRecord{}}
}

func (f *appFake) put(id, name string, extra map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body := map[string]any{
		"id":          id,
		"name":        name,
		"subpath":     "demo",
		"description": "",
		"visibility":  "ENVIRONMENT",
		"config":      nil,
	}
	for k, v := range extra {
		body[k] = v
	}
	if sub, _ := body["subpath"].(string); sub == "" {
		if cfg, ok := body["config"].(map[string]any); ok {
			if s, _ := cfg["subpath"].(string); s != "" {
				body["subpath"] = s
			}
		}
	}
	f.byID[id] = appRecord{ID: id, Name: name, Body: body}
}

func (f *appFake) listItem(rec appRecord) map[string]any {
	item := map[string]any{
		"id":         rec.ID,
		"name":       rec.Name,
		"subpath":    rec.Body["subpath"],
		"visibility": rec.Body["visibility"],
		"config":     nil,
	}
	return item
}

func (f *appFake) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := strings.Trim(r.URL.Path, "/")
		parts := strings.Split(path, "/")
		if len(parts) == 0 || parts[0] != "applications" {
			http.NotFound(w, r)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && len(parts) == 2 && parts[1] == "configured":
			items := make([]any, 0)
			for _, rec := range f.byID {
				cfg, _ := rec.Body["config"].(map[string]any)
				cols, _ := cfg["collections"].([]any)
				if rec.Body["subpath"] == "" || rec.Body["subpath"] == nil || len(cols) == 0 {
					continue
				}
				item := f.listItem(rec)
				items = append(items, item)
				f.configured = append(f.configured, rec.ID)
			}
			_ = json.NewEncoder(w).Encode(items)
		case r.Method == http.MethodGet && len(parts) == 1:
			items := make([]any, 0, len(f.byID))
			for _, rec := range f.byID {
				items = append(items, f.listItem(rec))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results":    items,
				"pagination": map[string]any{"page": 1, "pages": 1},
			})
		case r.Method == http.MethodPost && len(parts) == 1:
			var payload map[string]any
			_ = json.Unmarshal(raw, &payload)
			f.posts = append(f.posts, payload)
			id := "app-1"
			payload["id"] = id
			name, _ := payload["name"].(string)
			if cfg, ok := payload["config"].(map[string]any); ok {
				if sub, _ := cfg["subpath"].(string); sub != "" {
					payload["subpath"] = sub
				}
			}
			if _, ok := payload["subpath"]; !ok {
				payload["subpath"] = ""
			}
			f.byID[id] = appRecord{ID: id, Name: name, Body: payload}
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
			if cfg, ok := payload["config"].(map[string]any); ok {
				if sub, _ := cfg["subpath"].(string); sub != "" {
					rec.Body["subpath"] = sub
				}
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

func startAppAPI(t *testing.T, fake *appFake) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	return srv
}

func appEnv(t *testing.T, dir, baseURL string) {
	t.Helper()
	isolateCLI(t, dir)
	t.Setenv("POLY_API_KEY", "test-key-xxxx")
	t.Setenv("POLY_API_BASE_URL", baseURL)
}

func runApp(args ...string) (string, string, int) {
	all := append([]string{"--non-interactive"}, args...)
	return run(all...)
}

func TestAppListShowsSubpathAndVisibility(t *testing.T) {
	dir := t.TempDir()
	fake := newAppFake()
	fake.put("a1", "dashboard", map[string]any{"subpath": "quantum-quirk-dashboard", "visibility": "ENVIRONMENT"})
	fake.put("a2", "partner", map[string]any{"subpath": "partners", "visibility": "TENANT"})
	srv := startAppAPI(t, fake)
	appEnv(t, dir, srv.URL)

	stdout, stderr, code := runApp("app", "list")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout)
	if !strings.Contains(got, "NAME") || !strings.Contains(got, "SUBPATH") || !strings.Contains(got, "VISIBILITY") || !strings.Contains(got, "ID") {
		t.Fatalf("expected header:\n%s", got)
	}
	if !strings.Contains(got, "dashboard") || !strings.Contains(got, "quantum-quirk-dashboard") {
		t.Fatalf("expected dashboard:\n%s", got)
	}
	if !strings.Contains(got, "partner") || !strings.Contains(got, "TENANT") {
		t.Fatalf("expected partner:\n%s", got)
	}
	if strings.Contains(got, `"collections"`) {
		t.Fatalf("list should omit config:\n%s", got)
	}
}

func TestAppListConfiguredFiltersEmptyCollections(t *testing.T) {
	dir := t.TempDir()
	fake := newAppFake()
	fake.put("a1", "ready", map[string]any{
		"subpath": "ready",
		"config":  map[string]any{"subpath": "ready", "collections": []any{map[string]any{"id": "tasks"}}},
	})
	fake.put("a2", "empty", map[string]any{
		"subpath": "empty",
		"config":  map[string]any{"subpath": "empty", "collections": []any{}},
	})
	srv := startAppAPI(t, fake)
	appEnv(t, dir, srv.URL)

	stdout, stderr, code := runApp("app", "list", "--configured")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout)
	if !strings.Contains(got, "ready") {
		t.Fatalf("expected configured app:\n%s", got)
	}
	if strings.Contains(got, "empty") {
		t.Fatalf("empty collections leaked:\n%s", got)
	}
}

func TestAppGetByIDNameAndSubpath(t *testing.T) {
	dir := t.TempDir()
	fake := newAppFake()
	fake.put("abc123", "dashboard", map[string]any{
		"subpath": "quantum-quirk-dashboard",
		"config": map[string]any{
			"name":        "dashboard",
			"subpath":     "quantum-quirk-dashboard",
			"collections": []any{},
		},
	})
	srv := startAppAPI(t, fake)
	appEnv(t, dir, srv.URL)

	stdout, stderr, code := runApp("app", "get", "abc123")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"name": "dashboard"`) || !strings.Contains(stdout, "quantum-quirk-dashboard") {
		t.Fatalf("get by id:\n%s", stdout)
	}

	stdout, stderr, code = runApp("app", "get", "dashboard")
	if code != 0 {
		t.Fatalf("get by name exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"id": "abc123"`) {
		t.Fatalf("get by name:\n%s", stdout)
	}

	stdout, stderr, code = runApp("app", "get", "quantum-quirk-dashboard")
	if code != 0 {
		t.Fatalf("get by subpath exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"id": "abc123"`) {
		t.Fatalf("get by subpath:\n%s", stdout)
	}

	stdout, stderr, code = runApp("app", "get", "dashboard", "--config")
	if code != 0 {
		t.Fatalf("get --config exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"subpath": "quantum-quirk-dashboard"`) || strings.Contains(stdout, `"id": "abc123"`) {
		t.Fatalf("config-only should omit the application envelope:\n%s", stdout)
	}
}

func TestAppCreatePostsDefaultConfigAndVisibility(t *testing.T) {
	dir := t.TempDir()
	fake := newAppFake()
	srv := startAppAPI(t, fake)
	appEnv(t, dir, srv.URL)

	stdout, stderr, code := runApp("app", "create", "--name", "Quantum Quirk")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout)
	if !strings.Contains(got, "created") || !strings.Contains(got, "Quantum Quirk") {
		t.Fatalf("stdout=%s", stdout)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.posts) != 1 {
		t.Fatalf("posts=%d", len(fake.posts))
	}
	post := fake.posts[0]
	if post["name"] != "Quantum Quirk" || post["visibility"] != "ENVIRONMENT" {
		t.Fatalf("post=%v", post)
	}
	cfg, _ := post["config"].(map[string]any)
	if cfg["name"] != "Quantum Quirk" || cfg["subpath"] != "quantum-quirk" {
		t.Fatalf("config=%v", cfg)
	}
	cols, _ := cfg["collections"].([]any)
	if cols == nil {
		t.Fatalf("expected collections array: %v", cfg)
	}
}

func TestAppCreateFromConfigFileWrapper(t *testing.T) {
	dir := t.TempDir()
	fake := newAppFake()
	srv := startAppAPI(t, fake)
	appEnv(t, dir, srv.URL)
	path := filepath.Join(dir, "app.json")
	body := `{"config":{"name":"dashboard","subpath":"partner-portal","collections":[{"id":"tasks","name":"Tasks"}]}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runApp("app", "create", "--name", "dashboard", "--config-file", path)
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	cfg, _ := fake.posts[0]["config"].(map[string]any)
	if cfg["subpath"] != "partner-portal" {
		t.Fatalf("config=%v", cfg)
	}
	cols, _ := cfg["collections"].([]any)
	if len(cols) != 1 {
		t.Fatalf("collections=%v", cfg["collections"])
	}
}

func TestAppUpdatePatchesConfigAndSubpath(t *testing.T) {
	dir := t.TempDir()
	fake := newAppFake()
	fake.put("abc123", "dashboard", map[string]any{
		"subpath": "dashboard",
		"config":  map[string]any{"name": "dashboard", "subpath": "dashboard", "collections": []any{}},
	})
	srv := startAppAPI(t, fake)
	appEnv(t, dir, srv.URL)

	stdout, stderr, code := runApp("app", "update", "dashboard", "--subpath", "partner-portal", "--description", "Partner UI")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(plain(stdout), "updated") {
		t.Fatalf("stdout=%s", stdout)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.patches) != 1 {
		t.Fatalf("patches=%d", len(fake.patches))
	}
	patch := fake.patches[0]
	if patch["description"] != "Partner UI" {
		t.Fatalf("patch=%v", patch)
	}
	cfg, _ := patch["config"].(map[string]any)
	if cfg["subpath"] != "partner-portal" {
		t.Fatalf("config=%v", cfg)
	}
}

func TestAppDeleteByName(t *testing.T) {
	dir := t.TempDir()
	fake := newAppFake()
	fake.put("abc123", "dashboard", nil)
	srv := startAppAPI(t, fake)
	appEnv(t, dir, srv.URL)

	stdout, stderr, code := runApp("app", "delete", "dashboard")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(plain(stdout), "deleted") {
		t.Fatalf("stdout=%s", stdout)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.deleted) != 1 || fake.deleted[0] != "abc123" {
		t.Fatalf("deleted=%v", fake.deleted)
	}
}

func TestAppURLPrintsCanopyPath(t *testing.T) {
	dir := t.TempDir()
	fake := newAppFake()
	fake.put("abc123", "dashboard", map[string]any{"subpath": "quantum-quirk-dashboard"})
	srv := startAppAPI(t, fake)
	appEnv(t, dir, srv.URL)

	stdout, stderr, code := runApp("app", "url", "dashboard")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := strings.TrimSpace(stdout)
	if !strings.HasSuffix(got, "/canopy/quantum-quirk-dashboard") {
		t.Fatalf("url=%q", got)
	}
}

func TestAppUnknownNameIsUsage(t *testing.T) {
	dir := t.TempDir()
	fake := newAppFake()
	srv := startAppAPI(t, fake)
	appEnv(t, dir, srv.URL)
	_, stderr, code := runApp("app", "get", "missing")
	if code != exitcode.Usage {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
}

func TestAppCreateRejectsBothConfigFlags(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	_, stderr, code := runApp("app", "create", "--name", "dashboard", "--config", `{"collections":[]}`, "--config-file", "nope.json")
	if code != exitcode.Usage {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
}

func TestAppCanopyAliasIsRegistered(t *testing.T) {
	stdout, _, code := run("canopy", "--help")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	got := plain(stdout)
	if !strings.Contains(got, "app") && !strings.Contains(got, "Canopy") {
		t.Fatalf("expected canopy alias help:\n%s", got)
	}
}
