package glide_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/polyapi/polyglot/src/api"
	"github.com/polyapi/polyglot/src/config"
	"github.com/polyapi/polyglot/src/delegate"
	"github.com/polyapi/polyglot/src/glide"
)

func writeJSON(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func engine(root string, client api.Client) *glide.Engine {
	return &glide.Engine{
		Root:           root,
		PolyPath:       ".poly",
		Client:         client,
		Env:            map[string]string{"BILLING_API_KEY": "secret-value"},
		Scope:          glide.MergeScope(nil, nil, nil, nil, nil),
		HasCredentials: client != nil,
		PushAllowed:    true,
		EvalStatus:     "ok",
	}
}

func TestFindIgnoresPolyConfigMentionsAndTestDirs(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, root, "tests/fixtures/delegate/adapter.js", `reply(true, { export: "polyConfig", file: "src/x.ts" });
`)
	p := filepath.Join(root, "src", "billing", "vari", "apiKey.ts")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("export const polyConfig = { name: 'apiKey', context: 'billing' };\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mention := filepath.Join(root, "src", "util.ts")
	if err := os.WriteFile(mention, []byte("export const hint = 'see polyConfig in the SDK';\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	found, err := glide.Find(root, glide.MergeScope(nil, nil, nil, nil, nil), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Rel != "src/billing/vari/apiKey.ts" {
		t.Fatalf("%+v", found)
	}
}

func TestFindJSONCAndSkipNoise(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, root, "package.json", `{"name":"app"}`)
	writeJSON(t, root, "src/billing/artifacts/vari/apiKey.json", `{
  "name": "apiKey",
  "context": "billing",
  "value": "{{BILLING_API_KEY}}",
  "description": "key"
}`)
	writeJSON(t, root, "src/experiments/skip.json", `{
  "name": "skipMe",
  "context": "billing",
  "value": "x"
}`)
	scope := glide.MergeScope(nil, nil, []string{"src/experiments"}, nil, nil)
	found, err := glide.Find(root, scope, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Rel != "src/billing/artifacts/vari/apiKey.json" {
		t.Fatalf("%+v", found)
	}
}

func TestStripJSONC(t *testing.T) {
	got := glide.StripJSONC(`{ // hi
  "a": "http://x", /* block */
  "b": 1
}`)
	if strings.Contains(got, "hi") || strings.Contains(got, "block") {
		t.Fatalf("%q", got)
	}
	if !strings.Contains(got, "http://x") {
		t.Fatalf("%q", got)
	}
}

func TestValidateEnvAndCrossRef(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, root, "src/artifacts/vari/apiKey.json", `{
  "name": "apiKey",
  "context": "billing",
  "value": "{{MISSING_TOKEN}}",
  "description": "key"
}`)
	writeJSON(t, root, "src/artifacts/webhooks/hook.json", `{
  "name": "hook",
  "context": "billing",
  "description": "wh",
  "securityFunctions": [{"id": "billing.missingFn"}]
}`)
	eng := engine(root, nil)
	rep, err := eng.Validate()
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Failed(false) {
		t.Fatalf("expected errors: %+v", rep.Issues)
	}
	var sawEnv, sawRef, sawDesc bool
	for _, iss := range rep.Issues {
		if iss.Category == glide.CatEnv && strings.Contains(iss.Message, "MISSING_TOKEN") {
			sawEnv = true
		}
		if iss.Category == glide.CatCrossResource && strings.Contains(iss.Message, "billing.missingFn") {
			sawRef = true
		}
		if iss.Category == glide.CatMetadata && strings.Contains(iss.Message, "description") {
			sawDesc = true
		}
	}
	if !sawEnv || !sawRef {
		t.Fatalf("env=%v ref=%v issues=%+v", sawEnv, sawRef, rep.Issues)
	}
	_ = sawDesc
}

func TestValidateStrictMetadata(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, root, "src/artifacts/vari/apiKey.json", `{
  "name": "apiKey",
  "context": "billing",
  "value": "x"
}`)
	eng := engine(root, nil)
	rep, err := eng.Validate()
	if err != nil {
		t.Fatal(err)
	}
	if rep.Failed(false) {
		t.Fatalf("warnings should not fail: %+v", rep.Issues)
	}
	if !rep.Failed(true) {
		t.Fatal("strict should fail on missing description")
	}
}

func TestPlanPushSkipAndOrphan(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, root, "src/artifacts/vari/apiKey.json", `{
  "name": "apiKey",
  "context": "billing",
  "value": "from-source",
  "description": "key"
}`)
	writeJSON(t, root, "src/artifacts/webhooks/hook.json", `{
  "name": "hook",
  "context": "billing",
  "description": "wh",
  "securityFunctions": [{"id": "billing.handler"}]
}`)
	writeJSON(t, root, "src/artifacts/jobs/nightly.json", `{
  "name": "nightly",
  "functions": [{"functionContext": "billing", "functionName": "handler"}],
  "executionType": "sequential",
  "schedule": {"type": "periodical", "value": "0 3 * * *"},
  "description": "job"
}`)
	writeJSON(t, root, "src/server/handler.json", `{
  "type": "server-function",
  "name": "handler",
  "context": "billing",
  "code": "return 1;",
  "description": "fn"
}`)
	client := api.NewMemoryClient()
	eng := engine(root, client)

	plan, err := eng.Plan()
	if err != nil {
		t.Fatal(err)
	}
	creates := 0
	for _, r := range plan {
		if r.Action == glide.ActionWouldCreate {
			creates++
		}
	}
	if creates < 4 {
		t.Fatalf("plan %+v", plan)
	}

	push, err := eng.Push()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range push {
		if r.Action != glide.ActionCreate && r.Action != glide.ActionSkip {
			t.Fatalf("first push %+v", push)
		}
	}

	again, err := eng.Push()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range again {
		if r.Action != glide.ActionSkip && r.Action != glide.ActionOrphan {
			t.Fatalf("second push want skip, got %+v", again)
		}
	}

	if err := os.Remove(filepath.Join(root, "src/artifacts/vari/apiKey.json")); err != nil {
		t.Fatal(err)
	}
	eng.DeleteOrphans = true
	after, err := eng.Plan()
	if err != nil {
		t.Fatal(err)
	}
	sawDelete := false
	for _, r := range after {
		if r.Type == glide.TypeVariable && r.Name == "apiKey" && r.Action == glide.ActionWouldDelete {
			sawDelete = true
		}
	}
	if !sawDelete {
		t.Fatalf("expected would_delete for removed variable: %+v", after)
	}
}

func TestPushBlockedWithoutTarget(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref: refs/heads/feature/x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, root, "src/artifacts/vari/apiKey.json", `{
  "name": "apiKey",
  "context": "billing",
  "value": "x",
  "description": "key"
}`)
	eval := config.EvaluateDeploy(root, &config.DeployFile{
		UnallowedBranch: "error",
		Targets: []config.DeployTargetFile{{
			Branches:    []string{"main"},
			Environment: "prod",
		}},
	}, "")
	eng := engine(root, api.NewMemoryClient())
	eng.PushAllowed = eval.Allowed
	eng.EvalStatus = eval.Status
	eng.EvalMessage = eval.Message
	if _, err := eng.Push(); err == nil || !strings.Contains(err.Error(), "deploy.targets") {
		t.Fatalf("err=%v", err)
	}
}

func TestPullDryRunJSON(t *testing.T) {
	root := t.TempDir()
	client := api.NewMemoryClient()
	_, err := client.Create("variables", map[string]any{
		"name": "apiKey", "context": "billing", "value": "from-remote", "description": "key",
	})
	if err != nil {
		t.Fatal(err)
	}
	eng := engine(root, client)
	eng.DryRun = true
	results, err := eng.Pull()
	if err != nil {
		t.Fatal(err)
	}
	saw := false
	for _, r := range results {
		if r.Type == glide.TypeVariable && r.Action == glide.ActionWouldCreate {
			saw = true
			if strings.Contains(r.File, "..") {
				t.Fatalf("path %s", r.File)
			}
		}
	}
	if !saw {
		t.Fatalf("%+v", results)
	}
	if _, err := os.Stat(filepath.Join(root, "src/billing/artifacts/vari/apiKey.json")); err == nil {
		t.Fatal("dry-run wrote a file")
	}
}

func TestPushBlanksSecretVariableValues(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, root, "src/artifacts/vari/apiKey.json", `{
  "name": "apiKey",
  "context": "billing",
  "value": "super-secret-from-repo",
  "secrecy": "SECRET",
  "description": "key"
}`)
	writeJSON(t, root, "src/artifacts/vari/token.json", `{
  "name": "token",
  "context": "billing",
  "value": "obscured-ok",
  "secrecy": "OBSCURED",
  "description": "tok"
}`)
	client := api.NewMemoryClient()
	eng := engine(root, client)
	if _, err := eng.Push(); err != nil {
		t.Fatal(err)
	}
	rows, err := client.List("variables")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]any{}
	for _, row := range rows {
		m, _ := row.(map[string]any)
		got[m["name"].(string)] = m["value"]
	}
	secretVal, _ := got["apiKey"].(map[string]any)
	if len(secretVal) != 0 {
		t.Fatalf("SECRET value deployed from source: %v", got["apiKey"])
	}
	if got["token"] != "obscured-ok" {
		t.Fatalf("OBSCURED should keep value: %v", got["token"])
	}
}

func TestOptionalReceiptsWriteComment(t *testing.T) {
	root := t.TempDir()
	rel := "src/artifacts/vari/apiKey.json"
	writeJSON(t, root, rel, `{
  "name": "apiKey",
  "context": "billing",
  "value": "x",
  "description": "key"
}`)
	eng := engine(root, api.NewMemoryClient())
	eng.Receipts = true
	if _, err := eng.Push(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Poly deployed @") {
		t.Fatalf("%s", raw)
	}
}

func TestCodeWithoutAdapterIsDiscoveryError(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "src", "billing", "vari", "apiKey.ts")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("export const polyConfig = { name: 'apiKey', context: 'billing' };\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	eng := engine(root, nil)
	rep, err := eng.Validate()
	if err != nil {
		t.Fatal(err)
	}
	saw := false
	for _, iss := range rep.Issues {
		if iss.Category == glide.CatDiscovery && strings.Contains(iss.Message, "adapter") {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("%+v", rep.Issues)
	}
}

func TestNothingFoundIsAnError(t *testing.T) {
	root := t.TempDir()
	eng := engine(root, api.NewMemoryClient())
	if _, err := eng.Validate(); !glide.IsNothingFound(err) {
		t.Fatalf("validate err=%v", err)
	}
	if _, _, err := eng.Prepare(); !glide.IsNothingFound(err) {
		t.Fatalf("prepare err=%v", err)
	}
	if _, err := eng.Plan(); !glide.IsNothingFound(err) {
		t.Fatalf("plan err=%v", err)
	}
	if _, err := eng.Push(); !glide.IsNothingFound(err) {
		t.Fatalf("push err=%v", err)
	}
	if _, err := eng.Pull(); !glide.IsNothingFound(err) {
		t.Fatalf("pull err=%v", err)
	}
}

func TestPrepareJSONCOnlyIsNotNothingFound(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, root, "src/artifacts/vari/apiKey.json", `{
  "name": "apiKey",
  "context": "billing",
  "value": "x",
  "description": "key"
}`)
	_, skipped, err := engine(root, nil).Prepare()
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 1 {
		t.Fatalf("skipped=%v", skipped)
	}
}

func TestTableGlideIsSchemaOnlyAndSkipsSeedRows(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, root, "src/billing/artifacts/tabi/orders.json", `{
  "name": "orders",
  "context": "billing",
  "description": "Order lines",
  "visibility": "ENVIRONMENT",
  "columns": [
    {"name": "id", "type": "uuid", "primary": true},
    {"name": "sku", "type": "text", "required": true}
  ],
  "rows": [{"sku": "should-not-deploy"}],
  "seed": [{"sku": "nope"}]
}`)
	client := api.NewMemoryClient()
	eng := engine(root, client)
	push, err := eng.Push()
	if err != nil {
		t.Fatal(err)
	}
	if len(push) != 1 || push[0].Action != glide.ActionCreate {
		t.Fatalf("push %+v", push)
	}
	got, err := client.Get("tables", push[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	m := got.(map[string]any)
	if _, ok := m["rows"]; ok {
		t.Fatalf("seed rows leaked: %v", m)
	}
	if _, ok := m["seed"]; ok {
		t.Fatalf("seed leaked: %v", m)
	}
	cols, _ := m["columns"].([]any)
	if len(cols) != 1 {
		t.Fatalf("managed id column should be stripped: %v", cols)
	}
	col, _ := cols[0].(map[string]any)
	if col["name"] != "sku" {
		t.Fatalf("columns=%v", cols)
	}

	again, err := eng.Push()
	if err != nil {
		t.Fatal(err)
	}
	if again[0].Action != glide.ActionSkip {
		t.Fatalf("second push %+v", again)
	}
}

func TestTableValidateRequiresColumns(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, root, "src/billing/artifacts/tabi/orders.json", `{
  "name": "orders",
  "context": "billing",
  "description": "Order lines"
}`)
	rep, err := engine(root, nil).Validate()
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Failed(false) {
		t.Fatalf("expected missing columns: %+v", rep.Issues)
	}
	saw := false
	for _, iss := range rep.Issues {
		if iss.Category == glide.CatMetadata && strings.Contains(iss.Message, "columns") {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("%+v", rep.Issues)
	}
}

func TestWebhookAndTriggerGlideRewriteRefs(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, root, "src/server/handler.json", `{
  "type": "server-function",
  "name": "handler",
  "context": "billing",
  "code": "return true;",
  "description": "fn"
}`)
	writeJSON(t, root, "src/artifacts/webhooks/hook.json", `{
  "name": "hook",
  "context": "billing",
  "description": "wh",
  "url": "https://should-not-push.example/webhooks/x",
  "securityFunctions": [{"id": "billing.handler", "message": "nope"}]
}`)
	writeJSON(t, root, "src/artifacts/triggers/weekly.json", `{
  "name": "weekly",
  "source": {"webhookContext": "billing", "webhookName": "hook"},
  "destination": {"functionContext": "billing", "functionName": "handler"},
  "waitForResponse": true,
  "description": "tr"
}`)
	client := api.NewMemoryClient()
	eng := engine(root, client)
	push, err := eng.Push()
	if err != nil {
		t.Fatal(err)
	}
	var hookID, fnID, trID string
	for _, r := range push {
		if r.Action != glide.ActionCreate && r.Action != glide.ActionSkip {
			t.Fatalf("push %+v", push)
		}
		switch r.Type {
		case glide.TypeWebhook:
			hookID = r.ID
		case glide.TypeServerFunction:
			fnID = r.ID
		case glide.TypeTrigger:
			trID = r.ID
		}
	}
	if hookID == "" || fnID == "" || trID == "" {
		t.Fatalf("missing ids %+v", push)
	}
	wh, err := client.Get("webhooks", hookID)
	if err != nil {
		t.Fatal(err)
	}
	wm := wh.(map[string]any)
	if _, ok := wm["url"]; ok {
		t.Fatalf("url leaked: %v", wm)
	}
	fns, _ := wm["securityFunctions"].([]any)
	sf, _ := fns[0].(map[string]any)
	if sf["id"] != fnID {
		t.Fatalf("securityFunctions=%v want %s", fns, fnID)
	}
	tr, err := client.Get("triggers", trID)
	if err != nil {
		t.Fatal(err)
	}
	tm := tr.(map[string]any)
	src, _ := tm["source"].(map[string]any)
	dest, _ := tm["destination"].(map[string]any)
	if src["webhookHandleId"] != hookID {
		t.Fatalf("source=%v", src)
	}
	if dest["serverFunctionId"] != fnID {
		t.Fatalf("destination=%v", dest)
	}

	again, err := eng.Push()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range again {
		if r.Action != glide.ActionSkip && r.Action != glide.ActionOrphan {
			t.Fatalf("second push %+v", again)
		}
	}
}

func TestErrorHandlerTriggerGlidePush(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, root, "src/server/handler.json", `{
  "type": "server-function",
  "name": "handler",
  "context": "billing",
  "code": "return true;",
  "description": "fn"
}`)
	writeJSON(t, root, "src/artifacts/triggers/on-error.json", `{
  "name": "on-error",
  "source": {"errorHandler": {"path": "poly.error"}},
  "destination": {"functionContext": "billing", "functionName": "handler"}
}`)
	client := api.NewMemoryClient()
	eng := engine(root, client)
	push, err := eng.Push()
	if err != nil {
		t.Fatal(err)
	}
	var fnID, trID string
	for _, r := range push {
		if r.Action != glide.ActionCreate && r.Action != glide.ActionSkip {
			t.Fatalf("push %+v", push)
		}
		switch r.Type {
		case glide.TypeServerFunction:
			fnID = r.ID
		case glide.TypeTrigger:
			trID = r.ID
		}
	}
	if fnID == "" || trID == "" {
		t.Fatalf("missing ids %+v", push)
	}
	tr, err := client.Get("triggers", trID)
	if err != nil {
		t.Fatal(err)
	}
	tm := tr.(map[string]any)
	if _, ok := tm["waitForResponse"]; ok {
		t.Fatalf("error-handler should omit waitForResponse: %v", tm)
	}
	src, _ := tm["source"].(map[string]any)
	eh, _ := src["errorHandler"].(map[string]any)
	if eh["path"] != "poly.error" {
		t.Fatalf("source=%v", src)
	}
	dest, _ := tm["destination"].(map[string]any)
	if dest["serverFunctionId"] != fnID {
		t.Fatalf("destination=%v want %s", dest, fnID)
	}

	again, err := eng.Push()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range again {
		if r.Action != glide.ActionSkip && r.Action != glide.ActionOrphan {
			t.Fatalf("second push %+v", again)
		}
	}
}

func TestTriggerSourceKindChangeFailsUpdate(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, root, "src/server/handler.json", `{
  "type": "server-function",
  "name": "handler",
  "context": "billing",
  "code": "return true;",
  "description": "fn"
}`)
	writeJSON(t, root, "src/artifacts/webhooks/hook.json", `{
  "name": "hook",
  "context": "billing",
  "description": "wh"
}`)
	writeJSON(t, root, "src/artifacts/triggers/weekly.json", `{
  "name": "weekly",
  "source": {"webhookContext": "billing", "webhookName": "hook"},
  "destination": {"functionContext": "billing", "functionName": "handler"},
  "waitForResponse": true
}`)
	client := api.NewMemoryClient()
	eng := engine(root, client)
	if _, err := eng.Push(); err != nil {
		t.Fatal(err)
	}

	writeJSON(t, root, "src/artifacts/triggers/weekly.json", `{
  "name": "weekly",
  "source": {"errorHandler": {"path": "poly.error"}},
  "destination": {"functionContext": "billing", "functionName": "handler"}
}`)
	again, err := eng.Push()
	if err != nil {
		t.Fatal(err)
	}
	var saw bool
	for _, r := range again {
		if r.Type != glide.TypeTrigger {
			continue
		}
		saw = true
		if r.Action != glide.ActionFailed {
			t.Fatalf("expected failed trigger update %+v", again)
		}
		if !strings.Contains(r.Error, "delete and recreate") {
			t.Fatalf("error=%q", r.Error)
		}
	}
	if !saw {
		t.Fatalf("missing trigger result %+v", again)
	}
}

func TestFindRemoteTriggerMatchesErrorHandlerPath(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, root, "src/server/handler.json", `{
  "type": "server-function",
  "name": "handler",
  "context": "billing",
  "code": "return true;",
  "description": "fn"
}`)
	client := api.NewMemoryClient()
	eng := engine(root, client)
	push, err := eng.Push()
	if err != nil {
		t.Fatal(err)
	}
	var fnID string
	for _, r := range push {
		if r.Type == glide.TypeServerFunction {
			fnID = r.ID
		}
	}
	if fnID == "" {
		t.Fatalf("missing function %+v", push)
	}
	client.Insert("triggers", "t1", map[string]any{
		"id": "t1", "name": "eh1",
		"source":      map[string]any{"errorHandler": map[string]any{"path": "path.one"}},
		"destination": map[string]any{"serverFunctionId": fnID},
	})
	client.Insert("triggers", "t2", map[string]any{
		"id": "t2", "name": "eh2",
		"source":      map[string]any{"errorHandler": map[string]any{"path": "path.two"}},
		"destination": map[string]any{"serverFunctionId": fnID},
	})
	writeJSON(t, root, "src/artifacts/triggers/fresh.json", `{
  "name": "fresh",
  "source": {"errorHandler": {"path": "path.two"}},
  "destination": {"functionContext": "billing", "functionName": "handler"}
}`)
	again, err := eng.Push()
	if err != nil {
		t.Fatal(err)
	}
	var tr glide.Result
	for _, r := range again {
		if r.Type == glide.TypeTrigger && r.Name == "fresh" {
			tr = r
		}
	}
	if tr.Action != glide.ActionUpdate || tr.ID != "t2" {
		t.Fatalf("expected update t2 %+v", again)
	}
	got, err := client.Get("triggers", "t2")
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["name"] != "fresh" {
		t.Fatalf("t2=%v", got)
	}
	still, err := client.Get("triggers", "t1")
	if err != nil {
		t.Fatal(err)
	}
	if still.(map[string]any)["name"] != "eh1" {
		t.Fatalf("t1 should be unchanged: %v", still)
	}
}

func TestJobGlideRewriteFunctionRefs(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, root, "src/server/handler.json", `{
  "type": "server-function",
  "name": "handler",
  "context": "billing",
  "code": "return true;",
  "description": "fn"
}`)
	writeJSON(t, root, "src/artifacts/jobs/nightly.json", `{
  "name": "nightly",
  "functions": [{"functionContext": "billing", "functionName": "handler", "eventPayload": {"n": 1}}],
  "executionType": "sequential",
  "schedule": "0 3 * * *",
  "description": "ignored"
}`)
	client := api.NewMemoryClient()
	eng := engine(root, client)
	push, err := eng.Push()
	if err != nil {
		t.Fatal(err)
	}
	var jobID, fnID string
	for _, r := range push {
		if r.Action != glide.ActionCreate && r.Action != glide.ActionSkip {
			t.Fatalf("push %+v", push)
		}
		switch r.Type {
		case glide.TypeJob:
			jobID = r.ID
		case glide.TypeServerFunction:
			fnID = r.ID
		}
	}
	if jobID == "" || fnID == "" {
		t.Fatalf("missing ids %+v", push)
	}
	got, err := client.Get("jobs", jobID)
	if err != nil {
		t.Fatal(err)
	}
	jm := got.(map[string]any)
	if _, ok := jm["description"]; ok {
		t.Fatalf("description leaked: %v", jm)
	}
	sch, _ := jm["schedule"].(map[string]any)
	if sch["type"] != "periodical" || sch["value"] != "0 3 * * *" {
		t.Fatalf("schedule=%v", sch)
	}
	fns, _ := jm["functions"].([]any)
	fn, _ := fns[0].(map[string]any)
	if fn["id"] != fnID {
		t.Fatalf("functions=%v want %s", fns, fnID)
	}
	if _, ok := fn["functionContext"]; ok {
		t.Fatalf("functionContext leaked: %v", fn)
	}

	again, err := eng.Push()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range again {
		if r.Action != glide.ActionSkip && r.Action != glide.ActionOrphan {
			t.Fatalf("second push %+v", again)
		}
	}
}

func TestSchemaGlideIgnoresInjectedDraft(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, root, "src/artifacts/schemas/order.json", `{
  "name": "Order",
  "context": "billing",
  "definition": {
    "type": "object",
    "properties": { "sku": { "type": "string" } }
  },
  "ownerUserId": "should-not-push"
}`)
	client := api.NewMemoryClient()
	eng := engine(root, client)
	push, err := eng.Push()
	if err != nil {
		t.Fatal(err)
	}
	if len(push) != 1 || push[0].Action != glide.ActionCreate {
		t.Fatalf("push %+v", push)
	}
	got, err := client.Get("schemas", push[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	m := got.(map[string]any)
	if _, ok := m["ownerUserId"]; ok {
		t.Fatalf("ownerUserId leaked: %v", m)
	}
	def, _ := m["definition"].(map[string]any)
	def["$schema"] = "http://json-schema.org/draft-06/schema#"
	def["additionalProperties"] = false
	client.Insert("schemas", push[0].ID, m)

	again, err := eng.Push()
	if err != nil {
		t.Fatal(err)
	}
	if again[0].Action != glide.ActionSkip {
		t.Fatalf("injected $schema should not update: %+v", again)
	}
}

func TestSchemaValidateRequiresDefinitionAndRefs(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, root, "src/artifacts/schemas/order.json", `{
  "name": "Order",
  "context": "billing"
}`)
	rep, err := engine(root, nil).Validate()
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Failed(false) {
		t.Fatalf("expected missing definition: %+v", rep.Issues)
	}
	sawDef, sawDesc := false, false
	for _, iss := range rep.Issues {
		if iss.Category == glide.CatMetadata && strings.Contains(iss.Message, "definition") {
			sawDef = true
		}
		if strings.Contains(iss.Message, "description") {
			sawDesc = true
		}
	}
	if !sawDef {
		t.Fatalf("%+v", rep.Issues)
	}
	if sawDesc {
		t.Fatalf("schemas should not warn missing description: %+v", rep.Issues)
	}

	root = t.TempDir()
	writeJSON(t, root, "src/artifacts/schemas/order.json", `{
  "name": "Order",
  "context": "billing",
  "definition": {
    "properties": {
      "customer": { "x-poly-ref": { "path": "billing.Missing" } }
    }
  }
}`)
	rep, err = engine(root, nil).Validate()
	if err != nil {
		t.Fatal(err)
	}
	sawRef := false
	for _, iss := range rep.Issues {
		if iss.Category == glide.CatCrossResource && strings.Contains(iss.Message, "billing.Missing") {
			sawRef = true
		}
	}
	if !sawRef {
		t.Fatalf("expected dangling x-poly-ref: %+v", rep.Issues)
	}

	root = t.TempDir()
	writeJSON(t, root, "src/artifacts/schemas/customer.json", `{
  "name": "Customer",
  "context": "billing",
  "definition": { "type": "object" }
}`)
	writeJSON(t, root, "src/artifacts/schemas/order.json", `{
  "name": "Order",
  "context": "billing",
  "definition": {
    "properties": {
      "customer": { "x-poly-ref": { "path": "billing.Customer" } }
    }
  }
}`)
	rep, err = engine(root, nil).Validate()
	if err != nil {
		t.Fatal(err)
	}
	for _, iss := range rep.Issues {
		if iss.Category == glide.CatCrossResource {
			t.Fatalf("expected resolved x-poly-ref: %+v", rep.Issues)
		}
	}
}

func TestSnippetGlideSecondPushSkips(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, root, "src/artifacts/snippets/header.json", `{
  "name": "header",
  "context": "billing",
  "code": "export const n = 1;",
  "language": "typescript",
  "description": "Shared header",
  "ownerUserId": "nope"
}`)
	client := api.NewMemoryClient()
	eng := engine(root, client)
	push, err := eng.Push()
	if err != nil {
		t.Fatal(err)
	}
	if len(push) != 1 || push[0].Action != glide.ActionCreate {
		t.Fatalf("push %+v", push)
	}
	got, err := client.Get("snippets", push[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	m := got.(map[string]any)
	if _, ok := m["ownerUserId"]; ok {
		t.Fatalf("ownerUserId leaked: %v", m)
	}
	if m["code"] != "export const n = 1;" || m["language"] != "typescript" {
		t.Fatalf("payload=%v", m)
	}

	again, err := eng.Push()
	if err != nil {
		t.Fatal(err)
	}
	if again[0].Action != glide.ActionSkip {
		t.Fatalf("second push %+v", again)
	}
}

func TestSnippetValidateRequiresCodeAndLanguage(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, root, "src/artifacts/snippets/header.json", `{
  "name": "header",
  "context": "billing"
}`)
	rep, err := engine(root, nil).Validate()
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Failed(false) {
		t.Fatalf("expected missing code/language: %+v", rep.Issues)
	}
	sawCode, sawLang := false, false
	for _, iss := range rep.Issues {
		if iss.Category == glide.CatMetadata && strings.Contains(iss.Message, "code") {
			sawCode = true
		}
		if iss.Category == glide.CatMetadata && strings.Contains(iss.Message, "language") {
			sawLang = true
		}
	}
	if !sawCode || !sawLang {
		t.Fatalf("%+v", rep.Issues)
	}
}

func TestEnvCheckOnlyIgnoresRefs(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, root, "src/artifacts/webhooks/hook.json", `{
  "name": "hook",
  "context": "billing",
  "description": "wh",
  "securityFunctions": [{"id": "billing.missingFn"}]
}`)
	eng := engine(root, nil)
	eng.Only = []string{"env"}
	rep, err := eng.Validate()
	if err != nil {
		t.Fatal(err)
	}
	for _, iss := range rep.Issues {
		if iss.Category == glide.CatCrossResource {
			t.Fatalf("env-only should skip refs: %+v", rep.Issues)
		}
	}
}

func TestApplicationValidateRequiresConfig(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, root, "src/artifacts/applications/dashboard.json", `{
  "name": "dashboard",
  "description": "ui"
}`)
	rep, err := engine(root, nil).Validate()
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Failed(false) {
		t.Fatalf("expected missing config: %+v", rep.Issues)
	}
	saw := false
	for _, iss := range rep.Issues {
		if iss.Category == glide.CatMetadata && strings.Contains(iss.Message, "config") {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("%+v", rep.Issues)
	}
}

func TestApplicationGlidePushHydratesConfig(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, root, "src/artifacts/applications/dashboard.json", `{
  "polyType": "application",
  "name": "dashboard",
  "description": "Partner portal",
  "visibility": "ENVIRONMENT",
  "config": {
    "name": "dashboard",
    "subpath": "partner-portal",
    "collections": []
  }
}`)
	client := api.NewMemoryClient()
	eng := engine(root, client)
	push, err := eng.Push()
	if err != nil {
		t.Fatal(err)
	}
	if len(push) != 1 || push[0].Action != glide.ActionCreate || push[0].Type != glide.TypeApplication {
		t.Fatalf("push %+v", push)
	}
	got, err := client.Get("applications", push[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	m := got.(map[string]any)
	if m["name"] != "dashboard" {
		t.Fatalf("%v", m)
	}
	if _, ok := m["subpath"]; ok {
		t.Fatalf("top-level subpath should not be sent: %v", m)
	}
	cfg, _ := m["config"].(map[string]any)
	if cfg["subpath"] != "partner-portal" {
		t.Fatalf("config=%v", cfg)
	}

	again, err := eng.Push()
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 1 || again[0].Action != glide.ActionSkip {
		t.Fatalf("second push %+v", again)
	}
}

type fakeCodeAdapter struct {
	inspected []string
	prepared  []delegate.PrepareFile
}

func (f *fakeCodeAdapter) Extract(file, typ string) (any, string, error) {
	return map[string]any{"name": "hello", "context": "billing", "code": "function hello() {}"}, "h", nil
}

func (f *fakeCodeAdapter) Inspect(files []string) ([]delegate.InspectItem, error) {
	f.inspected = append([]string{}, files...)
	var items []delegate.InspectItem
	for _, file := range files {
		items = append(items, delegate.InspectItem{
			File:        file,
			Type:        "server-function",
			Code:        "function hello() { return 1 }",
			Description: "",
			Arguments:   []delegate.InspectArg{{Name: "n", Type: "number", Description: ""}},
		})
	}
	return items, nil
}

func (f *fakeCodeAdapter) Prepare(files []delegate.PrepareFile, disableDocs bool) ([]string, []string, error) {
	f.prepared = append([]delegate.PrepareFile{}, files...)
	var names []string
	for _, file := range files {
		names = append(names, file.File)
	}
	return names, nil, nil
}

type fakeDescriber struct {
	called int
}

func (d *fakeDescriber) DescribeCustomFunction(kind string, payload any) (map[string]any, error) {
	d.called++
	return map[string]any{
		"description": "greets the caller",
		"arguments":   []any{map[string]any{"name": "n", "description": "how many"}},
	}, nil
}

func TestPrepareInspectsThenWritesHostDocs(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "src", "billing", "server", "hello.ts")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("const polyConfig: PolyServerFunction = { name: 'hello', context: 'billing' };\nfunction hello() { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ad := &fakeCodeAdapter{}
	ai := &fakeDescriber{}
	eng := engine(root, nil)
	eng.Adapter = ad
	eng.Describer = ai
	changed, _, err := eng.Prepare()
	if err == nil {
		t.Fatal("expected failure exit when files changed")
	}
	if len(changed) != 1 {
		t.Fatalf("changed=%v", changed)
	}
	if ai.called != 1 {
		t.Fatalf("describer called %d", ai.called)
	}
	if len(ad.prepared) != 1 || ad.prepared[0].Docs == nil || ad.prepared[0].Docs.Description != "greets the caller" {
		t.Fatalf("prepared %+v", ad.prepared)
	}
	if len(ad.prepared[0].Docs.Arguments) != 1 || ad.prepared[0].Docs.Arguments[0].Description != "how many" {
		t.Fatalf("args %+v", ad.prepared[0].Docs.Arguments)
	}
}

type patchSpy struct {
	*api.MemoryClient
	patches []map[string]any
}

func (s *patchSpy) Update(resource, id string, payload any, headers map[string]string) (any, error) {
	if m, ok := payload.(map[string]any); ok {
		cp := map[string]any{}
		for k, v := range m {
			cp[k] = v
		}
		s.patches = append(s.patches, cp)
	}
	return s.MemoryClient.Update(resource, id, payload, headers)
}

func TestSubscriptionGlideRewriteAndFieldPatch(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, root, "src/server/handler.json", `{
  "type": "server-function",
  "name": "handler",
  "context": "shopify",
  "code": "return true;",
  "description": "fn"
}`)
	writeJSON(t, root, "src/shopify/artifacts/subscriptions/ordersStream.json", `{
  "polyType": "subscription",
  "name": "ordersStream",
  "context": "shopify",
  "description": "stream",
  "type": "CUSTOM",
  "websocketUrl": "wss://example.com/graphql",
  "query": "subscription { orderUpdated { id } }",
  "functionId": "shopify.handler",
  "enabled": true
}`)
	spy := &patchSpy{MemoryClient: api.NewMemoryClient()}
	eng := engine(root, spy)
	push, err := eng.Push()
	if err != nil {
		t.Fatal(err)
	}
	var subID, fnID string
	for _, r := range push {
		switch r.Type {
		case glide.TypeSubscription:
			if r.Action != glide.ActionCreate {
				t.Fatalf("create %+v", push)
			}
			subID = r.ID
		case glide.TypeServerFunction:
			fnID = r.ID
		}
	}
	if subID == "" || fnID == "" {
		t.Fatalf("missing ids %+v", push)
	}
	got, err := spy.Get("subscriptions/graphql", subID)
	if err != nil {
		t.Fatal(err)
	}
	m := got.(map[string]any)
	if m["functionId"] != fnID {
		t.Fatalf("functionId=%v want %s", m["functionId"], fnID)
	}
	if _, ok := m["ohipOffset"]; ok {
		t.Fatalf("ohipOffset leaked on create: %v", m)
	}

	again, err := eng.Push()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range again {
		if r.Type == glide.TypeSubscription && r.Action != glide.ActionSkip {
			t.Fatalf("second push should skip %+v", again)
		}
	}

	m["ohipOffset"] = "9999"
	m["paramsObject"] = map[string]any{"ohip": map[string]any{"clientSecret": "live-secret"}}
	spy.Insert("subscriptions/graphql", subID, m)
	third, err := eng.Push()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range third {
		if r.Type == glide.TypeSubscription && r.Action != glide.ActionSkip {
			t.Fatalf("runtime ohipOffset / remote secrets should skip %+v", third)
		}
	}

	writeJSON(t, root, "src/shopify/artifacts/subscriptions/ordersStream.json", `{
  "polyType": "subscription",
  "name": "ordersStream",
  "context": "shopify",
  "description": "renamed desc",
  "type": "CUSTOM",
  "websocketUrl": "wss://example.com/graphql",
  "query": "subscription { orderUpdated { id } }",
  "functionId": "shopify.handler",
  "enabled": true
}`)
	spy.patches = nil
	fourth, err := eng.Push()
	if err != nil {
		t.Fatal(err)
	}
	var sub glide.Result
	for _, r := range fourth {
		if r.Type == glide.TypeSubscription {
			sub = r
		}
	}
	if sub.Action != glide.ActionUpdate {
		t.Fatalf("expected update %+v", fourth)
	}
	if len(spy.patches) != 1 {
		t.Fatalf("patches=%v", spy.patches)
	}
	p := spy.patches[0]
	if p["description"] != "renamed desc" {
		t.Fatalf("patch=%v", p)
	}
	if _, ok := p["query"]; ok {
		t.Fatalf("query must not be patched: %v", p)
	}
	if _, ok := p["websocketUrl"]; ok {
		t.Fatalf("url must not be patched: %v", p)
	}
	if _, ok := p["paramsObject"]; ok {
		t.Fatalf("placeholder params must not overwrite secrets: %v", p)
	}
}

func TestSubscriptionGlidePullRedactsSecrets(t *testing.T) {
	root := t.TempDir()
	client := api.NewMemoryClient()
	client.Insert("subscriptions/graphql", "s1", map[string]any{
		"id": "s1", "name": "ordersStream", "context": "shopify",
		"type": "OHIP", "websocketUrl": "wss://ohip.example/graphql",
		"query":      "subscription { newEvent { metadata { offset } } }",
		"functionId": "fn-1", "enabled": true,
		"ohipOffset": "4815",
		"paramsObject": map[string]any{"ohip": map[string]any{
			"hostName": "h", "appKey": "a", "enterpriseId": "e", "clientId": "c", "clientSecret": "live-secret",
		}},
	})
	eng := engine(root, client)
	res, err := eng.Pull()
	if err != nil {
		t.Fatal(err)
	}
	var pulled glide.Result
	for _, r := range res {
		if r.Type == glide.TypeSubscription {
			pulled = r
		}
	}
	if pulled.File == "" {
		t.Fatalf("pull %+v", res)
	}
	raw, err := os.ReadFile(filepath.Join(root, pulled.File))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "live-secret") {
		t.Fatalf("secret written to %s:\n%s", pulled.File, raw)
	}
	if strings.Contains(string(raw), "4815") {
		t.Fatalf("ohipOffset should not be pulled:\n%s", raw)
	}
}

func TestSubscriptionValidateRejectsCustomOHIPFields(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, root, "src/artifacts/subscriptions/bad.json", `{
  "name": "bad",
  "context": "shopify",
  "type": "CUSTOM",
  "websocketUrl": "wss://example.com/graphql",
  "query": "subscription { x }",
  "functionId": "shopify.handler",
  "ohipMaintainOffset": true
}`)
	rep, err := engine(root, nil).Validate()
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Failed(false) {
		t.Fatalf("expected error %+v", rep.Issues)
	}
	saw := false
	for _, iss := range rep.Issues {
		if strings.Contains(iss.Message, "OHIP fields") {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("%+v", rep.Issues)
	}
}
