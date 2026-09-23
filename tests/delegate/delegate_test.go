package delegate_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/polyapi/polyglot/src/delegate"
	"github.com/polyapi/polyglot/src/exitcode"
)

func TestLanguageAliases(t *testing.T) {
	ts, ok := delegate.ParseLanguage("ts")
	if !ok || ts != delegate.LangTypeScript {
		t.Fatal(ts)
	}
	py, ok := delegate.ParseLanguage("Python")
	if !ok || py != delegate.LangPython {
		t.Fatal(py)
	}
	java, ok := delegate.ParseLanguage("JAVA")
	if !ok || java != delegate.LangJava {
		t.Fatal(java)
	}
	if _, ok := delegate.ParseLanguage("nope"); ok {
		t.Fatal("expected none")
	}
}

func TestOpNamesAreSnakeCase(t *testing.T) {
	b, _ := json.Marshal(delegate.OpWriteReceipt)
	if string(b) != `"write_receipt"` {
		t.Fatalf("%s", b)
	}
	b, _ = json.Marshal(delegate.OpParseFunction)
	if string(b) != `"parse_function"` {
		t.Fatalf("%s", b)
	}
	var op delegate.Op
	if err := json.Unmarshal([]byte(`"discover"`), &op); err != nil || op != delegate.OpDiscover {
		t.Fatal(op, err)
	}
}

func TestCapabilitiesCompat(t *testing.T) {
	ok := delegate.Capabilities{Protocol: 1, MinProtocol: 1, Ops: []delegate.Op{delegate.OpGenerate}, Lang: "python", SDKVersion: "1.0.0"}
	if !ok.CompatibleWithHost() || !ok.Supports(delegate.OpGenerate) || !ok.Supports(delegate.OpCapabilities) {
		t.Fatal(ok)
	}
	old := delegate.Capabilities{Protocol: 0, MinProtocol: 0, Lang: "python", SDKVersion: "0.1.0"}
	if old.CompatibleWithHost() {
		t.Fatal("old should be incompatible")
	}
}

func TestDeployableDescRoundTrip(t *testing.T) {
	desc := delegate.DeployableDesc{
		Type: "variable", Name: "apiKey", Context: "billing",
		File: "src/billing/vari/apiKey.ts", Export: "polyConfig",
	}
	v, err := json.Marshal(desc)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	json.Unmarshal(v, &m)
	if m["type"] != "variable" || m["export"] != "polyConfig" {
		t.Fatalf("%s", v)
	}
	var back delegate.DeployableDesc
	if err := json.Unmarshal(v, &back); err != nil || back != desc {
		t.Fatalf("%+v %v", back, err)
	}
}

func TestProtocolMismatchIs11(t *testing.T) {
	err := &delegate.Error{Kind: "protocol", Code: exitcode.Protocol, Msg: "adapter protocol 0"}
	if err.ExitCode() != exitcode.Protocol {
		t.Fatalf("%d", err.ExitCode())
	}
}

func TestMissingAdapterIs10(t *testing.T) {
	err := delegate.MissingCommand([]string{"npx", "--no-install", "poly", "adapter"}, delegate.LangTypeScript)
	if err.ExitCode() != exitcode.AdapterMissing {
		t.Fatalf("%d", err.ExitCode())
	}
	if !strings.Contains(err.Error(), "npm install polyapi") {
		t.Fatalf("%s", err)
	}
}

func TestVersionConstantsMatch(t *testing.T) {
	if delegate.Version != 1 || delegate.Protocol != 1 {
		t.Fatalf("version=%d protocol=%d", delegate.Version, delegate.Protocol)
	}
}

func TestProtocolFixturesDeserialize(t *testing.T) {
	dir := filepath.Join(repoRoot(t), "tests", "fixtures", "delegate", "protocol", "v1")
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	load := func(name string) json.RawMessage {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	var sawRequest, sawResponse int
	for _, ent := range ents {
		name := ent.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		raw := load(name)
		switch {
		case strings.HasPrefix(name, "request-"):
			var req delegate.Request
			if err := json.Unmarshal(raw, &req); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if req.Protocol != delegate.Protocol || req.Op == "" {
				t.Fatalf("%s: %+v", name, req)
			}
			sawRequest++
		case strings.HasPrefix(name, "response-"):
			var resp delegate.Response
			if err := json.Unmarshal(raw, &resp); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if !resp.OK || resp.Op == "" {
				t.Fatalf("%s: %+v", name, resp)
			}
			sawResponse++
		default:
			t.Fatalf("unexpected fixture %s", name)
		}
	}
	if sawRequest < 7 || sawResponse < 7 {
		t.Fatalf("request=%d response=%d", sawRequest, sawResponse)
	}

	var resp delegate.Response
	if err := json.Unmarshal(load("response-generate.json"), &resp); err != nil {
		t.Fatal(err)
	}
	var gen delegate.GenerateResult
	if err := json.Unmarshal(resp.Result, &gen); err != nil || len(gen.FilesWritten) == 0 {
		t.Fatalf("%+v %v", gen, err)
	}
	var disc delegate.Response
	if err := json.Unmarshal(load("response-discover.json"), &disc); err != nil {
		t.Fatal(err)
	}
	var found delegate.DiscoverResult
	json.Unmarshal(disc.Result, &found)
	if found.Deployables[0].Type != "variable" || found.Deployables[0].Export != "polyConfig" {
		t.Fatalf("%+v", found.Deployables[0])
	}
	var ext delegate.Response
	json.Unmarshal(load("response-extract.json"), &ext)
	var extracted delegate.ExtractResult
	json.Unmarshal(ext.Result, &extracted)
	payload, _ := extracted.Payload.(map[string]any)
	if payload == nil || payload["name"] == nil || payload["code"] == nil || payload["language"] == nil {
		t.Fatalf("extract payload must be a REST function DTO, got %+v", extracted)
	}
	if _, ok := payload["type"]; ok {
		t.Fatalf("extract payload must omit type, got %+v", payload)
	}
	var inspectResp delegate.Response
	json.Unmarshal(load("response-inspect.json"), &inspectResp)
	var inspected delegate.InspectResult
	if err := json.Unmarshal(inspectResp.Result, &inspected); err != nil || len(inspected.Items) == 0 {
		t.Fatalf("%+v %v", inspected, err)
	}
	var prepResp delegate.Response
	json.Unmarshal(load("response-prepare.json"), &prepResp)
	var prepared delegate.PrepareResult
	if err := json.Unmarshal(prepResp.Result, &prepared); err != nil {
		t.Fatalf("%+v %v", prepared, err)
	}
	var capsResp delegate.Response
	json.Unmarshal(load("response-capabilities.json"), &capsResp)
	var caps delegate.Capabilities
	json.Unmarshal(capsResp.Result, &caps)
	if caps.Protocol != delegate.Protocol {
		t.Fatalf("%+v", caps)
	}
	for _, op := range delegate.RequiredForTSPython() {
		if op == delegate.OpCapabilities {
			continue
		}
		if !caps.Supports(op) {
			t.Fatalf("missing %s in %+v", op, caps.Ops)
		}
	}
}

type scriptedRunner struct {
	handle func(delegate.Job) delegate.RawOutput
}

func (s scriptedRunner) Run(job delegate.Job) (delegate.RawOutput, error) { return s.handle(job), nil }

func scripted(f func(delegate.Job) delegate.RawOutput) delegate.Delegate {
	return delegate.New("/tmp/proj", delegate.LangPython, []string{"python", "-m", "polyapi", "adapter"}, ".poly").
		WithRunner(scriptedRunner{handle: f})
}

func TestGenerateAndDiscoverViaScriptedAdapter(t *testing.T) {
	del := scripted(func(job delegate.Job) delegate.RawOutput {
		for _, pair := range job.ExtraEnv {
			if pair[0] == "POLY_API_KEY" {
				t.Errorf("unexpected key inject")
			}
		}
		op, id := requestIDFromStdin(job.Stdin)
		var result any
		switch op {
		case "capabilities":
			result = map[string]any{
				"protocol": 1, "min_protocol": 1,
				"ops":  []string{"capabilities", "generate", "discover", "inspect", "extract", "prepare", "write_receipt", "parse_function", "clear"},
				"lang": "python", "sdk_version": "fixture-1",
			}
		case "generate":
			result = map[string]any{"files_written": []string{"polyapi/index.py"}, "stats": map[string]any{"files": 1}}
		case "discover":
			result = map[string]any{"deployables": []any{map[string]any{
				"type": "variable", "name": "apiKey", "context": "billing",
				"file": "src/billing/vari/apiKey.py", "export": "polyConfig",
				"content_hash": nil, "receipt": nil,
			}}}
		case "extract":
			result = map[string]any{"payload": map[string]any{"name": "apiKey"}, "content_hash": "abc", "meta": map[string]any{}}
		default:
			result = map[string]any{"ok": true}
		}
		zero := 0
		return delegate.RawOutput{Status: &zero, Stdout: okEnvelope(op, id, result)}
	})
	generated, err := del.Generate(delegate.GenerateParams{Specs: delegate.SpecsJSON([]any{map[string]any{"name": "apiKey"}})})
	if err != nil {
		t.Fatal(err)
	}
	if len(generated.FilesWritten) != 1 || generated.FilesWritten[0] != "polyapi/index.py" {
		t.Fatalf("%+v", generated)
	}
	disc, err := del.Discover(delegate.DiscoverParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(disc.Deployables) != 1 || disc.Deployables[0].Type != "variable" || disc.Deployables[0].Name != "apiKey" {
		t.Fatalf("%+v", disc)
	}
	extracted, err := del.Extract(delegate.ExtractParams{File: "src/billing/vari/apiKey.py", Type: "variable"})
	if err != nil {
		t.Fatal(err)
	}
	if extracted.Payload.(map[string]any)["name"] != "apiKey" || extracted.ContentHash != "abc" {
		t.Fatalf("%+v", extracted)
	}
}

func TestScriptedProtocolMismatch(t *testing.T) {
	del := scripted(func(job delegate.Job) delegate.RawOutput {
		_, id := requestIDFromStdin(job.Stdin)
		zero := 0
		return delegate.RawOutput{Status: &zero, Stdout: okEnvelope("capabilities", id, map[string]any{
			"protocol": 0, "min_protocol": 0, "ops": []string{"generate"}, "lang": "python", "sdk_version": "0.0.1",
		})}
	})
	_, err := del.Generate(delegate.GenerateParams{})
	if err == nil || err.(*delegate.Error).ExitCode() != exitcode.Protocol {
		t.Fatalf("%v", err)
	}
	if !strings.Contains(err.Error(), "too old") {
		t.Fatalf("%v", err)
	}
}

func TestMissingOpIsSDKTooOld(t *testing.T) {
	del := scripted(func(job delegate.Job) delegate.RawOutput {
		_, id := requestIDFromStdin(job.Stdin)
		zero := 0
		return delegate.RawOutput{Status: &zero, Stdout: okEnvelope("capabilities", id, map[string]any{
			"protocol": 1, "min_protocol": 1, "ops": []string{"capabilities"}, "lang": "python", "sdk_version": "0.0.1",
		})}
	})
	_, err := del.Generate(delegate.GenerateParams{})
	if err == nil || err.(*delegate.Error).ExitCode() != exitcode.Protocol {
		t.Fatalf("%v", err)
	}
	if !strings.Contains(err.Error(), "does not support `generate`") {
		t.Fatalf("%v", err)
	}
}

func TestModuleNotFoundIs10(t *testing.T) {
	one := 1
	del := scripted(func(delegate.Job) delegate.RawOutput {
		return delegate.RawOutput{Status: &one, Stderr: "ModuleNotFoundError: No module named 'polyapi'"}
	})
	_, err := del.Capabilities()
	if err == nil || err.(*delegate.Error).ExitCode() != exitcode.AdapterMissing {
		t.Fatalf("%v", err)
	}
}

func TestGenerateDoesNotInjectCredentials(t *testing.T) {
	var seen string
	var mu sync.Mutex
	del := scripted(func(job delegate.Job) delegate.RawOutput {
		for _, pair := range job.ExtraEnv {
			if pair[0] == "POLY_API_KEY" {
				mu.Lock()
				seen = pair[1]
				mu.Unlock()
			}
		}
		op, id := requestIDFromStdin(job.Stdin)
		var result any
		if op == "capabilities" {
			result = map[string]any{"protocol": 1, "min_protocol": 1, "ops": []string{"generate"}, "lang": "typescript", "sdk_version": "1"}
		} else {
			result = map[string]any{"files_written": []any{}, "stats": map[string]any{}}
		}
		zero := 0
		return delegate.RawOutput{Status: &zero, Stdout: okEnvelope(op, id, result)}
	}).WithCreds(delegate.AdapterCreds{APIKey: "secret-key", BaseURL: "https://na1.polyapi.io"})
	if _, err := del.Generate(delegate.GenerateParams{}); err != nil {
		t.Fatal(err)
	}
	if seen != "" {
		t.Fatalf("adapter must not receive POLY_API_KEY, got %q", seen)
	}
}

func TestParseJSONUsesLastObject(t *testing.T) {
	v, err := delegate.ParseJSONDocument("log line\n{\"ok\":true,\"n\":1}\n")
	if err != nil {
		t.Fatal(err)
	}
	if v.(map[string]any)["n"].(float64) != 1 {
		t.Fatalf("%v", v)
	}
}

func TestDiscoverOrder(t *testing.T) {
	dir := t.TempDir()
	input := delegate.DiscoverInput{ProjectRoot: dir, NonInteractive: true}

	resolved, err := delegate.Discover(delegate.DiscoverInput{AdapterFlag: "node ./adapter.js", LangFlag: delegate.LangPython, ProjectRoot: dir, NonInteractive: true})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Source != delegate.AdapterFlag || resolved.Lang != delegate.LangPython || resolved.Argv[0] != "node" {
		t.Fatalf("%+v", resolved)
	}
	if !samePath(t, resolved.Root, dir) {
		t.Fatalf("root=%s want %s", resolved.Root, dir)
	}

	_, err = delegate.Discover(delegate.DiscoverInput{LangFlag: delegate.LangPython, ProjectRoot: dir, NonInteractive: true})
	if err == nil || err.(*delegate.Error).Kind != "missing" || err.(*delegate.Error).ExitCode() != exitcode.AdapterMissing {
		t.Fatalf("expected missing SDK, got %v", err)
	}

	if err := os.Mkdir(filepath.Join(dir, "polyapi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "polyapi", "__init__.py"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"demo\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, err = delegate.Discover(delegate.DiscoverInput{ConfigLanguage: delegate.LangPython, ProjectRoot: dir, NonInteractive: true})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Lang != delegate.LangPython || resolved.Source != delegate.AdapterConfig {
		t.Fatalf("%+v", resolved)
	}

	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = delegate.Discover(input)
	if err == nil || err.(*delegate.Error).Kind != "ambiguous" {
		t.Fatalf("expected ambiguous after adding package.json, got %v", err)
	}
}

func TestLocalAdapterJSPreferred(t *testing.T) {
	dir := t.TempDir()
	js := filepath.Join(dir, "node_modules", "polyapi", "build", "adapter.js")
	if err := os.MkdirAll(filepath.Dir(js), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(js, []byte("/* fixture */\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"polyapi":"1.0.0"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, err := delegate.Discover(delegate.DiscoverInput{ProjectRoot: dir, NonInteractive: true})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Argv[0] != "node" || !samePath(t, resolved.Argv[1], js) {
		t.Fatalf("%v want node %s", resolved.Argv, js)
	}
}

func TestPyprojectIsPython(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"demo\"\ndependencies = [\"polyapi>=1\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	langs := delegate.DetectLanguages(dir)
	if len(langs) != 1 || langs[0] != delegate.LangPython {
		t.Fatalf("%v", langs)
	}
	_, err := delegate.Discover(delegate.DiscoverInput{ProjectRoot: dir, NonInteractive: true})
	if err == nil || err.(*delegate.Error).Kind != "missing" {
		t.Fatalf("expected missing SDK, got %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "polyapi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "polyapi", "__init__.py"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, err := delegate.Discover(delegate.DiscoverInput{ProjectRoot: dir, NonInteractive: true})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Lang != delegate.LangPython {
		t.Fatalf("%+v", resolved)
	}
}

func TestLockfileIsTypeScript(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte(`{"packages":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	langs := delegate.DetectLanguages(dir)
	if len(langs) != 1 || langs[0] != delegate.LangTypeScript {
		t.Fatalf("%v", langs)
	}
	_, err := delegate.Discover(delegate.DiscoverInput{ProjectRoot: dir, NonInteractive: true})
	if err == nil || err.(*delegate.Error).Kind != "missing" || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("expected missing SDK, got %v", err)
	}
}

func TestPackageJSONWithoutSDKIsMissing(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := delegate.Discover(delegate.DiscoverInput{ProjectRoot: dir, NonInteractive: true})
	if err == nil || err.(*delegate.Error).ExitCode() != exitcode.AdapterMissing {
		t.Fatalf("%v", err)
	}
	if !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("%s", err)
	}
}

func TestLocateProjectWalksParents(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(dir, "src", "billing")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	loc := delegate.LocateProject(nested, ".poly")
	if !loc.Found || !samePath(t, loc.Root, dir) {
		t.Fatalf("%+v want root %s", loc, dir)
	}
	if len(loc.Languages) != 1 || loc.Languages[0] != delegate.LangTypeScript {
		t.Fatalf("%v", loc.Languages)
	}
	js := filepath.Join(dir, "node_modules", "polyapi", "build", "adapter.js")
	if err := os.MkdirAll(filepath.Dir(js), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(js, []byte("/* fixture */\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, err := delegate.Discover(delegate.DiscoverInput{ProjectRoot: nested, NonInteractive: true})
	if err != nil {
		t.Fatal(err)
	}
	if !samePath(t, resolved.Root, dir) || resolved.Lang != delegate.LangTypeScript {
		t.Fatalf("%+v want root %s", resolved, dir)
	}
	if resolved.Argv[0] != "node" || !samePath(t, resolved.Argv[1], js) {
		t.Fatalf("%v want node %s", resolved.Argv, js)
	}
}

func samePath(t *testing.T, a, b string) bool {
	t.Helper()
	norm := func(p string) string {
		x, err := filepath.Abs(p)
		if err != nil {
			return p
		}
		if r, err := filepath.EvalSymlinks(x); err == nil {
			return r
		}
		return x
	}
	return norm(a) == norm(b)
}

func TestInstalledSDKWithoutAdapterIsMissing(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "node_modules", "polyapi"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := delegate.Discover(delegate.DiscoverInput{ProjectRoot: dir, NonInteractive: true})
	if err == nil || err.(*delegate.Error).Kind != "missing" || !strings.Contains(err.Error(), "no adapter") {
		t.Fatalf("%v", err)
	}
}

func TestBothLanguagesAmbiguous(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"polyapi":"1"}}`), 0o644)
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\ndependencies = [\"polyapi\"]\n"), 0o644)
	_, err := delegate.Discover(delegate.DiscoverInput{ProjectRoot: dir, NonInteractive: true})
	if err == nil || err.(*delegate.Error).Kind != "ambiguous" || err.(*delegate.Error).ExitCode() != exitcode.Usage {
		t.Fatalf("%v", err)
	}
}

func TestUndetectedNonInteractive(t *testing.T) {
	dir := t.TempDir()
	_, err := delegate.Discover(delegate.DiscoverInput{ProjectRoot: dir, NonInteractive: true})
	if err == nil || err.(*delegate.Error).Kind != "undetected" {
		t.Fatalf("%v", err)
	}
}

func TestJavaDefaultUnsupported(t *testing.T) {
	dir := t.TempDir()
	_, err := delegate.Discover(delegate.DiscoverInput{ProjectRoot: dir, LangFlag: delegate.LangJava, NonInteractive: true})
	if err == nil || err.(*delegate.Error).Kind != "java" || err.(*delegate.Error).ExitCode() != exitcode.AdapterMissing {
		t.Fatalf("%v", err)
	}
}

func TestSplitCommandQuotes(t *testing.T) {
	argv, err := delegate.SplitCommandLine(`node "./my adapter.js"`)
	if err != nil {
		t.Fatal(err)
	}
	if len(argv) != 2 || argv[0] != "node" || argv[1] != "./my adapter.js" {
		t.Fatalf("%v", argv)
	}
}

func TestMissingBinaryIsExit10(t *testing.T) {
	dir := t.TempDir()
	_, err := delegate.ProcessRunner{}.Run(delegate.Job{
		Argv:    []string{"polyapi-definitely-not-installed-xyz"},
		Cwd:     dir,
		Stdin:   "{}\n",
		Timeout: 2 * time.Second,
	})
	if err == nil || err.(*delegate.Error).ExitCode() != exitcode.AdapterMissing {
		t.Fatalf("%v", err)
	}
}

func TestTimeoutKillsChild(t *testing.T) {
	dir := t.TempDir()
	argv := []string{"sh", "-c", "sleep 8"}
	if runtime.GOOS == "windows" {
		argv = []string{"cmd", "/C", "ping -n 10 127.0.0.1 >NUL"}
	}
	out, err := delegate.ProcessRunner{}.Run(delegate.Job{Argv: argv, Cwd: dir, Timeout: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if !out.TimedOut {
		t.Fatalf("expected timeout, got %+v", out)
	}
}

func TestEchoJSONRoundTrip(t *testing.T) {
	py := delegate.CommandOnPath(delegate.PythonLauncher())
	if py == "" {
		py = delegate.CommandOnPath("python")
	}
	if py == "" {
		py = delegate.CommandOnPath("python3")
	}
	if py == "" {
		t.Skip("python not on PATH")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "echo_adapter.py")
	if err := os.WriteFile(script, []byte("import json,sys\nreq=json.load(sys.stdin)\njson.dump({'ok': True, 'id': req.get('id')}, sys.stdout)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := delegate.ProcessRunner{}.Run(delegate.Job{
		Argv:    []string{py, script},
		Cwd:     dir,
		Stdin:   `{"id":"abc"}`,
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status == nil || *out.Status != 0 {
		t.Fatalf("status %+v stderr=%s", out.Status, out.Stderr)
	}
	if !strings.Contains(out.Stdout, "abc") {
		t.Fatalf("%s", out.Stdout)
	}
}

func okEnvelope(op, id string, result any) string {
	raw, _ := json.Marshal(map[string]any{
		"protocol": 1,
		"id":       id,
		"ok":       true,
		"op":       op,
		"result":   result,
		"warnings": []any{},
		"error":    nil,
	})
	return string(raw)
}

func requestIDFromStdin(stdin string) (op, id string) {
	var req struct {
		Op string `json:"op"`
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(stdin), &req)
	return req.Op, req.ID
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
