package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/polyapi/polyglot/src/exitcode"
	"github.com/polyapi/polyglot/src/glide"
)

func TestVariInitWritesDefaultPath(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("POLY_API_KEY", "")
	t.Setenv("POLY_API_BASE_URL", "")
	dir := t.TempDir()
	t.Chdir(dir)

	stdout, stderr, code := run("vari", "init", "--name", "apiKey", "--context", "billing")
	if code != 0 {
		t.Fatalf("exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	rel := "src/billing/artifacts/vari/apiKey.jsonc"
	if !strings.Contains(stdout, rel) {
		t.Fatalf("expected path in stdout %q", stdout)
	}
	raw, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatal(err)
	}
	stripped := glide.StripJSONC(string(raw))
	var obj map[string]any
	if err := json.Unmarshal([]byte(stripped), &obj); err != nil {
		t.Fatal(err)
	}
	if obj["name"] != "apiKey" || obj["context"] != "billing" || obj["polyType"] != "variable" {
		t.Fatalf("%+v", obj)
	}
	if _, ok := obj["value"]; !ok {
		t.Fatal("expected value placeholder")
	}
}

func TestVariInitRequiresOnlyName(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	dir := t.TempDir()
	t.Chdir(dir)
	stdout, stderr, code := run("vari", "init", "--name", "apiKey")
	if code != 0 {
		t.Fatalf("name-only exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	rel := "src/artifacts/vari/apiKey.jsonc"
	if !strings.Contains(stdout, rel) {
		t.Fatalf("expected default path without context %q", stdout)
	}
	raw, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(glide.StripJSONC(string(raw))), &obj); err != nil {
		t.Fatal(err)
	}
	if obj["name"] != "apiKey" || obj["context"] != "" {
		t.Fatalf("%+v", obj)
	}
	_, _, code = run("vari", "init", "--context", "billing")
	if code != exitcode.Usage {
		t.Fatalf("missing name exit %d", code)
	}
}

func TestJobInitSkipsContextAndUsesArtifactsRoot(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	dir := t.TempDir()
	t.Chdir(dir)
	stdout, stderr, code := run("job", "init", "--name", "nightly")
	if code != 0 {
		t.Fatalf("exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	rel := "src/artifacts/jobs/nightly.jsonc"
	if !strings.Contains(stdout, rel) {
		t.Fatalf("%q", stdout)
	}
	if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
		t.Fatal(err)
	}
}

func TestAppInitSkipsContextAndUsesArtifactsRoot(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	dir := t.TempDir()
	t.Chdir(dir)
	stdout, stderr, code := run("app", "init", "--name", "dashboard")
	if code != 0 {
		t.Fatalf("exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	rel := "src/artifacts/applications/dashboard.jsonc"
	if !strings.Contains(stdout, rel) {
		t.Fatalf("%q", stdout)
	}
	raw, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(glide.StripJSONC(string(raw))), &obj); err != nil {
		t.Fatal(err)
	}
	if obj["polyType"] != "application" || obj["name"] != "dashboard" {
		t.Fatalf("%+v", obj)
	}
	cfg, _ := obj["config"].(map[string]any)
	if cfg == nil {
		t.Fatal("expected config placeholder")
	}
	if _, ok := cfg["collections"]; !ok {
		t.Fatalf("config=%v", cfg)
	}
}

func TestFunctionInitRequiresType(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	_, _, code := run("function", "init", "--name", "echo", "--context", "billing")
	if code != exitcode.Usage {
		t.Fatalf("exit %d", code)
	}
}

func TestFunctionInitWritesTypeScriptSource(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	dir := t.TempDir()
	t.Chdir(dir)
	stdout, stderr, code := run("function", "init", "--name", "helloWorld", "--context", "billing", "--type", "server")
	if code != 0 {
		t.Fatalf("exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	rel := "src/billing/server/helloWorld.ts"
	if !strings.Contains(stdout, rel) {
		t.Fatalf("%q", stdout)
	}
	raw, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, "const polyConfig: PolyServerFunction") {
		t.Fatalf("%s", body)
	}
	if !strings.Contains(body, "name: 'helloWorld'") {
		t.Fatalf("%s", body)
	}
	if !strings.Contains(body, "function helloWorld(): string") {
		t.Fatalf("%s", body)
	}
}

func TestFunctionInitPythonAndClient(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	dir := t.TempDir()
	t.Chdir(dir)
	stdout, stderr, code := run("function", "init", "--name", "hello_world", "--type", "server", "--lang", "python")
	if code != 0 {
		t.Fatalf("python exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	rel := "src/server/hello_world.py"
	if !strings.Contains(stdout, rel) {
		t.Fatalf("%q", stdout)
	}
	raw, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "def hello_world(first_name: str)") {
		t.Fatalf("%s", raw)
	}

	stdout, stderr, code = run("function", "init", "--name", "greet", "--type", "client")
	if code != 0 {
		t.Fatalf("client exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	raw, err = os.ReadFile(filepath.Join(dir, "src/client/greet.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "PolyClientFunction") {
		t.Fatalf("%s", raw)
	}
	if strings.Contains(string(raw), "logsEnabled") {
		t.Fatalf("client logsEnabled: %s", raw)
	}
}

func TestFunctionInitRejectsInvalidIdentifier(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	dir := t.TempDir()
	t.Chdir(dir)
	_, stderr, code := run("function", "init", "--name", "hello-world", "--type", "server")
	if code != exitcode.Usage {
		t.Fatalf("hyphen exit %d stderr=%q", code, stderr)
	}
	if !strings.Contains(stderr, "identifier") {
		t.Fatalf("stderr=%q", stderr)
	}
	_, stderr, code = run("function", "init", "--name", "class", "--type", "server")
	if code != exitcode.Usage {
		t.Fatalf("keyword exit %d stderr=%q", code, stderr)
	}
	_, stderr, code = run("function", "init", "--name", "def", "--type", "server", "--lang", "python")
	if code != exitcode.Usage {
		t.Fatalf("python keyword exit %d stderr=%q", code, stderr)
	}
}

func TestFunctionInitLangPathConflict(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	dir := t.TempDir()
	t.Chdir(dir)
	_, stderr, code := run("function", "init", "--name", "helloWorld", "--type", "server", "--lang", "python", "--path", "echo.ts")
	if code != exitcode.Usage {
		t.Fatalf("exit %d stderr=%q", code, stderr)
	}
	if !strings.Contains(strings.ToLower(stderr), "does not match path extension") {
		t.Fatalf("stderr=%q", stderr)
	}
}

func TestFunctionInitAPIStillJSONC(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	dir := t.TempDir()
	t.Chdir(dir)
	stdout, stderr, code := run("function", "init", "--name", "proxy", "--type", "api")
	if code != 0 {
		t.Fatalf("exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "src/artifacts/api/proxy.jsonc") {
		t.Fatalf("%q", stdout)
	}
}

func TestInitCustomPathAndDirectory(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	dir := t.TempDir()
	t.Chdir(dir)

	stdout, stderr, code := run("schema", "init", "--name", "Order", "--context", "billing", "--path", "custom/order.jsonc")
	if code != 0 {
		t.Fatalf("exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "custom/order.jsonc") {
		t.Fatalf("%q", stdout)
	}

	if err := os.MkdirAll(filepath.Join(dir, "out"), 0o755); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code = run("table", "init", "--name", "orders", "--context", "billing", "--path", "out")
	if code != 0 {
		t.Fatalf("dir path exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "out/orders.jsonc") {
		t.Fatalf("%q", stdout)
	}
}

func TestInitRefusesOverwriteWithoutForce(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	dir := t.TempDir()
	t.Chdir(dir)
	rel := "src/billing/artifacts/vari/apiKey.jsonc"
	if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, rel), []byte(`{"name":"old"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := run("vari", "init", "--name", "apiKey", "--context", "billing")
	if code != exitcode.Failure {
		t.Fatalf("exit %d stderr=%q", code, stderr)
	}
	if !strings.Contains(stderr, "already exists") {
		t.Fatalf("expected already exists: stderr=%q", stderr)
	}
	stdout, stderr, code := run("vari", "init", "--name", "apiKey", "--context", "billing", "--force")
	if code != 0 {
		t.Fatalf("force exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	raw, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"polyType": "variable"`) {
		t.Fatalf("not overwritten: %s", raw)
	}
}

func TestInitQuietPrintsPathOnly(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	dir := t.TempDir()
	t.Chdir(dir)
	stdout, stderr, code := run("-q", "webhook", "init", "--name", "hook", "--context", "billing")
	if code != 0 {
		t.Fatalf("exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	got := strings.TrimSpace(stdout)
	if got != "src/billing/artifacts/webhooks/hook.jsonc" {
		t.Fatalf("%q", stdout)
	}
	if strings.Contains(stdout, "OK") {
		t.Fatalf("quiet should not print OK: %q", stdout)
	}
}

func TestInitDoesNotNeedCredentials(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("POLY_API_KEY", "")
	t.Setenv("POLY_API_BASE_URL", "")
	dir := t.TempDir()
	t.Chdir(dir)
	_, stderr, code := run("snippet", "init", "--name", "header", "--context", "billing")
	if code != 0 {
		t.Fatalf("exit %d stderr=%q", code, stderr)
	}
}

func TestInitHelpListsRequiredFlags(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("__FANG_TEST_WIDTH", "120")
	stdout, _, code := run("vari", "init", "--help")
	if code != 0 {
		t.Fatalf("help exit %d", code)
	}
	for _, s := range []string{"--name", "--context", "(required)", "--path", "--force", "--snippet", "polyapi vari init --name example"} {
		if !strings.Contains(stdout, s) {
			t.Errorf("help missing %q\n%s", s, stdout)
		}
	}
	if strings.Contains(stdout, "context (required)") || strings.Contains(stdout, "context. (required)") {
		t.Fatalf("--context should not be required:\n%s", stdout)
	}
}

func TestTriggerInitHasNoContextFlag(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	dir := t.TempDir()
	t.Chdir(dir)
	_, stderr, code := run("trigger", "init", "--name", "weekly", "--type", "webhook", "--context", "billing")
	if code != exitcode.Usage {
		t.Fatalf("unexpected --context exit %d stderr=%q", code, stderr)
	}
	stdout, stderr, code := run("trigger", "init", "--name", "weekly", "--type", "webhook")
	if code != 0 {
		t.Fatalf("exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "src/artifacts/triggers/weekly.jsonc") {
		t.Fatalf("%q", stdout)
	}
}

func TestTriggerInitRequiresType(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	dir := t.TempDir()
	t.Chdir(dir)
	_, stderr, code := run("trigger", "init", "--name", "weekly")
	if code != exitcode.Usage {
		t.Fatalf("exit %d stderr=%q", code, stderr)
	}
	if !strings.Contains(stderr, "type") {
		t.Fatalf("expected type required, stderr=%q", stderr)
	}
}

func TestTriggerInitWebhookAndErrorHandler(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	dir := t.TempDir()
	t.Chdir(dir)

	stdout, stderr, code := run("trigger", "init", "--name", "weekly", "--type", "webhook")
	if code != 0 {
		t.Fatalf("webhook exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	wh := readInitJSONC(t, dir, "src/artifacts/triggers/weekly.jsonc")
	src, _ := wh["source"].(map[string]any)
	if src["webhookHandleId"] != "example.hook" {
		t.Fatalf("webhook source=%v", src)
	}
	if _, ok := src["errorHandler"]; ok {
		t.Fatalf("webhook scaffold should not set errorHandler: %v", src)
	}
	if wh["waitForResponse"] != true {
		t.Fatalf("webhook waitForResponse=%v", wh["waitForResponse"])
	}

	stdout, stderr, code = run("trigger", "init", "--name", "on-error", "--type", "error-handler")
	if code != 0 {
		t.Fatalf("error-handler exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	eh := readInitJSONC(t, dir, "src/artifacts/triggers/on-error.jsonc")
	src, _ = eh["source"].(map[string]any)
	handler, _ := src["errorHandler"].(map[string]any)
	if handler["path"] != "example.handler" {
		t.Fatalf("error-handler source=%v", src)
	}
	if _, ok := src["webhookHandleId"]; ok {
		t.Fatalf("error-handler scaffold should not set webhookHandleId: %v", src)
	}
	if _, ok := eh["waitForResponse"]; ok {
		t.Fatalf("error-handler scaffold should omit waitForResponse: %v", eh)
	}

	_, stderr, code = run("trigger", "init", "--name", "bad", "--type", "cron")
	if code != exitcode.Usage {
		t.Fatalf("invalid type exit %d stderr=%q", code, stderr)
	}

	stdout, stderr, code = run("trigger", "init", "--name", "aliased", "--type", "error_handler")
	if code != 0 {
		t.Fatalf("alias exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	alias := readInitJSONC(t, dir, "src/artifacts/triggers/aliased.jsonc")
	src, _ = alias["source"].(map[string]any)
	handler, _ = src["errorHandler"].(map[string]any)
	if handler["path"] != "example.handler" {
		t.Fatalf("alias source=%v", src)
	}
}

func TestSubscriptionInitRequiresTypeAndWritesKinds(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	dir := t.TempDir()
	t.Chdir(dir)

	_, stderr, code := run("subscription", "init", "--name", "ordersStream")
	if code != exitcode.Usage {
		t.Fatalf("missing type exit %d stderr=%q", code, stderr)
	}
	if !strings.Contains(stderr, "type") {
		t.Fatalf("expected type required, stderr=%q", stderr)
	}

	_, stderr, code = run("subscription", "init", "--name", "orders-stream", "--type", "CUSTOM")
	if code != exitcode.Usage {
		t.Fatalf("hyphen name exit %d stderr=%q", code, stderr)
	}

	_, stderr, code = run("subscription", "init", "--name", "ordersStream", "--type", "webhook")
	if code != exitcode.Usage {
		t.Fatalf("bad type exit %d stderr=%q", code, stderr)
	}

	stdout, stderr, code := run("subscription", "init", "--name", "ordersStream", "--type", "custom")
	if code != 0 {
		t.Fatalf("CUSTOM exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	rel := "src/artifacts/subscriptions/ordersStream.jsonc"
	if !strings.Contains(stdout, rel) {
		t.Fatalf("%q", stdout)
	}
	custom := readInitJSONC(t, dir, rel)
	if custom["polyType"] != "subscription" || custom["type"] != "CUSTOM" {
		t.Fatalf("%+v", custom)
	}
	if _, ok := custom["ohipMaintainOffset"]; ok {
		t.Fatalf("CUSTOM scaffold should omit OHIP fields: %v", custom)
	}

	stdout, stderr, code = run("subscription", "init", "--name", "operaEvents", "--type", "OHIP", "--context", "opera")
	if code != 0 {
		t.Fatalf("OHIP exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	ohipRel := "src/opera/artifacts/subscriptions/operaEvents.jsonc"
	if !strings.Contains(stdout, ohipRel) {
		t.Fatalf("%q", stdout)
	}
	ohip := readInitJSONC(t, dir, ohipRel)
	if ohip["type"] != "OHIP" || ohip["ohipMaintainOffset"] != true {
		t.Fatalf("%+v", ohip)
	}
	params, _ := ohip["paramsObject"].(map[string]any)
	ohipObj, _ := params["ohip"].(map[string]any)
	if ohipObj["clientSecret"] != "{{OHIP_CLIENT_SECRET}}" {
		t.Fatalf("ohip params=%v", ohipObj)
	}
	if _, ok := ohip["ohipOffset"]; ok {
		t.Fatalf("scaffold should omit ohipOffset: %v", ohip)
	}
}

func readInitJSONC(t *testing.T, dir, rel string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(glide.StripJSONC(string(raw))), &obj); err != nil {
		t.Fatalf("%s: %v\n%s", rel, err, raw)
	}
	return obj
}
