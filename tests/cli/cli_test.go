package cli_test

import (
	"bytes"
	"fmt"
	"image/color"
	"regexp"
	"strings"
	"testing"

	"github.com/polyapi/polyglot/src/cli"
	"github.com/polyapi/polyglot/src/exitcode"
)

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func plain(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}

func rgbSeq(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("38;2;%d;%d;%d", r>>8, g>>8, b>>8)
}

func run(args ...string) (stdout, stderr string, code int) {
	var out, err bytes.Buffer
	code = cli.Run(args, &out, &err)
	return out.String(), err.String(), code
}

func TestThemePaint(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "1")
	s := cli.Header("polyapi")
	if !strings.Contains(s, "polyapi") || !strings.Contains(s, "\x1b") {
		t.Fatalf("expected ANSI in %q", s)
	}
	for _, action := range []string{
		"created", "updated", "skipped", "blocked", "failed", "deleted",
		"orphan", "adopted", "would_create", "would_update", "would_delete",
	} {
		if _, ok := cli.LookupStatusTag(action); !ok {
			t.Errorf("missing status tag %s", action)
		}
	}
	if _, ok := cli.LookupStatusTag("nope"); ok {
		t.Fatal("expected no tag")
	}
	var buf bytes.Buffer
	cli.PrintOk(&buf, "generated")
	cli.PrintWarning(&buf, "heads up")
	if !strings.Contains(buf.String(), "✓ OK:") {
		t.Fatalf("%q", buf.String())
	}
}

func TestHelpUsesPolyBrandColors(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "1")
	stdout, _, code := run("--help")
	if code != 0 {
		t.Fatalf("help exit %d", code)
	}
	for _, seq := range []string{rgbSeq(cli.River600), rgbSeq(cli.Sunray600), rgbSeq(cli.Stone500), rgbSeq(cli.Macaw500)} {
		if !strings.Contains(stdout, seq) {
			t.Errorf("expected %s in help:\n%s", seq, stdout)
		}
	}
}

func TestRootHelpListsCoreCommands(t *testing.T) {
	stdout, _, code := run("--help")
	if code != 0 {
		t.Fatalf("help exit %d", code)
	}
	for _, cmd := range []string{
		"auth", "config", "generate", "deploy", "function", "vari", "init",
		"doctor", "version", "update",
	} {
		if !strings.Contains(stdout, cmd) {
			t.Errorf("missing %q in help:\n%s", cmd, stdout)
		}
	}
}

func TestTopLevelCommandSurface(t *testing.T) {
	root := cli.NewRoot()
	got := map[string]bool{}
	for _, c := range root.Commands() {
		if c.Hidden || c.Name() == "help" || c.Name() == "completion" {
			continue
		}
		got[c.Name()] = true
	}
	for _, name := range []string{
		"init", "auth", "config", "generate", "deploy",
		"function", "vari", "table", "webhook", "trigger", "job", "schema", "snippet", "subscription", "app", "version",
	} {
		if !got[name] {
			t.Errorf("missing top-level %s", name)
		}
	}
	for _, name := range []string{
		"setup", "login", "logout", "whoami",
		"prepare", "validate", "plan", "push", "pull", "sync", "env-check",
	} {
		if got[name] {
			t.Errorf("%s should not be top-level", name)
		}
	}
}

func TestHelpIncludesExamples(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "1")
	t.Setenv("__FANG_TEST_WIDTH", "120")
	cases := []struct {
		args []string
		want []string
	}{
		{[]string{"--help"}, []string{
			"EXAMPLES",
			"polyapi auth login na1",
			"polyapi generate",
			"polyapi deploy plan --env prod",
			"polyapi deploy push --env prod",
		}},
		{[]string{"auth", "login", "--help"}, []string{
			"polyapi auth login na1",
			"polyapi auth login --base-url https://eu1.polyapi.io",
		}},
		{[]string{"generate", "--help"}, []string{
			"polyapi generate --contexts billing",
			"polyapi generate --lang typescript",
		}},
		{[]string{"deploy", "push", "--help"}, []string{
			"polyapi deploy push --env prod",
			"polyapi deploy sync --env prod",
			"polyapi deploy push --delete-orphans --yes --env prod",
		}},
		{[]string{"function", "add", "--help"}, []string{
			"polyapi function add echo echo.ts --type server --context billing",
			"--skip-generate",
			"(required)",
		}},
		{[]string{"function", "list", "--help"}, []string{
			"polyapi function list --context billing",
			"--type",
		}},
		{[]string{"function", "delete", "--help"}, []string{
			"polyapi function delete echo --context billing --type server",
		}},
		{[]string{"function", "execute", "--help"}, []string{
			"--data",
			"polyapi function execute echo --context billing",
		}},
		{[]string{"function", "logs", "--help"}, []string{
			"--keyword",
			"--delete",
		}},
		{[]string{"job", "create", "--help"}, []string{
			"--name",
			"(required)",
			"--function",
		}},
		{[]string{"schema", "create", "--help"}, []string{
			"--name",
			"(required)",
			"--definition",
			"polyapi schema create --name Order --context billing",
		}},
		{[]string{"snippet", "add", "--help"}, []string{
			"--context",
			"(required)",
			"--language",
			"polyapi snippet add header snippets/header.ts --context billing",
		}},
		{[]string{"model", "generate", "--help"}, []string{
			"polyapi model generate openapi.yaml spec.json --context billing",
			"--host-url-as-argument",
		}},
		{[]string{"vari", "list", "--help"}, []string{
			"polyapi vari list --context billing",
		}},
		{[]string{"vari", "create", "--help"}, []string{
			"--name",
			"(required)",
			"--context",
			"--value",
			"polyapi vari create --name apiKey --context billing",
		}},
		{[]string{"vari", "init", "--help"}, []string{
			"--name",
			"--context",
			"(required)",
			"--path",
			"polyapi vari init --name example",
		}},
		{[]string{"function", "init", "--help"}, []string{
			"--type",
			"(required)",
			"polyapi function init --name helloWorld --type server",
		}},
		{[]string{"job", "init", "--help"}, []string{
			"--name",
			"(required)",
			"polyapi job init --name example",
		}},
		{[]string{"app", "create", "--help"}, []string{
			"--name",
			"(required)",
			"--config",
			"polyapi app create --name dashboard",
		}},
		{[]string{"app", "init", "--help"}, []string{
			"--name",
			"(required)",
			"polyapi app init --name example",
		}},
		{[]string{"vari", "copy", "--help"}, []string{
			"--to-context",
			"polyapi vari copy apiKey --context billing --to-context staging",
		}},
		{[]string{"doctor", "--help"}, []string{
			"polyapi doctor --offline",
			"--offline",
		}},
		{[]string{"update", "--help"}, []string{
			"polyapi update --check",
			"--check",
		}},
	}
	for _, tc := range cases {
		stdout, _, code := run(tc.args...)
		if code != 0 {
			t.Errorf("%v: help exit %d", tc.args, code)
			continue
		}
		got := plain(stdout)
		for _, want := range tc.want {
			if !strings.Contains(got, want) {
				t.Errorf("%v: missing %q in help:\n%s", tc.args, want, got)
			}
		}
	}
}

func TestSyncIsPushAlias(t *testing.T) {
	cmd, _, err := cli.NewRoot().Find([]string{"deploy", "sync"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Name() != "push" {
		t.Fatalf("sync should resolve to push, got %q", cmd.Name())
	}
}

func TestEnvCheckParses(t *testing.T) {
	cmd, _, err := cli.NewRoot().Find([]string{"deploy", "env-check"})
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Parse([]string{"--strict", "--online"}); err != nil {
		t.Fatal(err)
	}
	strict, _ := cmd.Flags().GetBool("strict")
	online, _ := cmd.Flags().GetBool("online")
	if !strict || !online {
		t.Fatalf("strict=%v online=%v", strict, online)
	}
}

func TestFunctionAddHelp(t *testing.T) {
	stdout, _, code := run("function", "add", "--help")
	if code != 0 {
		t.Fatalf("help exit %d", code)
	}
	if !strings.Contains(stdout, "--type") {
		t.Fatalf("expected --type in help:\n%s", stdout)
	}
}

func TestVariListHelp(t *testing.T) {
	_, _, code := run("vari", "list", "--help")
	if code != 0 {
		t.Fatalf("help exit %d", code)
	}
}

func TestUnknownCommandFails(t *testing.T) {
	_, stderr, code := run("nope")
	if code != exitcode.Usage {
		t.Fatalf("exit %d, want %d; stderr=%s", code, exitcode.Usage, stderr)
	}
}

func TestVersionFlag(t *testing.T) {
	stdout, _, code := run("--version")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	got := plain(stdout)
	if !strings.Contains(got, "0.1.0") || !strings.Contains(got, "polyapi") {
		t.Fatalf("version output: %q", stdout)
	}
	if !strings.Contains(got, "protocol") || !strings.Contains(got, "platform") {
		t.Fatalf("missing build metadata:\n%s", got)
	}
}

func TestVersionSubcommand(t *testing.T) {
	stdout, _, code := run("version")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	got := plain(stdout)
	if !strings.Contains(got, "0.1.0") {
		t.Fatalf("version output: %q", stdout)
	}
	if !strings.Contains(got, "commit") || !strings.Contains(got, "protocol") || !strings.Contains(got, "  1") {
		t.Fatalf("missing metadata:\n%s", got)
	}
}

func TestInitHelp(t *testing.T) {
	stdout, _, code := run("init", "--help")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	got := plain(stdout)
	for _, s := range []string{
		"--template", "--provider", "--existing", "--no-deps",
		"polyapi init --lang typescript --template none --yes",
	} {
		if !strings.Contains(got, s) {
			t.Errorf("missing %q in init help:\n%s", s, got)
		}
	}
}

func TestGenerateFiltersParse(t *testing.T) {
	root := cli.NewRoot()
	root.SetArgs([]string{"generate", "--contexts", "billing,maps", "--names", "apiKey", "--ids", "abc,def", "--no-types"})
	_ = root.Execute()
	gen, _, err := root.Find([]string{"generate"})
	if err != nil {
		t.Fatal(err)
	}
	contexts, _ := gen.Flags().GetStringSlice("contexts")
	names, _ := gen.Flags().GetStringSlice("names")
	ids, _ := gen.Flags().GetStringSlice("ids")
	noTypes, _ := gen.Flags().GetBool("no-types")
	if strings.Join(contexts, ",") != "billing,maps" {
		t.Fatalf("contexts=%v", contexts)
	}
	if strings.Join(names, ",") != "apiKey" {
		t.Fatalf("names=%v", names)
	}
	if strings.Join(ids, ",") != "abc,def" {
		t.Fatalf("ids=%v", ids)
	}
	if !noTypes {
		t.Fatal("expected --no-types")
	}
}

func TestValidateOnlyEnv(t *testing.T) {
	root := cli.NewRoot()
	root.SetArgs([]string{"deploy", "validate", "--only", "env", "--strict"})
	_ = root.Execute()
	cmd, _, err := root.Find([]string{"deploy", "validate"})
	if err != nil {
		t.Fatal(err)
	}
	only, _ := cmd.Flags().GetStringSlice("only")
	strict, _ := cmd.Flags().GetBool("strict")
	if strings.Join(only, ",") != "env" {
		t.Fatalf("only=%v", only)
	}
	if !strict {
		t.Fatal("expected --strict")
	}
}

func TestJobCreateRequiresNameAndContext(t *testing.T) {
	_, stderr, code := run("job", "create")
	if code != exitcode.Usage {
		t.Fatalf("exit %d, want usage; stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "name") && !strings.Contains(plain(stderr), "required") {
		t.Fatalf("expected required-flag error, stderr=%s", stderr)
	}
}

func TestFunctionAddRequiresType(t *testing.T) {
	_, stderr, code := run("function", "add", "echo", "echo.ts")
	if code != exitcode.Usage {
		t.Fatalf("exit %d, want usage; stderr=%s", code, stderr)
	}
}

func TestFunctionAddParsesType(t *testing.T) {
	root := cli.NewRoot()
	root.SetArgs([]string{"function", "add", "echo", "echo.ts", "--type", "server", "--context", "test"})
	_ = root.Execute()
	cmd, _, err := root.Find([]string{"function", "add"})
	if err != nil {
		t.Fatal(err)
	}
	kind := cmd.Flags().Lookup("type").Value.String()
	context, _ := cmd.Flags().GetString("context")
	if kind != "server" || context != "test" {
		t.Fatalf("type=%q context=%q", kind, context)
	}
}

func TestLangAcceptsTypescriptAndTs(t *testing.T) {
	for _, flag := range []string{"typescript", "ts"} {
		root := cli.NewRoot()
		root.SetArgs([]string{"--lang", flag, "version"})
		if err := root.Execute(); err != nil {
			t.Fatalf("--lang %s: %v", flag, err)
		}
		got := root.PersistentFlags().Lookup("lang").Value.String()
		if got != "typescript" {
			t.Fatalf("--lang %s stored %q", flag, got)
		}
	}
}

func TestGlobalEnvAfterSubcommand(t *testing.T) {
	root := cli.NewRoot()
	root.SetArgs([]string{"deploy", "plan", "--env", "prod"})
	_ = root.Execute()
	got, _ := root.PersistentFlags().GetString("env")
	if got != "prod" {
		t.Fatalf("env=%q", got)
	}
}

func TestTableRowsListParses(t *testing.T) {
	cmd, _, err := cli.NewRoot().Find([]string{"table", "rows", "list"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Name() != "list" {
		t.Fatalf("got %q", cmd.Name())
	}
}
