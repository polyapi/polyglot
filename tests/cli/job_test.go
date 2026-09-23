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

type jobRecord struct {
	ID   string
	Name string
	Body map[string]any
}

type jobExecution struct {
	ID     string
	JobID  string
	Status string
	Body   map[string]any
}

type jobFake struct {
	mu         sync.Mutex
	byID       map[string]jobRecord
	functions  map[string]map[string]any
	executions map[string][]jobExecution
	posts      []map[string]any
	patches    []map[string]any
	triggers   []string
	deleted    []string
	execGets   []string
	execDels   []string
	listQuery  string
}

func newJobFake() *jobFake {
	return &jobFake{
		byID:       map[string]jobRecord{},
		functions:  map[string]map[string]any{},
		executions: map[string][]jobExecution{},
	}
}

func (f *jobFake) putJob(id, name string, extra map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body := map[string]any{
		"id":            id,
		"name":          name,
		"enabled":       true,
		"executionType": "sequential",
		"schedule":      map[string]any{"type": "periodical", "value": "0 3 * * *"},
	}
	for k, v := range extra {
		body[k] = v
	}
	f.byID[id] = jobRecord{ID: id, Name: name, Body: body}
}

func (f *jobFake) putFn(id, context, name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.functions[id] = map[string]any{"id": id, "name": name, "context": context}
}

func (f *jobFake) putExec(jobID, execID, status string, extra map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body := map[string]any{
		"id":          execID,
		"jobId":       jobID,
		"status":      status,
		"duration":    12,
		"processedOn": "2026-09-22T03:00:00Z",
		"finishedOn":  "2026-09-22T03:00:12Z",
	}
	for k, v := range extra {
		body[k] = v
	}
	f.executions[jobID] = append(f.executions[jobID], jobExecution{ID: execID, JobID: jobID, Status: status, Body: body})
}

func (f *jobFake) handler() http.Handler {
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
		case len(parts) == 0 || parts[0] != "jobs":
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
			id := "job-1"
			payload["id"] = id
			name, _ := payload["name"].(string)
			f.byID[id] = jobRecord{ID: id, Name: name, Body: payload}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(payload)
		case r.Method == http.MethodPost && len(parts) == 3 && parts[2] == "trigger":
			if _, ok := f.byID[parts[1]]; !ok {
				http.NotFound(w, r)
				return
			}
			f.triggers = append(f.triggers, parts[1])
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jobId":    parts[1],
				"runId":    "run-1",
				"queuedAt": "2026-09-22T12:00:00Z",
			})
		case r.Method == http.MethodGet && len(parts) == 3 && parts[2] == "executions":
			f.listQuery = r.URL.RawQuery
			items := make([]any, 0)
			for _, ex := range f.executions[parts[1]] {
				items = append(items, ex.Body)
			}
			_ = json.NewEncoder(w).Encode(items)
		case r.Method == http.MethodGet && len(parts) == 4 && parts[2] == "executions":
			f.execGets = append(f.execGets, parts[3])
			for _, ex := range f.executions[parts[1]] {
				if ex.ID == parts[3] {
					_ = json.NewEncoder(w).Encode(ex.Body)
					return
				}
			}
			http.NotFound(w, r)
		case r.Method == http.MethodDelete && len(parts) == 3 && parts[2] == "executions":
			f.execDels = append(f.execDels, parts[1]+":all")
			delete(f.executions, parts[1])
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && len(parts) == 4 && parts[2] == "executions":
			f.execDels = append(f.execDels, parts[3])
			kept := f.executions[parts[1]][:0]
			for _, ex := range f.executions[parts[1]] {
				if ex.ID != parts[3] {
					kept = append(kept, ex)
				}
			}
			f.executions[parts[1]] = kept
			w.WriteHeader(http.StatusNoContent)
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

func startJobAPI(t *testing.T, fake *jobFake) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	return srv
}

func runJob(args ...string) (string, string, int) {
	all := append([]string{"--non-interactive"}, args...)
	return run(all...)
}

func TestJobListShowsScheduleAndEnabled(t *testing.T) {
	dir := t.TempDir()
	fake := newJobFake()
	fake.putJob("j1", "nightly", map[string]any{"enabled": true, "schedule": map[string]any{"type": "periodical", "value": "0 3 * * *"}})
	fake.putJob("j2", "heartbeat", map[string]any{"enabled": false, "schedule": map[string]any{"type": "interval", "value": 5}})
	srv := startJobAPI(t, fake)
	webhookEnv(t, dir, srv.URL)

	stdout, stderr, code := runJob("job", "list")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout)
	if !strings.Contains(got, "NAME") || !strings.Contains(got, "ENABLED") || !strings.Contains(got, "SCHEDULE") || !strings.Contains(got, "ID") {
		t.Fatalf("expected header:\n%s", got)
	}
	if !strings.Contains(got, "nightly") || !strings.Contains(got, "0 3 * * *") {
		t.Fatalf("expected nightly cron:\n%s", got)
	}
	if !strings.Contains(got, "heartbeat") || !strings.Contains(got, "every 5 min") {
		t.Fatalf("expected heartbeat interval:\n%s", got)
	}
}

func TestJobCreateResolvesFunctionAndScheduleTypes(t *testing.T) {
	dir := t.TempDir()
	fake := newJobFake()
	fake.putFn("fn-1", "billing", "weeklyReport")
	srv := startJobAPI(t, fake)
	webhookEnv(t, dir, srv.URL)

	stdout, stderr, code := runJob("job", "create",
		"--name", "nightly",
		"--cron", "0 3 * * *",
		"--function", "billing.weeklyReport",
	)
	if code != 0 {
		t.Fatalf("cron create exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(plain(stdout), "created") || !strings.Contains(plain(stdout), "nightly") {
		t.Fatalf("stdout=%s", stdout)
	}
	fake.mu.Lock()
	if len(fake.posts) != 1 {
		fake.mu.Unlock()
		t.Fatalf("posts=%d", len(fake.posts))
	}
	post := fake.posts[0]
	fake.mu.Unlock()
	if post["executionType"] != "sequential" || post["enabled"] != true {
		t.Fatalf("defaults=%v", post)
	}
	sch, _ := post["schedule"].(map[string]any)
	if sch["type"] != "periodical" || sch["value"] != "0 3 * * *" {
		t.Fatalf("schedule=%v", sch)
	}
	fns, _ := post["functions"].([]any)
	fn, _ := fns[0].(map[string]any)
	if fn["id"] != "fn-1" {
		t.Fatalf("functions=%v", post["functions"])
	}

	stdout, stderr, code = runJob("job", "create",
		"--name", "heartbeat",
		"--interval", "5",
		"--function", "billing.weeklyReport",
		"--execution-type", "parallel",
		"--enabled=false",
	)
	if code != 0 {
		t.Fatalf("interval create exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.posts) != 2 {
		t.Fatalf("posts=%d", len(fake.posts))
	}
	post = fake.posts[1]
	if post["executionType"] != "parallel" || post["enabled"] != false {
		t.Fatalf("post=%v", post)
	}
	sch, _ = post["schedule"].(map[string]any)
	if sch["type"] != "interval" {
		t.Fatalf("interval schedule=%v", sch)
	}
	if formatJobTestNumber(sch["value"]) != "5" {
		t.Fatalf("interval value=%v", sch["value"])
	}
}

func TestJobCreateRequiresNameAndFunction(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	_, stderr, code := runJob("job", "create")
	if code != exitcode.Usage {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
	_, stderr, code = runJob("job", "create", "--name", "nightly")
	if code != exitcode.Usage {
		t.Fatalf("missing function exit %d stderr=%s", code, stderr)
	}
}

func TestJobGetUpdateEnableDisableRunAndDelete(t *testing.T) {
	dir := t.TempDir()
	fake := newJobFake()
	fake.putJob("abc123", "nightly", nil)
	fake.putFn("fn-1", "billing", "weeklyReport")
	srv := startJobAPI(t, fake)
	webhookEnv(t, dir, srv.URL)

	stdout, stderr, code := runJob("job", "get", "nightly")
	if code != 0 {
		t.Fatalf("get exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"id": "abc123"`) {
		t.Fatalf("get=%s", stdout)
	}

	stdout, stderr, code = runJob("job", "update", "nightly", "--cron", "0 4 * * *")
	if code != 0 {
		t.Fatalf("update exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	fake.mu.Lock()
	if len(fake.patches) != 1 {
		fake.mu.Unlock()
		t.Fatalf("patches=%v", fake.patches)
	}
	sch, _ := fake.patches[0]["schedule"].(map[string]any)
	if sch["value"] != "0 4 * * *" {
		fake.mu.Unlock()
		t.Fatalf("patch schedule=%v", fake.patches[0])
	}
	fake.mu.Unlock()

	stdout, stderr, code = runJob("job", "disable", "NIGHTLY")
	if code != 0 {
		t.Fatalf("disable exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	fake.mu.Lock()
	if fake.patches[len(fake.patches)-1]["enabled"] != false {
		fake.mu.Unlock()
		t.Fatalf("disable patch=%v", fake.patches)
	}
	fake.mu.Unlock()

	stdout, stderr, code = runJob("job", "enable", "nightly")
	if code != 0 {
		t.Fatalf("enable exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}

	stdout, stderr, code = runJob("job", "run", "nightly")
	if code != 0 {
		t.Fatalf("run exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"runId": "run-1"`) {
		t.Fatalf("run stdout=%s", stdout)
	}
	fake.mu.Lock()
	if len(fake.triggers) != 1 || fake.triggers[0] != "abc123" {
		fake.mu.Unlock()
		t.Fatalf("triggers=%v", fake.triggers)
	}
	fake.mu.Unlock()

	stdout, stderr, code = runJob("job", "delete", "nightly")
	if code != 0 {
		t.Fatalf("delete exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.deleted) != 1 || fake.deleted[0] != "abc123" {
		t.Fatalf("deleted=%v", fake.deleted)
	}
}

func TestJobExecutionsListGetAndDelete(t *testing.T) {
	dir := t.TempDir()
	fake := newJobFake()
	fake.putJob("abc123", "nightly", nil)
	fake.putExec("abc123", "ex-1", "finished", nil)
	fake.putExec("abc123", "ex-2", "job_error", map[string]any{"duration": 3})
	srv := startJobAPI(t, fake)
	webhookEnv(t, dir, srv.URL)

	stdout, stderr, code := runJob("job", "executions", "list", "nightly", "--status", "job_error", "--last-days", "1", "--limit", "10")
	if code != 0 {
		t.Fatalf("list exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout)
	if !strings.Contains(got, "STATUS") || !strings.Contains(got, "ex-1") {
		t.Fatalf("list=%s", got)
	}
	fake.mu.Lock()
	q := fake.listQuery
	fake.mu.Unlock()
	if !strings.Contains(q, "status=job_error") || !strings.Contains(q, "lastDays=1") || !strings.Contains(q, "limit=10") {
		t.Fatalf("query=%s", q)
	}

	stdout, stderr, code = runJob("job", "executions", "get", "nightly", "ex-2")
	if code != 0 {
		t.Fatalf("get exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"id": "ex-2"`) {
		t.Fatalf("get=%s", stdout)
	}

	stdout, stderr, code = runJob("job", "executions", "delete", "nightly", "ex-1")
	if code != 0 {
		t.Fatalf("delete one exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	stdout, stderr, code = runJob("job", "executions", "delete", "nightly", "--all")
	if code != 0 {
		t.Fatalf("delete all exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.execDels) != 2 || fake.execDels[0] != "ex-1" || fake.execDels[1] != "abc123:all" {
		t.Fatalf("execDels=%v", fake.execDels)
	}
}

func formatJobTestNumber(v any) string {
	switch n := v.(type) {
	case float64:
		if n == float64(int64(n)) {
			return strings.TrimSuffix(strings.TrimSuffix(jsonNumber(n), ".0"), "")
		}
	case json.Number:
		return n.String()
	}
	s, _ := json.Marshal(v)
	return strings.Trim(string(s), `"`)
}

func jsonNumber(n float64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
