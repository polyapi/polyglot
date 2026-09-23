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

	"github.com/polyapi/polyglot/src/cli"
	"github.com/polyapi/polyglot/src/exitcode"
)

type functionRecord struct {
	ID      string
	Kind    string
	Name    string
	Context string
	Body    map[string]any
}

type functionFake struct {
	mu            sync.Mutex
	byID          map[string]functionRecord
	posts         []map[string]any
	deleted       []string
	exec          []map[string]any
	logs          map[string]any
	lastLogsQuery string
}

func newFunctionFake() *functionFake {
	return &functionFake{
		byID: map[string]functionRecord{},
		logs: map[string]any{
			"logsEnabled": true,
			"logs": []any{
				map[string]any{
					"timestamp":   "2024-07-15T18:02:42Z",
					"value":       "hello",
					"level":       "INFO",
					"executionId": "ex1",
					"revision":    "00004",
				},
			},
		},
	}
}

func (f *functionFake) put(kind, id, context, name string, extra map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body := map[string]any{
		"id":      id,
		"name":    name,
		"context": context,
		"type":    kind + "Function",
	}
	for k, v := range extra {
		body[k] = v
	}
	f.byID[kind+"/"+id] = functionRecord{ID: id, Kind: kind, Name: name, Context: context, Body: body}
}

func (f *functionFake) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := strings.Trim(r.URL.Path, "/")
		parts := strings.Split(path, "/")
		if len(parts) < 2 || parts[0] != "functions" {
			http.NotFound(w, r)
			return
		}
		kind := parts[1]
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && len(parts) == 2:
			items := make([]any, 0)
			prefix := kind + "/"
			for key, rec := range f.byID {
				if strings.HasPrefix(key, prefix) {
					items = append(items, rec.Body)
				}
			}
			_ = json.NewEncoder(w).Encode(items)
		case r.Method == http.MethodPost && len(parts) == 2:
			raw, _ := io.ReadAll(r.Body)
			var payload map[string]any
			_ = json.Unmarshal(raw, &payload)
			f.posts = append(f.posts, payload)
			id := "fn-1"
			payload["id"] = id
			payload["type"] = kind + "Function"
			name, _ := payload["name"].(string)
			ctx, _ := payload["context"].(string)
			f.byID[kind+"/"+id] = functionRecord{ID: id, Kind: kind, Name: name, Context: ctx, Body: payload}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(payload)
		case r.Method == http.MethodGet && len(parts) == 3:
			rec, ok := f.byID[kind+"/"+parts[2]]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(rec.Body)
		case r.Method == http.MethodDelete && len(parts) == 3:
			key := kind + "/" + parts[2]
			if _, ok := f.byID[key]; !ok {
				http.NotFound(w, r)
				return
			}
			delete(f.byID, key)
			f.deleted = append(f.deleted, key)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && len(parts) == 4 && parts[3] == "execute":
			raw, _ := io.ReadAll(r.Body)
			var payload map[string]any
			_ = json.Unmarshal(raw, &payload)
			if payload == nil {
				payload = map[string]any{}
			}
			payload["_path"] = r.URL.Path
			f.exec = append(f.exec, payload)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "echo": payload})
		case r.Method == http.MethodGet && len(parts) == 4 && (parts[3] == "logs" || parts[3] == "system-logs"):
			f.lastLogsQuery = r.URL.RawQuery
			out := map[string]any{}
			for k, v := range f.logs {
				out[k] = v
			}
			_ = json.NewEncoder(w).Encode(out)
		case r.Method == http.MethodDelete && len(parts) == 4 && (parts[3] == "logs" || parts[3] == "system-logs"):
			f.deleted = append(f.deleted, kind+"/"+parts[2]+"/"+parts[3])
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
}

func startFunctionAPI(t *testing.T, fake *functionFake) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	return srv
}

func functionEnv(t *testing.T, dir, baseURL string) {
	t.Helper()
	isolateCLI(t, dir)
	t.Setenv("POLY_API_KEY", "test-key-xxxx")
	t.Setenv("POLY_API_BASE_URL", baseURL)
}

func runFn(args ...string) (string, string, int) {
	all := append([]string{"--non-interactive"}, args...)
	return run(all...)
}

func TestFunctionListFiltersByContextAndType(t *testing.T) {
	dir := t.TempDir()
	fake := newFunctionFake()
	fake.put("server", "s1", "billing", "echo", map[string]any{"visibility": "ENVIRONMENT"})
	fake.put("server", "s2", "maps", "geocode", map[string]any{"visibility": "TENANT"})
	fake.put("client", "c1", "billing", "greet", map[string]any{"visibility": "PUBLIC"})
	srv := startFunctionAPI(t, fake)
	functionEnv(t, dir, srv.URL)

	stdout, stderr, code := runFn("function", "list", "--context", "billing")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout)
	if !strings.Contains(got, "TYPE") || !strings.Contains(got, "NAME") || !strings.Contains(got, "VISIBILITY") || !strings.Contains(got, "ID") {
		t.Fatalf("expected column header:\n%s", got)
	}
	if !strings.Contains(got, "echo") || !strings.Contains(got, "greet") {
		t.Fatalf("expected billing functions:\n%s", got)
	}
	if !strings.Contains(got, "ENVIRONMENT") || !strings.Contains(got, "PUBLIC") {
		t.Fatalf("expected visibility values:\n%s", got)
	}
	if strings.Contains(got, "geocode") {
		t.Fatalf("maps function leaked:\n%s", got)
	}

	stdout, stderr, code = runFn("function", "list", "--type", "server", "--context", "billing")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got = plain(stdout)
	if !strings.Contains(got, "echo") {
		t.Fatalf("missing echo:\n%s", got)
	}
	if strings.Contains(got, "greet") {
		t.Fatalf("client leaked:\n%s", got)
	}
}

func TestFunctionGetByIDAndName(t *testing.T) {
	dir := t.TempDir()
	fake := newFunctionFake()
	fake.put("server", "abc123", "billing", "echo", map[string]any{"description": "Echo input"})
	srv := startFunctionAPI(t, fake)
	functionEnv(t, dir, srv.URL)

	stdout, stderr, code := runFn("function", "get", "abc123")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"id": "abc123"`) || !strings.Contains(stdout, "Echo input") {
		t.Fatalf("get by id:\n%s", stdout)
	}

	stdout, stderr, code = runFn("function", "get", "echo", "--context", "billing")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"name": "echo"`) {
		t.Fatalf("get by name:\n%s", stdout)
	}

	_, stderr, code = runFn("function", "get", "missing", "--context", "billing")
	if code != exitcode.Usage {
		t.Fatalf("missing name exit %d stderr=%s", code, stderr)
	}
}

func TestFunctionDeleteByNameCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	fake := newFunctionFake()
	fake.put("server", "abc123", "billing", "echo", nil)
	srv := startFunctionAPI(t, fake)
	functionEnv(t, dir, srv.URL)

	stdout, stderr, code := runFn("function", "delete", "ECHO", "--context", "Billing", "--type", "server")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(plain(stdout), "deleted") {
		t.Fatalf("stdout=%s", stdout)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.deleted) != 1 || fake.deleted[0] != "server/abc123" {
		t.Fatalf("deleted=%v", fake.deleted)
	}
}

func TestFunctionDeleteAmbiguousRequiresType(t *testing.T) {
	dir := t.TempDir()
	fake := newFunctionFake()
	fake.put("server", "s1", "billing", "echo", nil)
	fake.put("api", "a1", "billing", "echo", nil)
	srv := startFunctionAPI(t, fake)
	functionEnv(t, dir, srv.URL)

	_, stderr, code := runFn("function", "delete", "echo", "--context", "billing")
	if code != exitcode.Usage {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
	if !strings.Contains(strings.ToLower(stderr), "unclear") && !strings.Contains(strings.ToLower(stderr), "--type") {
		t.Fatalf("expected unclear/--type, stderr=%s", stderr)
	}
}

func TestFunctionAddParsesAndPosts(t *testing.T) {
	if !onPath("node") {
		t.Skip("node not on PATH")
	}
	dir := t.TempDir()
	fake := newFunctionFake()
	srv := startFunctionAPI(t, fake)
	functionEnv(t, dir, srv.URL)
	src := filepath.Join(dir, "echo.ts")
	if err := os.WriteFile(src, []byte("export function echo(text: string) { return text; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter := filepath.Join(fixturesDir(t), "adapter.js")

	stdout, stderr, code := runFn(
		"--adapter", "node "+adapter,
		"function", "add", "echo", src,
		"--type", "server",
		"--context", "billing",
		"--description", "Echo input",
		"--skip-generate",
	)
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout)
	if !strings.Contains(got, "deployed") || !strings.Contains(got, "fn-1") {
		t.Fatalf("stdout=%s", stdout)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.posts) != 1 {
		t.Fatalf("posts=%d", len(fake.posts))
	}
	post := fake.posts[0]
	if post["name"] != "echo" || post["context"] != "billing" || post["description"] != "Echo input" {
		t.Fatalf("post=%v", post)
	}
	if post["visibility"] != "ENVIRONMENT" {
		t.Fatalf("visibility=%v", post["visibility"])
	}
	codeStr, _ := post["code"].(string)
	if !strings.Contains(codeStr, "export function echo") {
		t.Fatalf("code=%v", post["code"])
	}
}

func TestFunctionAddRejectsClientLogsFlag(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	_, stderr, code := runFn("function", "add", "echo", "echo.ts", "--type", "client", "--logs", "enabled")
	if code != exitcode.Usage {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
	if !strings.Contains(strings.ToLower(stderr), "server") {
		t.Fatalf("stderr=%s", stderr)
	}
}

func TestFunctionAddRejectsAPIType(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	_, stderr, code := runFn("function", "add", "echo", "echo.ts", "--type", "api")
	if code != exitcode.Usage {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
}

func TestFunctionExecuteHTTPAndPositionalArgs(t *testing.T) {
	dir := t.TempDir()
	fake := newFunctionFake()
	fake.put("server", "abc123", "billing", "echo", map[string]any{
		"arguments": []any{map[string]any{"key": "text", "name": "text"}},
	})
	srv := startFunctionAPI(t, fake)
	functionEnv(t, dir, srv.URL)

	stdout, stderr, code := runFn("function", "execute", "echo", "--context", "billing", "--type", "server", "hello")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"text": "hello"`) {
		t.Fatalf("execute body:\n%s", stdout)
	}

	stdout, stderr, code = runFn("function", "execute", "abc123", "--data", `{"text":"hi"}`)
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"text": "hi"`) {
		t.Fatalf("data execute:\n%s", stdout)
	}
}

func TestFunctionExecuteClientIsUsage(t *testing.T) {
	dir := t.TempDir()
	fake := newFunctionFake()
	fake.put("client", "c1", "billing", "greet", nil)
	srv := startFunctionAPI(t, fake)
	functionEnv(t, dir, srv.URL)
	_, stderr, code := runFn("function", "execute", "greet", "--context", "billing", "--type", "client")
	if code != exitcode.Usage {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
	if !strings.Contains(strings.ToLower(stderr), "client") {
		t.Fatalf("stderr=%s", stderr)
	}
}

func TestFunctionLogsGetAndDelete(t *testing.T) {
	dir := t.TempDir()
	fake := newFunctionFake()
	fake.put("server", "abc123", "billing", "echo", nil)
	srv := startFunctionAPI(t, fake)
	functionEnv(t, dir, srv.URL)

	stdout, stderr, code := runFn("function", "logs", "abc123", "--keyword", "hello", "--limit", "5", "--last-hours", "24")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout)
	want := "ex1 • INFO • 2024-07-15T18:02:42Z • 00004\nhello\n\n"
	if got != want {
		t.Fatalf("logs:\n%s", got)
	}
	fake.mu.Lock()
	q := fake.lastLogsQuery
	fake.mu.Unlock()
	if !strings.Contains(q, "keyword=hello") || !strings.Contains(q, "limit=5") || !strings.Contains(q, "lastHours=24") {
		t.Fatalf("query not forwarded: %s", q)
	}

	stdout, stderr, code = runFn("function", "logs", "abc123", "--delete")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	found := false
	for _, d := range fake.deleted {
		if d == "server/abc123/logs" {
			found = true
		}
	}
	if !found {
		t.Fatalf("deleted=%v", fake.deleted)
	}
}

func TestFunctionLogsPrettyPrintsJSONAndSkipsBlankMeta(t *testing.T) {
	dir := t.TempDir()
	fake := newFunctionFake()
	fake.put("server", "abc123", "billing", "echo", nil)
	encoded, err := json.Marshal(map[string]any{"orderId": "o-1", "ok": true})
	if err != nil {
		t.Fatal(err)
	}
	nested, err := json.Marshal(map[string]any{"nested": true})
	if err != nil {
		t.Fatal(err)
	}
	doubleEncoded, err := json.Marshal(string(nested))
	if err != nil {
		t.Fatal(err)
	}
	fake.logs = map[string]any{
		"logsEnabled": true,
		"logs": []any{
			map[string]any{
				"timestamp":   "2024-07-15T18:02:42Z",
				"value":       `{"orderId":"o-1","items":[1,2]}`,
				"level":       "INFO",
				"executionId": "ex1",
				"revision":    "00004",
			},
			map[string]any{
				"timestamp":   "2024-07-15T18:03:00Z",
				"value":       string(encoded),
				"level":       "WARN",
				"executionId": "ex2",
			},
			map[string]any{
				"value": string(doubleEncoded),
				"level": "ERROR",
			},
			map[string]any{
				"timestamp":   "2024-07-15T18:04:00Z",
				"value":       "plain text",
				"level":       "DEBUG",
				"executionId": "ex3",
				"revision":    "",
			},
		},
	}
	srv := startFunctionAPI(t, fake)
	functionEnv(t, dir, srv.URL)

	stdout, stderr, code := runFn("function", "logs", "abc123")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout)
	blocks := []string{
		"ex1 • INFO • 2024-07-15T18:02:42Z • 00004\n{\n  \"items\": [\n    1,\n    2\n  ],\n  \"orderId\": \"o-1\"\n}\n",
		"ex2 • WARN • 2024-07-15T18:03:00Z\n{\n  \"ok\": true,\n  \"orderId\": \"o-1\"\n}\n",
		"ERROR\n{\n  \"nested\": true\n}\n",
		"ex3 • DEBUG • 2024-07-15T18:04:00Z\nplain text\n",
	}
	for _, block := range blocks {
		if !strings.Contains(got, block) {
			t.Fatalf("missing block %q in:\n%s", block, got)
		}
	}
}

func TestFunctionLogsEmpty(t *testing.T) {
	dir := t.TempDir()
	fake := newFunctionFake()
	fake.put("server", "abc123", "billing", "echo", nil)
	fake.logs = map[string]any{"logsEnabled": true, "logs": []any{}}
	srv := startFunctionAPI(t, fake)
	functionEnv(t, dir, srv.URL)

	stdout, stderr, code := runFn("function", "logs", "abc123")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if plain(stdout) != "no logs found\n" {
		t.Fatalf("empty logs:\n%s", stdout)
	}
}

func TestFunctionLogsRejectsClient(t *testing.T) {
	dir := t.TempDir()
	fake := newFunctionFake()
	fake.put("client", "c1", "billing", "greet", nil)
	srv := startFunctionAPI(t, fake)
	functionEnv(t, dir, srv.URL)
	_, stderr, code := runFn("function", "logs", "greet", "--context", "billing")
	if code != exitcode.Usage {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
}

func TestFunctionListMissingCredentialsIsAuth(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	_, stderr, code := runFn("function", "list")
	if code != exitcode.Auth {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
}

func TestFunctionUpdateIsRegistered(t *testing.T) {
	cmd, _, err := cli.NewRoot().Find([]string{"function", "update"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Name() != "update" {
		t.Fatalf("got %q", cmd.Name())
	}
	if cmd.Flags().Lookup("type") == nil || cmd.Flags().Lookup("skip-generate") == nil {
		t.Fatal("update should share add flags")
	}
}
