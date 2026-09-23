package cli_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/polyapi/polyglot/src/exitcode"
)

type modelFake struct {
	mu sync.Mutex

	permissions map[string]bool
	translate   map[string]any
	describe    map[string]any
	validateErr string
	putFailName string

	oasQuery     string
	oasBody      string
	oasType      string
	describeAPI  int
	describeWH   int
	validateAPI  int
	validateWH   int
	putFunctions []map[string]any
	putWebhooks  []map[string]any
	putSchemas   []map[string]any
}

func newModelFake() *modelFake {
	return &modelFake{
		permissions: map[string]bool{
			"manageApiFunctions": true,
			"manageSchemas":      true,
			"manageWebhooks":     true,
		},
		translate: map[string]any{
			"title": "Fake json placeholder spec",
			"functions": []any{
				map[string]any{
					"name":        "createPost",
					"context":     "jsonPlaceholder",
					"description": "Creates a new post.",
					"arguments": []any{
						map[string]any{"name": "hostUrl", "type": "string", "required": true},
					},
					"source": map[string]any{"method": "POST", "url": "{{hostUrl}}/posts"},
				},
			},
			"webhooks": []any{},
			"schemas": []any{
				map[string]any{
					"name":    "C",
					"context": "mews",
					"definition": map[string]any{
						"allOf": []any{map[string]any{"x-poly-ref": map[string]any{"path": "mews.B"}}},
					},
				},
				map[string]any{
					"name":    "B",
					"context": "mews",
					"definition": map[string]any{
						"properties": map[string]any{
							"a": map[string]any{"x-poly-ref": map[string]any{"path": "mews.A"}},
						},
					},
				},
				map[string]any{
					"name":       "A",
					"context":    "mews",
					"definition": map[string]any{"type": "object"},
				},
			},
		},
	}
}

func (f *modelFake) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := strings.Trim(r.URL.Path, "/")
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && path == "auth":
			_ = json.NewEncoder(w).Encode(map[string]any{"permissions": f.permissions})
		case r.Method == http.MethodPost && path == "specification-input/oas":
			f.oasQuery = r.URL.RawQuery
			f.oasBody = string(body)
			f.oasType = r.Header.Get("Content-Type")
			_ = json.NewEncoder(w).Encode(f.translate)
		case r.Method == http.MethodPost && path == "functions/api/description-generation":
			f.describeAPI++
			if f.describe != nil {
				_ = json.NewEncoder(w).Encode(f.describe)
				return
			}
			var in map[string]any
			_ = json.Unmarshal(body, &in)
			in["description"] = "AI " + stringify(in["name"])
			_ = json.NewEncoder(w).Encode(in)
		case r.Method == http.MethodPost && path == "webhooks/description-generation":
			f.describeWH++
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "hook", "context": "jsonPlaceholder", "description": "AI hook"})
		case r.Method == http.MethodPost && path == "specification-input/validation/api-function":
			f.validateAPI++
			if f.validateErr != "" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"message": f.validateErr})
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && path == "specification-input/validation/webhook-handle":
			f.validateWH++
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut && path == "functions/api":
			var payload map[string]any
			_ = json.Unmarshal(body, &payload)
			if stringify(payload["name"]) == f.putFailName {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"message": "upsert failed"})
				return
			}
			payload["id"] = "api-1"
			f.putFunctions = append(f.putFunctions, payload)
			_ = json.NewEncoder(w).Encode(payload)
		case r.Method == http.MethodPut && path == "webhooks":
			var payload map[string]any
			_ = json.Unmarshal(body, &payload)
			payload["id"] = "wh-1"
			f.putWebhooks = append(f.putWebhooks, payload)
			_ = json.NewEncoder(w).Encode(payload)
		case r.Method == http.MethodPut && path == "schemas":
			var payload map[string]any
			_ = json.Unmarshal(body, &payload)
			payload["id"] = "schema-" + stringify(payload["name"])
			f.putSchemas = append(f.putSchemas, payload)
			_ = json.NewEncoder(w).Encode(payload)
		default:
			http.NotFound(w, r)
		}
	})
}

func stringify(v any) string {
	s, _ := v.(string)
	return s
}

func startModelAPI(t *testing.T, fake *modelFake) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	return srv
}

func modelEnv(t *testing.T, dir, baseURL string) {
	t.Helper()
	isolateCLI(t, dir)
	t.Setenv("POLY_API_KEY", "test-key-xxxx")
	t.Setenv("POLY_API_BASE_URL", baseURL)
}

func runModel(args ...string) (string, string, int) {
	all := append([]string{"--non-interactive"}, args...)
	return run(all...)
}

func modelFixture(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "fixtures", "model", "json-placeholder-spec.yaml")
}

func TestModelGenerateWritesSpecAndSkipsAI(t *testing.T) {
	dir := t.TempDir()
	fake := newModelFake()
	srv := startModelAPI(t, fake)
	modelEnv(t, dir, srv.URL)
	src := filepath.Join(dir, "json-placeholder-spec.yaml")
	in, err := os.ReadFile(modelFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, in, 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runModel("model", "generate", src, "--context", "jsonPlaceholder", "--host-url-as-argument", "--disable-ai")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout)
	if !strings.Contains(got, "wrote specification input") {
		t.Fatalf("stdout=%s", stdout)
	}
	out := filepath.Join(dir, "fake-json-placeholder-spec.json")
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("missing output: %v\nstdout=%s", err, stdout)
	}
	if !strings.Contains(string(raw), `"name": "createPost"`) {
		t.Fatalf("%s", raw)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.describeAPI != 0 || fake.describeWH != 0 {
		t.Fatalf("AI called describeAPI=%d describeWH=%d", fake.describeAPI, fake.describeWH)
	}
	if !strings.Contains(fake.oasQuery, "context=jsonPlaceholder") || !strings.Contains(fake.oasQuery, "hostUrlAsArgument=hostUrl") {
		t.Fatalf("query %s", fake.oasQuery)
	}
	if !strings.Contains(fake.oasType, "text/plain") {
		t.Fatalf("content-type %s", fake.oasType)
	}
	if !strings.Contains(fake.oasBody, "openapi: 3.0.0") {
		t.Fatalf("body %s", fake.oasBody)
	}
}

func TestModelGenerateDestinationAndRename(t *testing.T) {
	dir := t.TempDir()
	fake := newModelFake()
	srv := startModelAPI(t, fake)
	modelEnv(t, dir, srv.URL)
	src := filepath.Join(dir, "spec.yaml")
	if err := os.WriteFile(src, []byte("openapi: 3.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "out.json")
	stdout, stderr, code := runModel("model", "generate", src, dest, "--disable-ai", "--rename", "createPost:makePost")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	raw, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"name": "makePost"`) {
		t.Fatalf("%s", raw)
	}
	if strings.Contains(string(raw), `"createPost"`) {
		t.Fatalf("rename missed: %s", raw)
	}

	_, stderr, code = runModel("model", "generate", src, dest, "--disable-ai")
	if code != exitcode.Usage {
		t.Fatalf("overwrite exit %d stderr=%s", code, stderr)
	}
	if !strings.Contains(plain(stderr), "already exists") {
		t.Fatalf("stderr=%s", stderr)
	}
}

func TestModelGenerateInvalidHostAndRename(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	t.Setenv("POLY_API_KEY", "test-key-xxxx")
	t.Setenv("POLY_API_BASE_URL", "http://127.0.0.1:1")
	_, stderr, code := runModel("model", "generate", "x.yaml", "--host-url", "not-a-url")
	if code != exitcode.Usage {
		t.Fatalf("host exit %d stderr=%s", code, stderr)
	}
	_, stderr, code = runModel("model", "generate", "x.yaml", "--rename", "nocolon")
	if code != exitcode.Usage {
		t.Fatalf("rename exit %d stderr=%s", code, stderr)
	}
}

func TestModelGenerateMissingFile(t *testing.T) {
	dir := t.TempDir()
	fake := newModelFake()
	srv := startModelAPI(t, fake)
	modelEnv(t, dir, srv.URL)
	_, stderr, code := runModel("model", "generate", filepath.Join(dir, "missing.yaml"), "--disable-ai")
	if code != exitcode.Usage {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
	if !strings.Contains(plain(stderr), "does not exist") {
		t.Fatalf("stderr=%s", stderr)
	}
}

func TestModelGenerateRemoteAndAI(t *testing.T) {
	dir := t.TempDir()
	oas := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "openapi: 3.0.0\ninfo:\n  title: Remote\n")
	}))
	t.Cleanup(oas.Close)
	fake := newModelFake()
	fake.describe = map[string]any{
		"name":        "createPostAI",
		"context":     "fromAI",
		"description": "AI text",
		"arguments":   []any{map[string]any{"name": "hostUrl"}},
	}
	srv := startModelAPI(t, fake)
	modelEnv(t, dir, srv.URL)

	stdout, stderr, code := runModel("model", "generate", oas.URL+"/openapi.yaml", "--context", "billing")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	fake.mu.Lock()
	n := fake.describeAPI
	fake.mu.Unlock()
	if n != 1 {
		t.Fatalf("describeAPI=%d", n)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "fake-json-placeholder-spec.json"))
	if len(matches) != 1 {
		t.Fatalf("outputs %v stdout=%s", matches, stdout)
	}
	raw, _ := os.ReadFile(matches[0])
	if !strings.Contains(string(raw), `"name": "createPostAI"`) {
		t.Fatalf("%s", raw)
	}
	if !strings.Contains(string(raw), `"context": "billing"`) {
		t.Fatalf("context flag should win: %s", raw)
	}
}

func TestModelValidateOKAndDuplicateAndHTTP(t *testing.T) {
	dir := t.TempDir()
	fake := newModelFake()
	srv := startModelAPI(t, fake)
	modelEnv(t, dir, srv.URL)

	okPath := filepath.Join(dir, "ok.json")
	if err := os.WriteFile(okPath, []byte(`{"functions":[{"name":"a","context":"c"}],"webhooks":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runModel("model", "validate", okPath)
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(plain(stdout), "valid") {
		t.Fatalf("stdout=%s", stdout)
	}

	dup := filepath.Join(dir, "dup.json")
	if err := os.WriteFile(dup, []byte(`{"functions":[{"name":"a","context":"c"},{"name":"a","context":"c"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code = runModel("model", "validate", dup)
	if code != exitcode.Failure {
		t.Fatalf("dup exit %d stderr=%s", code, stderr)
	}
	if !strings.Contains(strings.ToLower(plain(stderr)), "duplicated") || !strings.Contains(plain(stderr), "c.a") {
		t.Fatalf("stderr=%s", stderr)
	}

	empty := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(empty, []byte(`{"schemas":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code = runModel("model", "validate", empty)
	if code != exitcode.Failure {
		t.Fatalf("empty exit %d stderr=%s", code, stderr)
	}

	fake.validateErr = "bad dto"
	_, stderr, code = runModel("model", "validate", okPath)
	if code != exitcode.Failure {
		t.Fatalf("http exit %d stderr=%s", code, stderr)
	}
	if !strings.Contains(strings.ToLower(plain(stderr)), "bad dto") {
		t.Fatalf("stderr=%s", stderr)
	}
}

func TestModelTrainOrdersSchemasAndUpserts(t *testing.T) {
	dir := t.TempDir()
	fake := newModelFake()
	srv := startModelAPI(t, fake)
	modelEnv(t, dir, srv.URL)

	spec := `{
	  "functions": [{"name":"createPost","context":"jsonPlaceholder"}],
	  "webhooks": [{"name":"onPost","context":"jsonPlaceholder"}],
	  "schemas": [
	    {"name":"C","context":"mews","definition":{"allOf":[{"x-poly-ref":{"path":"mews.B"}}]}},
	    {"name":"B","context":"mews","definition":{"properties":{"a":{"x-poly-ref":{"path":"mews.A"}}}}},
	    {"name":"A","context":"mews","definition":{"type":"object"}}
	  ]
	}`
	path := filepath.Join(dir, "spec.json")
	if err := os.WriteFile(path, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runModel("model", "train", path)
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout + stderr)
	if !strings.Contains(got, "trained api functions") || !strings.Contains(got, "jsonPlaceholder.createPost") {
		t.Fatalf("stdout=%s", stdout)
	}
	if !strings.Contains(got, "SDK generate skipped") && !strings.Contains(got, "generated") {
		t.Fatalf("expected generate attempt:\n%s", got)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.putFunctions) != 1 || len(fake.putWebhooks) != 1 || len(fake.putSchemas) != 3 {
		t.Fatalf("puts fn=%d wh=%d sc=%d", len(fake.putFunctions), len(fake.putWebhooks), len(fake.putSchemas))
	}
	var names []string
	for _, s := range fake.putSchemas {
		names = append(names, stringify(s["name"]))
	}
	if strings.Join(names, ",") != "A,B,C" {
		t.Fatalf("schema order %v", names)
	}
}

func TestModelTrainPermissionsAndPartial(t *testing.T) {
	dir := t.TempDir()
	fake := newModelFake()
	fake.permissions = map[string]bool{"manageWebhooks": true}
	srv := startModelAPI(t, fake)
	modelEnv(t, dir, srv.URL)
	path := filepath.Join(dir, "spec.json")
	if err := os.WriteFile(path, []byte(`{"functions":[{"name":"a","context":"c"}],"webhooks":[],"schemas":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := runModel("model", "train", path)
	if code != exitcode.Auth {
		t.Fatalf("perm exit %d stderr=%s", code, stderr)
	}
	if !strings.Contains(plain(stderr), "manageApiFunctions") {
		t.Fatalf("stderr=%s", stderr)
	}

	fake.permissions = map[string]bool{"manageApiFunctions": true, "manageSchemas": true, "manageWebhooks": true}
	fake.putFailName = "a"
	_, stderr, code = runModel("model", "train", path)
	if code != exitcode.Failure {
		t.Fatalf("fail exit %d stderr=%s", code, stderr)
	}
	if !strings.Contains(plain(stderr), "failed to train") {
		t.Fatalf("stderr=%s", stderr)
	}
}

func TestModelTrainPartialExit12(t *testing.T) {
	dir := t.TempDir()
	fake := newModelFake()
	fake.putFailName = "b"
	srv := startModelAPI(t, fake)
	modelEnv(t, dir, srv.URL)
	path := filepath.Join(dir, "spec.json")
	body := `{"functions":[{"name":"a","context":"c"},{"name":"b","context":"c"}],"webhooks":[],"schemas":[]}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := runModel("model", "train", path)
	if code != exitcode.Partial {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
}
