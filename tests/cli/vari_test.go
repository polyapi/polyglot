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

type variRecord struct {
	ID      string
	Name    string
	Context string
	Body    map[string]any
}

type variFake struct {
	mu      sync.Mutex
	byID    map[string]variRecord
	posts   []map[string]any
	patches []map[string]any
	headers []http.Header
	deleted []string
}

func newVariFake() *variFake {
	return &variFake{byID: map[string]variRecord{}}
}

func (f *variFake) put(id, context, name string, extra map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body := map[string]any{
		"id":         id,
		"name":       name,
		"context":    context,
		"visibility": "ENVIRONMENT",
		"secrecy":    "NONE",
		"secret":     false,
	}
	for k, v := range extra {
		body[k] = v
	}
	f.byID[id] = variRecord{ID: id, Name: name, Context: context, Body: body}
}

func (f *variFake) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := strings.Trim(r.URL.Path, "/")
		parts := strings.Split(path, "/")
		if len(parts) == 0 || parts[0] != "variables" {
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
				items = append(items, rec.Body)
			}
			_ = json.NewEncoder(w).Encode(items)
		case r.Method == http.MethodPost && len(parts) == 1:
			var payload map[string]any
			_ = json.Unmarshal(raw, &payload)
			f.posts = append(f.posts, payload)
			id := "var-1"
			payload["id"] = id
			name, _ := payload["name"].(string)
			ctx, _ := payload["context"].(string)
			f.byID[id] = variRecord{ID: id, Name: name, Context: ctx, Body: payload}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(payload)
		case r.Method == http.MethodGet && len(parts) == 2:
			rec, ok := f.byID[parts[1]]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(rec.Body)
		case r.Method == http.MethodGet && len(parts) == 3 && parts[2] == "value":
			rec, ok := f.byID[parts[1]]
			if !ok {
				http.NotFound(w, r)
				return
			}
			secrecy, _ := rec.Body["secrecy"].(string)
			secret, _ := rec.Body["secret"].(bool)
			if secret || strings.EqualFold(secrecy, "SECRET") {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(rec.Body["value"])
		case r.Method == http.MethodPatch && len(parts) == 2:
			rec, ok := f.byID[parts[1]]
			if !ok {
				http.NotFound(w, r)
				return
			}
			var payload map[string]any
			_ = json.Unmarshal(raw, &payload)
			f.patches = append(f.patches, payload)
			h := r.Header.Clone()
			f.headers = append(f.headers, h)
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
			h := r.Header.Clone()
			f.headers = append(f.headers, h)
			delete(f.byID, parts[1])
			f.deleted = append(f.deleted, parts[1])
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
}

func startVariAPI(t *testing.T, fake *variFake) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	return srv
}

func variEnv(t *testing.T, dir, baseURL string) {
	t.Helper()
	isolateCLI(t, dir)
	t.Setenv("POLY_API_KEY", "test-key-xxxx")
	t.Setenv("POLY_API_BASE_URL", baseURL)
}

func runVari(args ...string) (string, string, int) {
	all := append([]string{"--non-interactive"}, args...)
	return run(all...)
}

func TestVariListFiltersByContextAndShowsVisibility(t *testing.T) {
	dir := t.TempDir()
	fake := newVariFake()
	fake.put("v1", "billing", "apiKey", map[string]any{"visibility": "ENVIRONMENT", "secrecy": "SECRET", "secret": true})
	fake.put("v2", "maps", "token", map[string]any{"visibility": "PUBLIC", "secrecy": "NONE"})
	srv := startVariAPI(t, fake)
	variEnv(t, dir, srv.URL)

	stdout, stderr, code := runVari("vari", "list", "--context", "billing")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout)
	if !strings.Contains(got, "NAME") || !strings.Contains(got, "VISIBILITY") || !strings.Contains(got, "ID") {
		t.Fatalf("expected column header:\n%s", got)
	}
	if strings.Contains(got, "SECRECY") {
		t.Fatalf("list should not fetch/print secrecy:\n%s", got)
	}
	if !strings.Contains(got, "billing.apiKey") || !strings.Contains(got, "ENVIRONMENT") {
		t.Fatalf("expected billing variable:\n%s", got)
	}
	if strings.Contains(got, "token") {
		t.Fatalf("maps leaked:\n%s", got)
	}
}

func TestVariGetByIDAndNameRedactsSecret(t *testing.T) {
	dir := t.TempDir()
	fake := newVariFake()
	fake.put("abc123", "billing", "apiKey", map[string]any{
		"description": "Stripe key",
		"secrecy":     "SECRET",
		"secret":      true,
		"value":       "sk-live-super-secret-value",
	})
	srv := startVariAPI(t, fake)
	variEnv(t, dir, srv.URL)

	stdout, stderr, code := runVari("vari", "get", "abc123")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if strings.Contains(stdout, "sk-live-super-secret-value") {
		t.Fatalf("secret leaked:\n%s", stdout)
	}
	if !strings.Contains(stdout, "********") || !strings.Contains(stdout, `"name": "apiKey"`) {
		t.Fatalf("get by id:\n%s", stdout)
	}

	stdout, stderr, code = runVari("vari", "get", "apiKey", "--context", "billing")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if strings.Contains(stdout, "sk-live-super-secret-value") {
		t.Fatalf("secret leaked on name get:\n%s", stdout)
	}

	_, stderr, code = runVari("vari", "get", "abc123", "--value")
	if code != exitcode.Usage {
		t.Fatalf("secret --value exit %d stderr=%s", code, stderr)
	}
}

func TestVariGetValueNonSecret(t *testing.T) {
	dir := t.TempDir()
	fake := newVariFake()
	fake.put("v1", "billing", "region", map[string]any{"value": "us-east-1", "secrecy": "NONE"})
	srv := startVariAPI(t, fake)
	variEnv(t, dir, srv.URL)
	stdout, stderr, code := runVari("vari", "get", "region", "--context", "billing", "--value")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "us-east-1") {
		t.Fatalf("stdout=%s", stdout)
	}
}

func TestVariCreatePostsAndOmitsSecretFromOutput(t *testing.T) {
	dir := t.TempDir()
	fake := newVariFake()
	srv := startVariAPI(t, fake)
	variEnv(t, dir, srv.URL)

	stdout, stderr, code := runVari("vari", "create",
		"--name", "apiKey",
		"--context", "billing",
		"--value", `"sk-live-not-for-stdout"`,
		"--secret",
		"--description", "Stripe",
	)
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout)
	if !strings.Contains(got, "created") || !strings.Contains(got, "billing.apiKey") {
		t.Fatalf("stdout=%s", stdout)
	}
	if strings.Contains(stdout, "sk-live-not-for-stdout") {
		t.Fatalf("secret leaked on create:\n%s", stdout)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.posts) != 1 {
		t.Fatalf("posts=%d", len(fake.posts))
	}
	post := fake.posts[0]
	if post["name"] != "apiKey" || post["context"] != "billing" || post["secrecy"] != "SECRET" {
		t.Fatalf("post=%v", post)
	}
	if post["secret"] != true {
		t.Fatalf("secret flag=%v", post["secret"])
	}
	if post["value"] != "sk-live-not-for-stdout" {
		t.Fatalf("value=%v", post["value"])
	}
	if post["visibility"] != "ENVIRONMENT" {
		t.Fatalf("visibility=%v", post["visibility"])
	}
}

func TestVariCreateJSONObjectAndValueFile(t *testing.T) {
	dir := t.TempDir()
	fake := newVariFake()
	srv := startVariAPI(t, fake)
	variEnv(t, dir, srv.URL)
	path := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(path, []byte(`{"region":"us"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runVari("vari", "create", "--name", "config", "--context", "billing", "--value-file", path)
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	val, _ := fake.posts[0]["value"].(map[string]any)
	if val["region"] != "us" {
		t.Fatalf("value=%v", fake.posts[0]["value"])
	}
}

func TestVariCreateRequiresValue(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	_, stderr, code := runVari("vari", "create", "--name", "apiKey", "--context", "billing")
	if code != exitcode.Usage {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
}

func TestVariUpdateAndDelete(t *testing.T) {
	dir := t.TempDir()
	fake := newVariFake()
	fake.put("abc123", "billing", "apiKey", map[string]any{"value": "old"})
	srv := startVariAPI(t, fake)
	variEnv(t, dir, srv.URL)

	stdout, stderr, code := runVari("vari", "update", "apiKey", "--context", "billing", "--value", `"new"`, "--otp", "123456")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(plain(stdout), "updated") {
		t.Fatalf("stdout=%s", stdout)
	}
	fake.mu.Lock()
	if len(fake.patches) != 1 || fake.patches[0]["value"] != "new" {
		t.Fatalf("patches=%v", fake.patches)
	}
	if fake.headers[0].Get("x-otp") != "123456" {
		t.Fatalf("otp header %v", fake.headers[0])
	}
	fake.mu.Unlock()

	stdout, stderr, code = runVari("vari", "delete", "APIKEY", "--context", "Billing")
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

func TestVariCopyNonSecretAndSecretRequiresValue(t *testing.T) {
	dir := t.TempDir()
	fake := newVariFake()
	fake.put("v1", "billing", "apiKey", map[string]any{"value": "plain", "secrecy": "NONE", "description": "key"})
	fake.put("s1", "billing", "secretKey", map[string]any{"value": "********abcd", "secrecy": "SECRET", "secret": true})
	srv := startVariAPI(t, fake)
	variEnv(t, dir, srv.URL)

	stdout, stderr, code := runVari("vari", "copy", "apiKey", "--context", "billing", "--to-context", "staging")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(plain(stdout), "copied") || !strings.Contains(plain(stdout), "staging.apiKey") {
		t.Fatalf("stdout=%s", stdout)
	}
	fake.mu.Lock()
	if len(fake.posts) != 1 {
		t.Fatalf("posts=%d", len(fake.posts))
	}
	post := fake.posts[0]
	if post["context"] != "staging" || post["name"] != "apiKey" || post["value"] != "plain" {
		t.Fatalf("copy post=%v", post)
	}
	fake.mu.Unlock()

	_, stderr, code = runVari("vari", "copy", "secretKey", "--context", "billing", "--to-context", "staging")
	if code != exitcode.Usage {
		t.Fatalf("secret copy exit %d stderr=%s", code, stderr)
	}
	if !strings.Contains(strings.ToLower(plain(stderr)), "secret") {
		t.Fatalf("stderr=%s", stderr)
	}

	stdout, stderr, code = runVari("vari", "copy", "secretKey", "--context", "billing", "--to-context", "staging", "--value", `"sk-staging"`)
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestVariCopySameIdentityIsUsage(t *testing.T) {
	dir := t.TempDir()
	fake := newVariFake()
	fake.put("v1", "billing", "apiKey", map[string]any{"value": "x", "secrecy": "NONE"})
	srv := startVariAPI(t, fake)
	variEnv(t, dir, srv.URL)
	_, stderr, code := runVari("vari", "copy", "apiKey", "--context", "billing")
	if code != exitcode.Usage {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
}

func TestVariMissingNameIsUsage(t *testing.T) {
	dir := t.TempDir()
	fake := newVariFake()
	srv := startVariAPI(t, fake)
	variEnv(t, dir, srv.URL)
	_, stderr, code := runVari("vari", "get", "missing", "--context", "billing")
	if code != exitcode.Usage {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
	if !strings.Contains(strings.ToLower(plain(stderr)), "no variable") {
		t.Fatalf("stderr=%s", stderr)
	}
}
