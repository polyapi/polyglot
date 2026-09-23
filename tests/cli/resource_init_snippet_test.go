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

const tsSnippetBody = `import { PolyServerFunction } from 'polyapi';

const polyConfig: PolyServerFunction = {
    name: 'oldFn',
    context: 'oldCtx',
    visibility: 'ENVIRONMENT',
};

function oldFn(): string {
    return 'from snippet';
}
`

const pySnippetBody = `from polyapi.typedefs import PolyServerFunction

polyConfig: PolyServerFunction = {
    'name': 'old_fn',
    'context': 'old_ctx',
    'visibility': 'ENVIRONMENT',
}

def old_fn(first_name: str) -> str:
    return first_name
`

func TestFunctionInitFromSnippetOverlaysIdentity(t *testing.T) {
	dir := t.TempDir()
	fake := newSnippetFake()
	fake.put("aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee", "templates", "helloWorld", map[string]any{
		"language": "typescript",
		"code":     tsSnippetBody,
	})
	srv := startSnippetAPI(t, fake)
	snippetEnv(t, dir, srv.URL)

	stdout, stderr, code := run("function", "init", "--name", "helloWorld", "--type", "server", "--context", "billing", "--snippet", "templates.helloWorld")
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
	if !strings.Contains(body, "from snippet") {
		t.Fatalf("expected snippet body:\n%s", body)
	}
	if !strings.Contains(body, "name: 'helloWorld'") || !strings.Contains(body, "context: 'billing'") {
		t.Fatalf("identity not overlaid:\n%s", body)
	}
	if !strings.Contains(body, "function helloWorld(): string") {
		t.Fatalf("function ident not renamed:\n%s", body)
	}
	if strings.Contains(body, "oldFn") || strings.Contains(body, "oldCtx") {
		t.Fatalf("old identity still present:\n%s", body)
	}
}

func TestFunctionInitFromSnippetPython(t *testing.T) {
	dir := t.TempDir()
	fake := newSnippetFake()
	fake.put("s1", "templates", "hello_world", map[string]any{
		"language": "python",
		"code":     pySnippetBody,
	})
	srv := startSnippetAPI(t, fake)
	snippetEnv(t, dir, srv.URL)

	stdout, stderr, code := run("--lang", "python", "function", "init", "--name", "hello_world", "--type", "server", "--snippet", "templates.hello_world")
	if code != 0 {
		t.Fatalf("exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "src/server/hello_world.py"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, "def hello_world(first_name: str)") {
		t.Fatalf("%s", body)
	}
	if !strings.Contains(body, "'context': ''") {
		t.Fatalf("expected empty context overlay:\n%s", body)
	}
}

func TestFunctionInitSnippetLanguageMismatchDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	fake := newSnippetFake()
	fake.put("s1", "templates", "helloWorld", map[string]any{
		"language": "python",
		"code":     pySnippetBody,
	})
	srv := startSnippetAPI(t, fake)
	snippetEnv(t, dir, srv.URL)

	_, stderr, code := run("function", "init", "--name", "helloWorld", "--type", "server", "--snippet", "templates.helloWorld")
	if code != exitcode.Failure {
		t.Fatalf("exit %d stderr=%q", code, stderr)
	}
	if !strings.Contains(stderr, "python") || !strings.Contains(stderr, "typescript") {
		t.Fatalf("expected language mismatch: %q", stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "src/server/helloWorld.ts")); err == nil {
		t.Fatal("must not write a file on language mismatch")
	}
}

func TestVariInitFromJSONSnippet(t *testing.T) {
	dir := t.TempDir()
	fake := newSnippetFake()
	fake.put("s1", "templates", "variable", map[string]any{
		"language": "json",
		"code": `{
  "name": "template",
  "context": "old",
  "description": "from snippet",
  "secrecy": "NONE",
  "value": {"token": true}
}`,
	})
	srv := startSnippetAPI(t, fake)
	snippetEnv(t, dir, srv.URL)

	stdout, stderr, code := run("vari", "init", "--name", "apiKey", "--context", "billing", "--snippet", "templates.variable")
	if code != 0 {
		t.Fatalf("exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	rel := "src/billing/artifacts/vari/apiKey.jsonc"
	raw, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(glide.StripJSONC(string(raw))), &obj); err != nil {
		t.Fatal(err)
	}
	if obj["name"] != "apiKey" || obj["context"] != "billing" || obj["polyType"] != "variable" {
		t.Fatalf("%+v", obj)
	}
	if obj["description"] != "from snippet" {
		t.Fatalf("expected snippet fields kept: %+v", obj)
	}
	if !strings.Contains(stdout, rel) {
		t.Fatalf("%q", stdout)
	}
}

func TestVariInitRejectsTypeScriptSnippet(t *testing.T) {
	dir := t.TempDir()
	fake := newSnippetFake()
	fake.put("s1", "templates", "header", map[string]any{
		"language": "typescript",
		"code":     "export const n = 1;\n",
	})
	srv := startSnippetAPI(t, fake)
	snippetEnv(t, dir, srv.URL)

	_, stderr, code := run("vari", "init", "--name", "apiKey", "--snippet", "templates.header")
	if code != exitcode.Failure {
		t.Fatalf("exit %d stderr=%q", code, stderr)
	}
	if !strings.Contains(stderr, "typescript") || !strings.Contains(stderr, "json") {
		t.Fatalf("expected language mismatch: %q", stderr)
	}
}

func TestInitSnippetFetchErrorHalts(t *testing.T) {
	dir := t.TempDir()
	fake := newSnippetFake()
	srv := startSnippetAPI(t, fake)
	snippetEnv(t, dir, srv.URL)

	_, stderr, code := run("function", "init", "--name", "helloWorld", "--type", "server", "--snippet", "missing.template")
	if code == 0 {
		t.Fatalf("expected failure, stderr=%q", stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "src/server/helloWorld.ts")); err == nil {
		t.Fatal("must not write a file when fetch fails")
	}
}

func TestInitSnippetUniqueNameAndAmbiguous(t *testing.T) {
	dir := t.TempDir()
	fake := newSnippetFake()
	fake.put("s1", "templates", "header", map[string]any{
		"language": "json",
		"code":     `{"name":"header","description":"one"}`,
	})
	srv := startSnippetAPI(t, fake)
	snippetEnv(t, dir, srv.URL)

	stdout, stderr, code := run("vari", "init", "--name", "apiKey", "--context", "billing", "--snippet", "header")
	if code != 0 {
		t.Fatalf("unique name exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}

	fake.put("s2", "maps", "header", map[string]any{
		"language": "json",
		"code":     `{"name":"header","description":"two"}`,
	})
	_, stderr, code = run("vari", "init", "--name", "other", "--snippet", "header")
	if code != exitcode.Usage {
		t.Fatalf("ambiguous exit %d stderr=%q", code, stderr)
	}
	if !strings.Contains(strings.ToLower(stderr), "unclear snippet reference") {
		t.Fatalf("stderr=%q", stderr)
	}
	// init --context is the new resource, not the snippet context
	_, stderr, code = run("vari", "init", "--name", "other", "--context", "templates", "--snippet", "header")
	if code != exitcode.Usage {
		t.Fatalf("init --context must not disambiguate snippet: exit %d stderr=%q", code, stderr)
	}
}

func TestInitSnippetByID(t *testing.T) {
	dir := t.TempDir()
	id := "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	fake := newSnippetFake()
	fake.put(id, "templates", "variable", map[string]any{
		"language": "jsonc",
		"code":     `{"name":"template","value":{}}`,
	})
	srv := startSnippetAPI(t, fake)
	snippetEnv(t, dir, srv.URL)

	stdout, stderr, code := run("vari", "init", "--name", "apiKey", "--snippet", id)
	if code != 0 {
		t.Fatalf("exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "src/artifacts/vari/apiKey.jsonc") {
		t.Fatalf("%q", stdout)
	}
}

func TestInitSnippetRequiresCredentials(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	_, stderr, code := run("vari", "init", "--name", "apiKey", "--snippet", "templates.variable")
	if code != exitcode.Auth {
		t.Fatalf("exit %d stderr=%q", code, stderr)
	}
}

func TestInitSnippetJavascriptMatchesTypeScript(t *testing.T) {
	dir := t.TempDir()
	fake := newSnippetFake()
	fake.put("s1", "templates", "helloWorld", map[string]any{
		"language": "javascript",
		"code":     tsSnippetBody,
	})
	srv := startSnippetAPI(t, fake)
	snippetEnv(t, dir, srv.URL)

	stdout, stderr, code := run("function", "init", "--name", "helloWorld", "--type", "server", "--snippet", "templates.helloWorld")
	if code != 0 {
		t.Fatalf("exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "helloWorld.ts") {
		t.Fatalf("%q", stdout)
	}
}

func TestTriggerInitSnippetMustMatchType(t *testing.T) {
	dir := t.TempDir()
	fake := newSnippetFake()
	fake.put("s1", "templates", "weekly", map[string]any{
		"language": "json",
		"code":     `{"name":"weekly","source":{"webhookHandleId":"billing.hook"},"destination":{"serverFunctionId":"billing.fn"},"waitForResponse":true}`,
	})
	fake.put("s2", "templates", "on-error", map[string]any{
		"language": "json",
		"code":     `{"name":"on-error","source":{"errorHandler":{"path":"billing.handle"}},"destination":{"serverFunctionId":"billing.fn"}}`,
	})
	srv := startSnippetAPI(t, fake)
	snippetEnv(t, dir, srv.URL)

	stdout, stderr, code := run("trigger", "init", "--name", "weekly", "--type", "webhook", "--snippet", "templates.weekly")
	if code != 0 {
		t.Fatalf("match exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	wh := readInitJSONC(t, dir, "src/artifacts/triggers/weekly.jsonc")
	src, _ := wh["source"].(map[string]any)
	if src["webhookHandleId"] != "billing.hook" {
		t.Fatalf("webhook snippet source=%v", src)
	}

	_, stderr, code = run("trigger", "init", "--name", "mismatch", "--type", "error-handler", "--snippet", "templates.weekly")
	if code != exitcode.Usage {
		t.Fatalf("mismatch exit %d stderr=%q", code, stderr)
	}
	if !strings.Contains(stderr, "webhook") || !strings.Contains(stderr, "error-handler") {
		t.Fatalf("stderr=%q", stderr)
	}

	stdout, stderr, code = run("trigger", "init", "--name", "on-error", "--type", "error-handler", "--snippet", "templates.on-error")
	if code != 0 {
		t.Fatalf("eh match exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	eh := readInitJSONC(t, dir, "src/artifacts/triggers/on-error.jsonc")
	src, _ = eh["source"].(map[string]any)
	handler, _ := src["errorHandler"].(map[string]any)
	if handler["path"] != "billing.handle" {
		t.Fatalf("error-handler snippet source=%v", src)
	}
}

func TestSubscriptionInitSnippetMustMatchType(t *testing.T) {
	dir := t.TempDir()
	fake := newSnippetFake()
	fake.put("s1", "templates", "customSub", map[string]any{
		"language": "json",
		"code":     `{"name":"old","context":"old","type":"CUSTOM","websocketUrl":"wss://x","query":"subscription { x }","functionId":"a.b"}`,
	})
	fake.put("s2", "templates", "ohipSub", map[string]any{
		"language": "json",
		"code":     `{"name":"old","context":"old","type":"OHIP","websocketUrl":"wss://x","query":"subscription { x }","functionId":"a.b"}`,
	})
	srv := startSnippetAPI(t, fake)
	snippetEnv(t, dir, srv.URL)

	stdout, stderr, code := run("subscription", "init", "--name", "ordersStream", "--type", "CUSTOM", "--snippet", "templates.customSub")
	if code != 0 {
		t.Fatalf("match exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	got := readInitJSONC(t, dir, "src/artifacts/subscriptions/ordersStream.jsonc")
	if got["name"] != "ordersStream" || got["type"] != "CUSTOM" {
		t.Fatalf("%+v", got)
	}

	_, stderr, code = run("subscription", "init", "--name", "mismatch", "--type", "OHIP", "--snippet", "templates.customSub")
	if code != exitcode.Usage {
		t.Fatalf("mismatch exit %d stderr=%q", code, stderr)
	}
	if !strings.Contains(stderr, "CUSTOM") || !strings.Contains(stderr, "OHIP") {
		t.Fatalf("stderr=%q", stderr)
	}
}

func TestInitSnippetEmptyCodeHalts(t *testing.T) {
	dir := t.TempDir()
	fake := newSnippetFake()
	fake.put("s1", "templates", "helloWorld", map[string]any{
		"language": "typescript",
		"code":     "   ",
	})
	srv := startSnippetAPI(t, fake)
	snippetEnv(t, dir, srv.URL)

	_, stderr, code := run("function", "init", "--name", "helloWorld", "--type", "server", "--snippet", "templates.helloWorld")
	if code != exitcode.Failure {
		t.Fatalf("exit %d stderr=%q", code, stderr)
	}
	if !strings.Contains(stderr, "has no code") {
		t.Fatalf("stderr=%q", stderr)
	}
}
