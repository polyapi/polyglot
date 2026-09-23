package api_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/polyapi/polyglot/src/api"
)

func TestForbiddenMatchesTSWording(t *testing.T) {
	err := api.Forbidden("generate Poly library", []string{"libraryGenerate"}, "")
	if err.Error() != "You don't have permission to generate Poly library. Missing permission: libraryGenerate." {
		t.Fatalf("%q", err.Error())
	}
	if err.ExitCode() != 3 {
		t.Fatalf("exit %d", err.ExitCode())
	}
}

func TestIsSafeResource(t *testing.T) {
	if api.IsSafeResource("https://evil.example/variables") {
		t.Fatal("absolute url")
	}
	if api.IsSafeResource("../secrets") {
		t.Fatal("dotdot")
	}
	if !api.IsSafeResource("functions/server") || !api.IsSafeResource("variables") {
		t.Fatal("expected safe")
	}
	for _, name := range []string{"variables", "tables", "webhooks", "jobs", "schemas", "triggers", "snippets", "applications", "functions/server"} {
		found := false
		for _, c := range api.Collections {
			if c == name {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing collection %s", name)
		}
	}
}

func TestUnwrapListAndNextPage(t *testing.T) {
	items, err := api.UnwrapList("u", []any{map[string]any{"id": "1"}})
	if err != nil || len(items) != 1 {
		t.Fatalf("%v %v", items, err)
	}
	items, err = api.UnwrapList("u", map[string]any{"data": []any{map[string]any{"id": "2"}}})
	if err != nil || items[0].(map[string]any)["id"] != "2" {
		t.Fatalf("%v %v", items, err)
	}
	next, ok := api.NextPage(map[string]any{"page": float64(0), "totalPages": float64(3), "data": []any{}})
	if !ok || next != 1 {
		t.Fatalf("%v %v", next, ok)
	}
	if _, ok := api.NextPage(map[string]any{"page": float64(2), "totalPages": float64(3)}); ok {
		t.Fatal("expected no next page")
	}
	next, ok = api.NextPage(map[string]any{
		"results":    []any{},
		"pagination": map[string]any{"page": float64(1), "pages": float64(3)},
	})
	if !ok || next != 2 {
		t.Fatalf("nested pagination next=%v ok=%v", next, ok)
	}
	if _, ok := api.NextPage(map[string]any{
		"pagination": map[string]any{"page": float64(3), "pages": float64(3)},
	}); ok {
		t.Fatal("expected no next nested page")
	}
}

func TestRequirePermissions(t *testing.T) {
	auth := api.AuthData{Permissions: map[string]bool{"libraryGenerate": true}}
	if err := api.RequirePermissions(auth, []api.PermissionReq{api.Requirement("libraryGenerate")}); err != nil {
		t.Fatal(err)
	}
	err := api.RequirePermissions(api.AuthData{}, []api.PermissionReq{api.Requirement("customDev")})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "add Client or Server functions") || !strings.Contains(msg, "customDev") {
		t.Fatalf("%q", msg)
	}
}

func TestMemoryCRUD(t *testing.T) {
	client := api.NewMemoryClient()
	created, err := client.Create("variables", map[string]any{"name": "x"})
	if err != nil {
		t.Fatal(err)
	}
	id := created.(map[string]any)["id"].(string)
	list, _ := client.List("variables")
	if len(list) != 1 {
		t.Fatalf("list %d", len(list))
	}
	if _, err := client.Update("variables", id, map[string]any{"name": "y"}, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := client.Get("variables", id)
	if got.(map[string]any)["name"] != "y" {
		t.Fatalf("%v", got)
	}
	if err := client.Delete("variables", id, nil); err != nil {
		t.Fatal(err)
	}
	list, _ = client.List("variables")
	if len(list) != 0 {
		t.Fatalf("list %d", len(list))
	}
}

func TestMemorySatisfiesClient(t *testing.T) {
	var _ api.Client = api.NewMemoryClient()
}

func testClient(t *testing.T, handler http.Handler) *api.HTTPClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := api.NewHTTPClient(srv.URL, "secret-key", api.Options{Retry: api.TestRetry()})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestListSendsBearerAndParsesArray(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/variables" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret-key" {
			t.Errorf("auth %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `[{"id":"1"}]`)
	}))
	items, err := c.List("variables")
	if err != nil {
		t.Fatal(err)
	}
	if items[0].(map[string]any)["id"] != "1" {
		t.Fatalf("%v", items)
	}
}

func TestCRUDMethodsHitExpectedPaths(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Path == "/variables/1":
			io.WriteString(w, `{"id":"1"}`)
		case r.Method == "POST" && r.URL.Path == "/variables":
			body, _ := io.ReadAll(r.Body)
			var m map[string]any
			json.Unmarshal(body, &m)
			if m["name"] != "x" {
				t.Errorf("body %s", body)
			}
			w.WriteHeader(201)
			io.WriteString(w, `{"id":"1","name":"x"}`)
		case r.Method == "PATCH" && r.URL.Path == "/variables/1":
			if r.Header.Get("x-otp") != "123456" {
				t.Errorf("otp %s", r.Header.Get("x-otp"))
			}
			io.WriteString(w, `{"id":"1","name":"y"}`)
		case r.Method == "DELETE" && r.URL.Path == "/variables/1":
			w.WriteHeader(204)
		default:
			http.NotFound(w, r)
		}
	}))
	got, err := c.Get("variables", "1")
	if err != nil || got.(map[string]any)["id"] != "1" {
		t.Fatalf("%v %v", got, err)
	}
	created, err := c.Create("variables", map[string]any{"name": "x"})
	if err != nil || created.(map[string]any)["name"] != "x" {
		t.Fatalf("%v %v", created, err)
	}
	updated, err := c.Update("variables", "1", map[string]any{"name": "y"}, map[string]string{"x-otp": "123456"})
	if err != nil || updated.(map[string]any)["name"] != "y" {
		t.Fatalf("%v %v", updated, err)
	}
	if err := c.Delete("variables", "1", nil); err != nil {
		t.Fatal(err)
	}
}

func TestUnauthorizedIsActionable(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		io.WriteString(w, `{"message":"nope"}`)
	}))
	_, err := c.Get("variables", "missing")
	ae, ok := err.(*api.Error)
	if !ok {
		t.Fatalf("%T %v", err, err)
	}
	if ae.Status() != 401 {
		t.Fatalf("status %d", ae.Status())
	}
	if !strings.Contains(ae.Error(), "polyapi auth login") {
		t.Fatalf("%v", ae)
	}
}

func TestRetries429ThenSucceeds(t *testing.T) {
	var n atomic.Int32
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `[]`)
	}))
	items, err := c.List("webhooks")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("%v", items)
	}
	if n.Load() != 2 {
		t.Fatalf("calls %d", n.Load())
	}
}

func TestListAllQueryMergesSearchAndPage(t *testing.T) {
	var queries []string
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		page := r.URL.Query().Get("page")
		if page == "" {
			io.WriteString(w, `{"results":[{"id":"1"}],"page":0,"totalPages":2}`)
			return
		}
		io.WriteString(w, `{"results":[{"id":"2"}],"page":1,"totalPages":2}`)
	}))
	items, err := c.ListAllQuery("functions/server", [][2]string{{"search", "billing.echo"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items=%v", items)
	}
	if len(queries) != 2 {
		t.Fatalf("queries=%v", queries)
	}
	if !strings.Contains(queries[0], "search=billing.echo") {
		t.Fatalf("missing search: %s", queries[0])
	}
	if !strings.Contains(queries[1], "page=1") || !strings.Contains(queries[1], "search=billing.echo") {
		t.Fatalf("page query: %s", queries[1])
	}
}

func TestListAllWalksNestedTablePagination(t *testing.T) {
	var pages []string
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pages = append(pages, r.URL.Query().Get("page"))
		w.Header().Set("Content-Type", "application/json")
		page := r.URL.Query().Get("page")
		switch page {
		case "", "1":
			io.WriteString(w, `{"results":[{"id":"t1"}],"pagination":{"page":1,"pages":2,"pageSize":20}}`)
		case "2":
			io.WriteString(w, `{"results":[{"id":"t2"}],"pagination":{"page":2,"pages":2,"pageSize":20}}`)
		default:
			t.Errorf("unexpected page %q", page)
			io.WriteString(w, `{"results":[],"pagination":{"page":3,"pages":2}}`)
		}
	}))
	items, err := c.ListAll("tables")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items=%v", items)
	}
	if items[0].(map[string]any)["id"] != "t1" || items[1].(map[string]any)["id"] != "t2" {
		t.Fatalf("items=%v", items)
	}
	if len(pages) != 2 {
		t.Fatalf("requests=%v", pages)
	}
}

func TestPostActionAcceptsPlainText2xx(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/webhooks/abc" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "Hello Weekly Report")
	}))
	got, err := c.PostAction("webhooks", "abc", "", map[string]any{"n": 5})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Hello Weekly Report" {
		t.Fatalf("got=%v", got)
	}
}

func TestPostActionAndLogsPaths(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && r.URL.Path == "/functions/server/abc/execute":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"text":"hi"`) {
				t.Errorf("body %s", body)
			}
			io.WriteString(w, `{"ok":true}`)
		case r.Method == "GET" && r.URL.Path == "/functions/server/abc/logs":
			if r.URL.Query().Get("limit") != "5" {
				t.Errorf("query %s", r.URL.RawQuery)
			}
			io.WriteString(w, `{"logs":[]}`)
		case r.Method == "DELETE" && r.URL.Path == "/functions/server/abc/logs":
			w.WriteHeader(204)
		default:
			http.NotFound(w, r)
		}
	}))
	got, err := c.PostAction("functions/server", "abc", "execute", map[string]any{"text": "hi"})
	if err != nil || got.(map[string]any)["ok"] != true {
		t.Fatalf("%v %v", got, err)
	}
	logs, err := c.GetQuery("functions/server", "abc/logs", [][2]string{{"limit", "5"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := logs.(map[string]any)["logs"]; !ok {
		t.Fatalf("%v", logs)
	}
	if err := c.DeleteAction("functions/server", "abc", "logs"); err != nil {
		t.Fatal(err)
	}
}

func TestNetworkErrorExitCode(t *testing.T) {
	c, err := api.NewHTTPClient("http://127.0.0.1:1", "k", api.Options{Retry: api.TestRetry()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Get("variables", "1")
	ae, ok := err.(*api.Error)
	if !ok {
		t.Fatalf("%T %v", err, err)
	}
	if ae.Kind != "network" {
		t.Fatalf("kind %s", ae.Kind)
	}
	if ae.ExitCode() != 4 {
		t.Fatalf("exit %d", ae.ExitCode())
	}
}

func TestRejectsAbsoluteResource(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not hit server")
	}))
	_, err := c.List("https://evil.example/variables")
	ae, ok := err.(*api.Error)
	if !ok || ae.Kind != "invalid_resource" {
		t.Fatalf("%v", err)
	}
}

func TestAuthParsesPermissions(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth" {
			t.Errorf("path %s", r.URL.Path)
		}
		io.WriteString(w, `{"tenant":{"id":"t"},"environment":{"id":"e"},"permissions":{"libraryGenerate":true}}`)
	}))
	auth, err := c.Auth()
	if err != nil {
		t.Fatal(err)
	}
	if auth.Tenant == nil || auth.Tenant.ID != "t" {
		t.Fatalf("%+v", auth)
	}
	if !auth.Permissions["libraryGenerate"] {
		t.Fatalf("%+v", auth.Permissions)
	}
}

func TestDebugDoesNotIncludeAPIKey(t *testing.T) {
	c := testClient(t, http.NotFoundHandler())
	s := fmt.Sprintf("%#v", c)
	if strings.Contains(s, "secret-key") {
		t.Fatalf("%s", s)
	}
}

func TestFunctionsServerCollection(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/functions/server" {
			t.Errorf("path %s", r.URL.Path)
		}
		io.WriteString(w, `[{"id":"fn1"}]`)
	}))
	items, err := c.List("functions/server")
	if err != nil {
		t.Fatal(err)
	}
	if items[0].(map[string]any)["id"] != "fn1" {
		t.Fatalf("%v", items)
	}
}

func TestPutAndPostText(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/functions/api":
			if r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("content-type %s", r.Header.Get("Content-Type"))
			}
			if !strings.Contains(string(body), `"name":"createPost"`) {
				t.Errorf("body %s", body)
			}
			io.WriteString(w, `{"id":"api-1","name":"createPost"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/specification-input/oas":
			if !strings.Contains(r.Header.Get("Content-Type"), "text/plain") {
				t.Errorf("content-type %s", r.Header.Get("Content-Type"))
			}
			if string(body) != "openapi: 3.0.0" {
				t.Errorf("quoted json? %s", body)
			}
			if r.URL.Query().Get("context") != "billing" || r.URL.Query().Get("hostUrlAsArgument") != "hostUrl" {
				t.Errorf("query %s", r.URL.RawQuery)
			}
			io.WriteString(w, `{"title":"t","functions":[],"webhooks":[],"schemas":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	got, err := c.Put("functions/api", map[string]any{"name": "createPost"})
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["id"] != "api-1" {
		t.Fatalf("%v", got)
	}
	spec, err := c.PostText("specification-input/oas", [][2]string{{"context", "billing"}, {"hostUrlAsArgument", "hostUrl"}}, "openapi: 3.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if spec.(map[string]any)["title"] != "t" {
		t.Fatalf("%v", spec)
	}
}

func TestRequestWaitBracketsHTTP(t *testing.T) {
	var starts, ends int
	api.SetRequestWait(func(start bool) {
		if start {
			starts++
		} else {
			ends++
		}
	})
	t.Cleanup(func() { api.SetRequestWait(nil) })
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `[{"id":"1"}]`)
	}))
	if _, err := c.List("variables"); err != nil {
		t.Fatal(err)
	}
	if starts != 1 || ends != 1 {
		t.Fatalf("starts=%d ends=%d", starts, ends)
	}
}

func TestUserMessagePrefersJSONMessage(t *testing.T) {
	err := api.FromStatus("POST", "http://x/specification-input/oas", 400, `{"message":"exactly one server"}`)
	if api.UserMessage(err) != "exactly one server" {
		t.Fatalf("%q", api.UserMessage(err))
	}
	netErr := &api.Error{Kind: "network", Msg: "network error: timeout"}
	if api.UserMessage(netErr) != "network error: timeout" {
		t.Fatalf("%q", api.UserMessage(netErr))
	}
}

func TestSpecsQueryAndDescribe(t *testing.T) {
	var specsQuery string
	var describePath string
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/specs"):
			specsQuery = r.URL.RawQuery
			io.WriteString(w, `[{"id":"abc","name":"echo","type":"serverFunction"}]`)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "description-generation"):
			describePath = r.URL.Path
			io.WriteString(w, `{"description":"says hello","arguments":[]}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	specs, err := c.Specs([]string{"billing"}, []string{"echo"}, []string{"abc"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 {
		t.Fatalf("%v", specs)
	}
	if !strings.Contains(specsQuery, "contexts=billing") || !strings.Contains(specsQuery, "names=echo") ||
		!strings.Contains(specsQuery, "ids=abc") || !strings.Contains(specsQuery, "noTypes=true") {
		t.Fatalf("query %q", specsQuery)
	}
	got, err := c.DescribeCustomFunction("client-function", map[string]any{"code": "x", "description": ""})
	if err != nil {
		t.Fatal(err)
	}
	if got["description"] != "says hello" {
		t.Fatalf("%v", got)
	}
	if !strings.Contains(describePath, "/functions/client/description-generation") {
		t.Fatalf("path %s", describePath)
	}
}
