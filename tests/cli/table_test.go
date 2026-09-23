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

type tableRecord struct {
	ID      string
	Name    string
	Context string
	Body    map[string]any
	Rows    []map[string]any
}

type tableFake struct {
	mu      sync.Mutex
	byID    map[string]tableRecord
	posts   []map[string]any
	patches []map[string]any
	actions []string
	bodies  []map[string]any
	deleted []string
}

func newTableFake() *tableFake {
	return &tableFake{byID: map[string]tableRecord{}}
}

func (f *tableFake) put(id, context, name string, extra map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body := map[string]any{
		"id":         id,
		"name":       name,
		"context":    context,
		"visibility": "ENVIRONMENT",
		"columns":    []any{},
	}
	for k, v := range extra {
		body[k] = v
	}
	f.byID[id] = tableRecord{ID: id, Name: name, Context: context, Body: body}
}

func (f *tableFake) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := strings.Trim(r.URL.Path, "/")
		parts := strings.Split(path, "/")
		if len(parts) == 0 || parts[0] != "tables" {
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
				items = append(items, map[string]any{
					"id": rec.ID, "name": rec.Name, "context": rec.Context,
					"visibility": rec.Body["visibility"], "description": rec.Body["description"],
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results":    items,
				"pagination": map[string]any{"page": 1, "pages": 1, "pageSize": 20},
			})
		case r.Method == http.MethodPost && len(parts) == 1:
			var payload map[string]any
			_ = json.Unmarshal(raw, &payload)
			f.posts = append(f.posts, payload)
			id := "tab-1"
			payload["id"] = id
			name, _ := payload["name"].(string)
			ctx, _ := payload["context"].(string)
			f.byID[id] = tableRecord{ID: id, Name: name, Context: ctx, Body: payload}
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
		case r.Method == http.MethodPost && len(parts) == 3:
			rec, ok := f.byID[parts[1]]
			if !ok {
				http.NotFound(w, r)
				return
			}
			var payload map[string]any
			_ = json.Unmarshal(raw, &payload)
			f.actions = append(f.actions, parts[2])
			f.bodies = append(f.bodies, payload)
			switch parts[2] {
			case "insert", "upsert":
				data, _ := payload["data"].([]any)
				for _, row := range data {
					m, _ := row.(map[string]any)
					if m["id"] == nil {
						m["id"] = "row-" + strings.ReplaceAll(r.URL.Path, "/", "")
					}
					rec.Rows = append(rec.Rows, m)
				}
				f.byID[parts[1]] = rec
				_ = json.NewEncoder(w).Encode(map[string]any{"results": data, "pagination": map[string]any{"page": 1, "pages": 1}})
			case "select":
				rows := rec.Rows
				if where, ok := payload["where"].(map[string]any); ok {
					var filtered []map[string]any
					for _, row := range rows {
						if tableRowMatches(row, where) {
							filtered = append(filtered, row)
						}
					}
					rows = filtered
				}
				out := make([]any, 0, len(rows))
				for _, row := range rows {
					out = append(out, row)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"results": out, "pagination": map[string]any{"page": 1, "pages": 1}})
			case "update":
				where, _ := payload["where"].(map[string]any)
				data, _ := payload["data"].(map[string]any)
				var updated []any
				for i, row := range rec.Rows {
					if where == nil || tableRowMatches(row, where) {
						for k, v := range data {
							row[k] = v
						}
						rec.Rows[i] = row
						updated = append(updated, row)
					}
				}
				f.byID[parts[1]] = rec
				_ = json.NewEncoder(w).Encode(map[string]any{"results": updated})
			case "delete":
				where, _ := payload["where"].(map[string]any)
				var keep []map[string]any
				n := 0
				for _, row := range rec.Rows {
					if where == nil || tableRowMatches(row, where) {
						n++
						continue
					}
					keep = append(keep, row)
				}
				rec.Rows = keep
				f.byID[parts[1]] = rec
				_ = json.NewEncoder(w).Encode(map[string]any{"deleted": n})
			case "count":
				where, _ := payload["where"].(map[string]any)
				n := 0
				for _, row := range rec.Rows {
					if where == nil || tableRowMatches(row, where) {
						n++
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"count": n})
			default:
				http.NotFound(w, r)
			}
		default:
			http.NotFound(w, r)
		}
	})
}

func tableRowMatches(row, where map[string]any) bool {
	for k, v := range where {
		if row[k] != v {
			return false
		}
	}
	return true
}

func startTableAPI(t *testing.T, fake *tableFake) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	return srv
}

func tableEnv(t *testing.T, dir, baseURL string) {
	t.Helper()
	isolateCLI(t, dir)
	t.Setenv("POLY_API_KEY", "test-key-xxxx")
	t.Setenv("POLY_API_BASE_URL", baseURL)
}

func runTable(args ...string) (string, string, int) {
	all := append([]string{"--non-interactive"}, args...)
	return run(all...)
}

func TestTableListFiltersByContextAndShowsVisibility(t *testing.T) {
	dir := t.TempDir()
	fake := newTableFake()
	fake.put("t1", "billing", "orders", map[string]any{"visibility": "ENVIRONMENT"})
	fake.put("t2", "maps", "places", map[string]any{"visibility": "TENANT"})
	srv := startTableAPI(t, fake)
	tableEnv(t, dir, srv.URL)

	stdout, stderr, code := runTable("table", "list", "--context", "billing")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout)
	if !strings.Contains(got, "NAME") || !strings.Contains(got, "VISIBILITY") || !strings.Contains(got, "ID") {
		t.Fatalf("expected column header:\n%s", got)
	}
	if !strings.Contains(got, "billing.orders") || !strings.Contains(got, "ENVIRONMENT") {
		t.Fatalf("expected billing table:\n%s", got)
	}
	if strings.Contains(got, "places") {
		t.Fatalf("maps leaked:\n%s", got)
	}
}

func TestTableGetByIDAndName(t *testing.T) {
	dir := t.TempDir()
	fake := newTableFake()
	fake.put("abc123", "billing", "orders", map[string]any{
		"description": "Order lines",
		"columns":     []any{map[string]any{"name": "sku", "type": "text"}},
	})
	srv := startTableAPI(t, fake)
	tableEnv(t, dir, srv.URL)

	stdout, stderr, code := runTable("table", "get", "abc123")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"name": "orders"`) || !strings.Contains(stdout, "sku") {
		t.Fatalf("get by id:\n%s", stdout)
	}

	stdout, stderr, code = runTable("table", "get", "orders", "--context", "billing")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"id": "abc123"`) {
		t.Fatalf("get by name:\n%s", stdout)
	}
}

func TestTableCreatePostsColumnsAndDefaultVisibility(t *testing.T) {
	dir := t.TempDir()
	fake := newTableFake()
	srv := startTableAPI(t, fake)
	tableEnv(t, dir, srv.URL)

	stdout, stderr, code := runTable("table", "create",
		"--name", "orders",
		"--context", "billing",
		"--description", "Order lines",
		"--columns", `[{"name":"sku","type":"string","required":true,"unique":true}]`,
	)
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout)
	if !strings.Contains(got, "created") || !strings.Contains(got, "billing.orders") {
		t.Fatalf("stdout=%s", stdout)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.posts) != 1 {
		t.Fatalf("posts=%d", len(fake.posts))
	}
	post := fake.posts[0]
	if post["name"] != "orders" || post["context"] != "billing" || post["visibility"] != "ENVIRONMENT" {
		t.Fatalf("post=%v", post)
	}
	cols, _ := post["columns"].([]any)
	col, _ := cols[0].(map[string]any)
	if col["name"] != "sku" || col["type"] != "string" {
		t.Fatalf("columns=%v", post["columns"])
	}
}

func TestTableCreateFromColumnsFile(t *testing.T) {
	dir := t.TempDir()
	fake := newTableFake()
	srv := startTableAPI(t, fake)
	tableEnv(t, dir, srv.URL)
	path := filepath.Join(dir, "cols.json")
	if err := os.WriteFile(path, []byte(`[{"name":"qty","type":"int","required":true}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runTable("table", "create", "--name", "orders", "--context", "billing", "--columns-file", path)
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	cols, _ := fake.posts[0]["columns"].([]any)
	col, _ := cols[0].(map[string]any)
	if col["type"] != "int" {
		t.Fatalf("columns=%v", fake.posts[0]["columns"])
	}
}

func TestTableCreateRequiresColumnsAndRejectsPublic(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	_, stderr, code := runTable("table", "create", "--name", "orders", "--context", "billing")
	if code != exitcode.Usage {
		t.Fatalf("missing columns exit %d stderr=%s", code, stderr)
	}

	fake := newTableFake()
	srv := startTableAPI(t, fake)
	tableEnv(t, dir, srv.URL)
	_, stderr, code = runTable("table", "create", "--name", "orders", "--context", "billing",
		"--columns", `[{"name":"sku","type":"text"}]`, "--visibility", "PUBLIC")
	if code != exitcode.Usage || !strings.Contains(strings.ToLower(stderr), "environment or tenant") {
		t.Fatalf("public visibility exit %d stderr=%s", code, stderr)
	}

	_, stderr, code = runTable("table", "create", "--name", "orders", "--context", "billing",
		"--columns", `[{"name":"sku","type":"xml"}]`)
	if code != exitcode.Usage || !strings.Contains(strings.ToLower(stderr), "unsupported type") {
		t.Fatalf("bad type exit %d stderr=%s", code, stderr)
	}
}

func TestTableUpdateAndDelete(t *testing.T) {
	dir := t.TempDir()
	fake := newTableFake()
	fake.put("abc123", "billing", "orders", map[string]any{"description": "old"})
	srv := startTableAPI(t, fake)
	tableEnv(t, dir, srv.URL)

	stdout, stderr, code := runTable("table", "update", "orders", "--context", "billing", "--description", "new", "--otp", "123456")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(plain(stdout), "updated") {
		t.Fatalf("stdout=%s", stdout)
	}
	fake.mu.Lock()
	if len(fake.patches) != 1 || fake.patches[0]["description"] != "new" {
		t.Fatalf("patches=%v", fake.patches)
	}
	fake.mu.Unlock()

	stdout, stderr, code = runTable("table", "delete", "ORDERS", "--context", "Billing")
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

func TestTableRowsInsertListGetUpdateDelete(t *testing.T) {
	dir := t.TempDir()
	fake := newTableFake()
	fake.put("abc123", "billing", "orders", nil)
	fake.mu.Lock()
	rec := fake.byID["abc123"]
	rec.Rows = []map[string]any{{"id": "r1", "sku": "A-1", "status": "open"}}
	fake.byID["abc123"] = rec
	fake.mu.Unlock()
	srv := startTableAPI(t, fake)
	tableEnv(t, dir, srv.URL)

	stdout, stderr, code := runTable("table", "rows", "insert", "orders", "--context", "billing", "--data", `{"sku":"B-2","status":"open"}`)
	if code != 0 {
		t.Fatalf("insert exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "B-2") {
		t.Fatalf("insert stdout=%s", stdout)
	}

	stdout, stderr, code = runTable("table", "rows", "list", "orders", "--context", "billing", "--where", `{"status":"open"}`)
	if code != 0 {
		t.Fatalf("list exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "A-1") || !strings.Contains(stdout, "B-2") {
		t.Fatalf("list stdout=%s", stdout)
	}

	stdout, stderr, code = runTable("table", "rows", "get", "orders", "r1", "--context", "billing")
	if code != 0 {
		t.Fatalf("get exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"sku": "A-1"`) {
		t.Fatalf("get stdout=%s", stdout)
	}

	stdout, stderr, code = runTable("table", "rows", "update", "orders", "--context", "billing", "--where", `{"sku":"A-1"}`, "--data", `{"status":"closed"}`)
	if code != 0 {
		t.Fatalf("update exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}

	stdout, stderr, code = runTable("table", "rows", "count", "orders", "--context", "billing", "--where", `{"status":"closed"}`)
	if code != 0 {
		t.Fatalf("count exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"count": 1`) {
		t.Fatalf("count stdout=%s", stdout)
	}

	stdout, stderr, code = runTable("table", "rows", "delete", "orders", "--context", "billing", "--where", `{"sku":"B-2"}`)
	if code != 0 {
		t.Fatalf("delete exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"deleted": 1`) {
		t.Fatalf("delete stdout=%s", stdout)
	}

	stdout, stderr, code = runTable("table", "rows", "query", "orders", "--context", "billing")
	if code != 0 {
		t.Fatalf("query exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"results"`) || !strings.Contains(stdout, "pagination") {
		t.Fatalf("query should print the full select body:\n%s", stdout)
	}

	stdout, stderr, code = runTable("table", "rows", "upsert", "orders", "--context", "billing", "--data", `{"sku":"A-1","status":"open"}`)
	if code != 0 {
		t.Fatalf("upsert exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.actions) == 0 || fake.actions[len(fake.actions)-1] != "upsert" {
		t.Fatalf("actions=%v", fake.actions)
	}
}

func TestTableRowsDeleteRequiresWhere(t *testing.T) {
	dir := t.TempDir()
	fake := newTableFake()
	fake.put("abc123", "billing", "orders", nil)
	srv := startTableAPI(t, fake)
	tableEnv(t, dir, srv.URL)
	_, stderr, code := runTable("table", "rows", "delete", "orders", "--context", "billing")
	if code != exitcode.Usage || !strings.Contains(strings.ToLower(stderr), "where") {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
}

func TestTableRowsListRequiresOrderByWithOffset(t *testing.T) {
	dir := t.TempDir()
	fake := newTableFake()
	fake.put("abc123", "billing", "orders", nil)
	srv := startTableAPI(t, fake)
	tableEnv(t, dir, srv.URL)
	_, stderr, code := runTable("table", "rows", "list", "orders", "--context", "billing", "--offset", "10")
	if code != exitcode.Usage || !strings.Contains(strings.ToLower(stderr), "order-by") {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
}
