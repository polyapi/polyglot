package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/polyapi/polyglot/src/exitcode"
)

func TestPrepareHelpHasNoLazy(t *testing.T) {
	stdout, _, code := run("deploy", "prepare", "--help")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	got := plain(stdout)
	if strings.Contains(got, "lazy") {
		t.Fatalf("lazy should be gone:\n%s", got)
	}
}

func TestValidateNothingFound(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	stdout, stderr, code := run("--non-interactive", "deploy", "validate")
	if code != exitcode.Failure {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := strings.ToLower(plain(stdout + stderr))
	if !strings.Contains(got, "no deployables found") {
		t.Fatalf("expected empty-discovery error:\n%s", stdout+stderr)
	}
}

func TestValidateMissingToken(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	writeJSONArtefact(t, dir, "src/artifacts/vari/apiKey.json", `{
  "name": "apiKey",
  "context": "billing",
  "value": "{{MISSING_TOKEN}}",
  "description": "key"
}`)
	stdout, stderr, code := run("--non-interactive", "deploy", "validate", "--only", "env")
	if code != exitcode.Failure {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout + stderr)
	if !strings.Contains(got, "MISSING_TOKEN") {
		t.Fatalf("expected token error:\n%s", got)
	}
}

func TestPushBlockedOnFeatureBranch(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	writeGitBranch(t, dir, "feature/x")
	writeJSONArtefact(t, dir, ".poly/config.toml", `
[environments.prod]
base_url = "http://127.0.0.1:9"

[deploy]
unallowed_branch = "error"

[[deploy.targets]]
branches = ["main"]
environment = "prod"
`)
	writeJSONArtefact(t, dir, "src/artifacts/vari/apiKey.json", `{
  "name": "apiKey",
  "context": "billing",
  "value": "x",
  "description": "key"
}`)
	t.Setenv("POLY_API_KEY", "test-key-xxxx")
	t.Setenv("POLY_API_BASE_URL", "http://127.0.0.1:9")
	_, stderr, code := run("--non-interactive", "deploy", "push")
	if code != exitcode.Failure {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
	if !strings.Contains(plain(stderr), "deploy.targets") {
		t.Fatalf("stderr=%s", stderr)
	}
}

func TestPlanWouldCreateJSON(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	store := newMemStore()
	srv := httptest.NewServer(store.handler())
	t.Cleanup(srv.Close)
	writeJSONArtefact(t, dir, "src/artifacts/vari/apiKey.json", `{
  "name": "apiKey",
  "context": "billing",
  "value": "x",
  "description": "key"
}`)
	t.Setenv("POLY_API_KEY", "test-key-xxxx")
	t.Setenv("POLY_API_BASE_URL", srv.URL)
	stdout, stderr, code := run("--non-interactive", "deploy", "plan")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout)
	if !strings.Contains(got, "would_create") && !strings.Contains(strings.ToLower(got), "would create") {
		t.Fatalf("plan:\n%s", got)
	}
}

func TestPushThenSkip(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	store := newMemStore()
	srv := httptest.NewServer(store.handler())
	t.Cleanup(srv.Close)
	writeJSONArtefact(t, dir, "src/artifacts/vari/apiKey.json", `{
  "name": "apiKey",
  "context": "billing",
  "value": "x",
  "description": "key"
}`)
	t.Setenv("POLY_API_KEY", "test-key-xxxx")
	t.Setenv("POLY_API_BASE_URL", srv.URL)
	stdout, stderr, code := run("--non-interactive", "deploy", "push")
	if code != 0 {
		t.Fatalf("push1 exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	stdout, stderr, code = run("--non-interactive", "deploy", "push")
	if code != 0 {
		t.Fatalf("push2 exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := strings.ToLower(plain(stdout))
	if !strings.Contains(got, "skip") {
		t.Fatalf("second push:\n%s", stdout)
	}
}

func writeJSONArtefact(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeGitBranch(t *testing.T, root, branch string) {
	t.Helper()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref: refs/heads/"+branch+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

type memStore struct {
	mu    sync.Mutex
	seq   int
	items map[string]map[string]any
}

func newMemStore() *memStore {
	return &memStore{items: map[string]map[string]any{}}
}

func (s *memStore) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key-xxxx" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		path := strings.Trim(r.URL.Path, "/")
		s.mu.Lock()
		defer s.mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			if path == "auth" {
				_ = json.NewEncoder(w).Encode(map[string]any{"tenant": map[string]string{"id": "t"}})
				return
			}
			out := make([]any, 0)
			prefix := path + "/"
			for id, row := range s.items {
				if id == path || strings.HasPrefix(id, prefix) {
					out = append(out, row)
				}
			}
			_ = json.NewEncoder(w).Encode(out)
		case http.MethodPost:
			var row map[string]any
			_ = json.NewDecoder(r.Body).Decode(&row)
			s.seq++
			id := "id-" + strings.ReplaceAll(path, "/", "-") + "-" + itoa(s.seq)
			row["id"] = id
			s.items[path+"/"+id] = row
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(row)
		case http.MethodPatch:
			var patch map[string]any
			_ = json.NewDecoder(r.Body).Decode(&patch)
			row := s.items[path]
			if row == nil {
				http.NotFound(w, r)
				return
			}
			for k, v := range patch {
				row[k] = v
			}
			_ = json.NewEncoder(w).Encode(row)
		case http.MethodDelete:
			delete(s.items, path)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	})
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
