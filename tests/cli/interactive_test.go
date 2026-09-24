package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/polyapi/polyglot/src/cli"
	"github.com/polyapi/polyglot/src/exitcode"
)

func TestBarePolyapiWithoutTTYPrintsHelp(t *testing.T) {
	stdout, _, code := run()
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	got := plain(stdout)
	if !strings.Contains(got, "Usage:") && !strings.Contains(got, "USAGE") {
		t.Fatalf("expected help, got:\n%s", got)
	}
	if strings.Contains(got, "▸") {
		t.Fatalf("TUI should not start without a TTY:\n%s", got)
	}
}

func TestSessionIncludesProjectAndLanguage(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"billing"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(dir, "src", "billing")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)
	t.Setenv("POLY_API_KEY", "")
	t.Setenv("POLY_API_BASE_URL", "")
	os.Unsetenv("POLY_API_KEY")
	os.Unsetenv("POLY_API_BASE_URL")

	s := cli.LoadSessionContext(cli.Globals{PolyPath: ".poly"})
	if !s.ProjectFound {
		t.Fatal("expected to find the package.json project")
	}
	if s.ProjectRoot != dir && !sameFile(t, s.ProjectRoot, dir) {
		t.Fatalf("project root %q, want %q", s.ProjectRoot, dir)
	}
	if s.Language != "typescript" {
		t.Fatalf("language %q, want typescript", s.Language)
	}
	if s.LanguageSource != "detected" {
		t.Fatalf("source %q", s.LanguageSource)
	}
	banner := cli.FormatBanner(s)
	if !strings.Contains(banner, "typescript") {
		t.Fatalf("banner missing language: %s", banner)
	}
	if !strings.Contains(banner, cli.CompactPath(dir)) {
		t.Fatalf("banner missing project path %s: %s", cli.CompactPath(dir), banner)
	}
}

func sameFile(t *testing.T, a, b string) bool {
	t.Helper()
	aa, err := filepath.EvalSymlinks(a)
	if err != nil {
		aa = a
	}
	bb, err := filepath.EvalSymlinks(b)
	if err != nil {
		bb = b
	}
	return aa == bb
}

func TestMissingCredentialsFocusBaseURL(t *testing.T) {
	st := cli.NewInteractive(cli.NewRoot(), cli.SessionContext{})
	cur, ok := st.Current()
	if !ok {
		t.Fatal("no cursor")
	}
	if cur.Name != "base-url" || cur.Filled {
		t.Fatalf("expected unfilled --base-url, got %+v", cur)
	}
}

func TestFilledFlagsFloatToTopGlobalToNarrow(t *testing.T) {
	st := cli.NewInteractive(cli.NewRoot(), cli.SessionContext{
		Instance:       "na1",
		BaseURL:        "https://na1.polyapi.io",
		URLSource:      "env",
		APIKey:         "super-secret-key",
		KeySource:      "env",
		Language:       "typescript",
		LanguageSource: "detected",
		ProjectRoot:    "/tmp/billing",
		ProjectFound:   true,
		HasCredentials: true,
	})
	rows := st.Rows()
	var filled []cli.Row
	for _, r := range rows {
		if r.Filled {
			filled = append(filled, r)
		}
	}
	if len(filled) < 3 {
		t.Fatalf("expected filled globals, got %v", labels(filled))
	}
	if filled[0].Name != "base-url" || filled[0].Source != "env" {
		t.Fatalf("first filled %q source %q", filled[0].Name, filled[0].Source)
	}
	if filled[1].Name != "api-key" {
		t.Fatalf("second filled %q", filled[1].Name)
	}
	if filled[2].Name != "lang" || filled[2].Source != "detected" {
		t.Fatalf("third filled %q source %q", filled[2].Name, filled[2].Source)
	}
	for i := 1; i < len(filled); i++ {
		if filled[i].Rank < filled[i-1].Rank {
			t.Fatalf("filled ranks not global→narrow: %s then %s", filled[i-1].Label, filled[i].Label)
		}
	}
	cur, ok := st.Current()
	if !ok {
		t.Fatal("no cursor")
	}
	if cur.Filled {
		t.Fatalf("focus should start on an incomplete item, got filled %s", cur.Label)
	}
	if cur.Kind != cli.KindCommand {
		t.Fatalf("focus should be a command, got %s kind %d", cur.Label, cur.Kind)
	}
}

func TestRootShowsOnlyOneCommandLevel(t *testing.T) {
	st := cli.NewInteractive(cli.NewRoot(), cli.SessionContext{})
	rows := st.Rows()
	if !hasRow(rows, "generate") || !hasRow(rows, "function") {
		t.Fatalf("missing top-level commands: %v", labels(rows))
	}
	if hasRow(rows, "--contexts") {
		t.Fatalf("generate flags should wait until that command is opened:\n%s", labels(rows))
	}
	if hasRow(rows, "list") || hasRow(rows, "--server") || hasRow(rows, "<name>") {
		t.Fatalf("nested commands/flags should not appear at the root:\n%s", labels(rows))
	}
	focusNamed(t, st, "generate")
	if _, run := st.Activate(); run {
		t.Fatal("opening generate should not run")
	}
	rows = st.Rows()
	if !hasRow(rows, "--contexts") {
		t.Fatalf("generate flags missing after drill-in:\n%s", labels(rows))
	}
	if hasRow(rows, "function") {
		t.Fatalf("sibling commands should hide after drill-in:\n%s", labels(rows))
	}
}

func TestEnterOnFilledFlagOverwrites(t *testing.T) {
	st := cli.NewInteractive(cli.NewRoot(), cli.SessionContext{
		BaseURL:        "https://na1.polyapi.io",
		URLSource:      "env",
		HasCredentials: false,
	})
	rows := st.Rows()
	if len(rows) == 0 || !rows[0].Filled || rows[0].Name != "base-url" {
		t.Fatalf("expected filled --base-url at top, got %v", labels(rows))
	}
	st.Cursor = 0
	edit, run := st.Activate()
	if run {
		t.Fatal("should not run")
	}
	if edit.Name != "base-url" {
		t.Fatalf("enter on filled flag should edit, got %+v", edit)
	}
	st.Commit(edit, "https://eu1.polyapi.io")
	cur, _ := st.Current()
	if cur.Filled && cur.Name == "base-url" {
		t.Fatalf("after overwrite, focus should leave the filled flag, got %s", cur.Label)
	}
	argv := strings.Join(st.Argv(), " ")
	if !strings.Contains(argv, "--base-url https://eu1.polyapi.io") {
		t.Fatalf("argv=%s", argv)
	}
}

func TestArgvOmitsSessionDefaultGlobals(t *testing.T) {
	st := cli.NewInteractive(cli.NewRoot(), cli.SessionContext{
		BaseURL:  "https://dev.polyapi.io",
		APIKey:   "secret-key-xxxx3b68",
		Language: "typescript",
	})
	focusNamed(t, st, "doctor")
	st.Activate()
	if strings.Join(st.Selected(), " ") != "doctor" {
		t.Fatalf("selected %v", st.Selected())
	}
	argv := strings.Join(st.Argv(), " ")
	if strings.Contains(argv, "--base-url") || strings.Contains(argv, "--api-key") || strings.Contains(argv, "--lang") {
		t.Fatalf("defaults should be omitted: %s", argv)
	}
	if !strings.Contains(argv, "doctor") {
		t.Fatalf("argv=%s", argv)
	}
}

func TestExtraArgvDropsOriginalTokens(t *testing.T) {
	got := cli.ExtraArgv([]string{"./polyapi"}, []string{"deploy", "validate"})
	if strings.Join(got, " ") != "deploy validate" {
		t.Fatalf("%v", got)
	}
	got = cli.ExtraArgv(
		[]string{"./polyapi", "--base-url", "https://dev.polyapi.io"},
		[]string{"--base-url", "https://dev.polyapi.io", "doctor"},
	)
	if strings.Join(got, " ") != "doctor" {
		t.Fatalf("%v", got)
	}
	got = cli.ExtraArgv(
		[]string{"./polyapi", "--base-url", "https://dev.polyapi.io"},
		[]string{"doctor"},
	)
	if strings.Join(got, " ") != "doctor" {
		t.Fatalf("%v", got)
	}
}

func TestFormatInvokedCommandQuotesAndJoins(t *testing.T) {
	got := cli.FormatInvokedCommand("./polyapi", []string{"deploy", "validate"})
	if got != "./polyapi deploy validate" {
		t.Fatalf("%q", got)
	}
	got = cli.FormatInvokedCommand("polyapi", []string{"deploy", "push", "--env", "prod"})
	if got != "polyapi deploy push --env prod" {
		t.Fatalf("%q", got)
	}
	got = cli.FormatInvokedCommand("./polyapi", []string{"config", "set", "language", "type script"})
	if !strings.Contains(got, `./polyapi config set language 'type script'`) {
		t.Fatalf("%q", got)
	}
}

func TestFormatPromptLineAppendWritesOnlyExtraAtEnd(t *testing.T) {
	got := cli.FormatPromptLineAppend(48, "doctor")
	want := "\x1b[1A\x1b[49G doctor\x1b[K\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	got = cli.FormatPromptLineAppend(48, "deploy validate")
	if !strings.Contains(got, " deploy validate") || strings.Contains(got, "./polyapi") {
		t.Fatalf("should append extra only, got %q", got)
	}
}

func TestFormatEditPromptPutsUsageAboveInput(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CLICOLOR_FORCE", "")
	got := cli.FormatEditPrompt("--contexts", "Contexts to generate (comma-separated)", "billing", 80)
	plainGot := plain(got)
	if !strings.Contains(plainGot, "Contexts to generate (comma-separated)") {
		t.Fatalf("missing usage:\n%s", got)
	}
	if !strings.Contains(plainGot, "--contexts") {
		t.Fatalf("missing label:\n%s", got)
	}
	if !strings.Contains(plainGot, "billing") {
		t.Fatalf("missing typed value:\n%s", got)
	}
	idxUsage := strings.Index(plainGot, "Contexts to generate")
	idxLabel := strings.Index(plainGot, "--contexts")
	if idxUsage < 0 || idxLabel < 0 || idxUsage > idxLabel {
		t.Fatalf("usage should sit above the input label:\n%s", got)
	}
	if strings.Contains(cli.FormatEditPrompt("--name", "", "echo", 80), "\n") {
		t.Fatal("empty usage should not add a description line")
	}
}

func TestFormatEditPromptUsageUsesListGray(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "1")
	t.Setenv("COLORTERM", "truecolor")
	t.Setenv("TERM", "xterm-256color")
	got := cli.FormatEditPrompt("--contexts", "Contexts to generate (comma-separated)", "billing", 80)
	if !strings.Contains(got, rgbSeq(cli.Stone500)) {
		t.Fatalf("usage should use Stone500 (list-view gray):\n%q", got)
	}
}

func TestFlagEditCarriesCobraUsage(t *testing.T) {
	st := cli.NewInteractive(cli.NewRoot(), cli.SessionContext{
		Language:       "typescript",
		LanguageSource: "detected",
	})
	focusNamed(t, st, "generate")
	st.Activate()
	focusNamed(t, st, "--contexts")
	edit, _ := st.Activate()
	if !strings.Contains(edit.Usage, "Contexts to generate") {
		t.Fatalf("flag usage %q", edit.Usage)
	}
}

func TestArgEditCarriesCommandDescription(t *testing.T) {
	st := cli.NewInteractive(cli.NewRoot(), cli.SessionContext{
		BaseURL:        "https://na1.polyapi.io",
		APIKey:         "k123456789",
		HasCredentials: true,
	})
	focusNamed(t, st, "subscription")
	st.Activate()
	focusNamed(t, st, "get")
	st.Activate()
	focusNamed(t, st, "<id-or-name>")
	edit, _ := st.Activate()
	if edit.Kind != cli.KindArg {
		t.Fatalf("expected arg edit, got %+v", edit)
	}
	if !strings.Contains(strings.ToLower(edit.Usage), "subscription") {
		t.Fatalf("arg usage should describe the command, got %q", edit.Usage)
	}
}

func TestSelectingCommandThenFlagBuildsArgv(t *testing.T) {
	st := cli.NewInteractive(cli.NewRoot(), cli.SessionContext{
		Language:       "typescript",
		LanguageSource: "detected",
	})
	focusNamed(t, st, "generate")
	if _, run := st.Activate(); run {
		t.Fatal("selecting generate should not run")
	}
	if got := strings.Join(st.Selected(), " "); got != "generate" {
		t.Fatalf("selected %q", got)
	}
	focusNamed(t, st, "--contexts")
	edit, _ := st.Activate()
	if edit.Name != "contexts" {
		t.Fatalf("edit %+v", edit)
	}
	st.Commit(edit, "billing")
	argv := strings.Join(st.Argv(), " ")
	if strings.Contains(argv, "--lang") {
		t.Fatalf("session language is the default and should be omitted: %s", argv)
	}
	if !strings.Contains(argv, "generate") || !strings.Contains(argv, "--contexts billing") {
		t.Fatalf("argv=%s", argv)
	}
	var filled []string
	for _, r := range st.Rows() {
		if r.Filled {
			filled = append(filled, r.Label)
		}
	}
	if strings.Join(filled, ",") != "--lang,generate,--contexts" &&
		!(len(filled) >= 2 && filled[0] == "--lang" && contains(filled, "generate") && contains(filled, "--contexts")) {
		t.Fatalf("filled order global→narrow: %v", filled)
	}
}

func TestRequiredOptionsMarkedAndBlockRunFocus(t *testing.T) {
	st := cli.NewInteractive(cli.NewRoot(), cli.SessionContext{
		BaseURL:        "https://na1.polyapi.io",
		APIKey:         "k123456789",
		HasCredentials: true,
	})
	focusNamed(t, st, "function")
	st.Activate()
	focusNamed(t, st, "add")
	st.Activate()
	if st.ReadyToRun() {
		t.Fatal("add should not be ready without name/file/server")
	}
	cur, _ := st.Current()
	if cur.Kind == cli.KindRun {
		t.Fatal("run should not be auto-focused while required options are empty")
	}
	if cur.Name != "name" || !cur.Required {
		t.Fatalf("expected required <name>, got %+v", cur)
	}
	edit, _ := st.Activate()
	st.Commit(edit, "echo")
	cur, _ = st.Current()
	if cur.Name != "file" || !cur.Required {
		t.Fatalf("expected required <file>, got %s required=%v", cur.Label, cur.Required)
	}
	edit, _ = st.Activate()
	st.Commit(edit, "echo.ts")
	if st.ReadyToRun() {
		t.Fatal("still need --type")
	}
	cur, _ = st.Current()
	if cur.Kind == cli.KindRun {
		t.Fatal("run should wait for --type")
	}
	if cur.Name != "type" || !cur.Required {
		t.Fatalf("expected required --type, got %+v", cur)
	}
	var contextFlag cli.Row
	for _, r := range st.Rows() {
		if r.Name == "context" {
			contextFlag = r
		}
	}
	if contextFlag.Required {
		t.Fatal("--context is optional")
	}
	edit, _ = st.Activate()
	st.Commit(edit, "server")
	if !st.ReadyToRun() {
		t.Fatal("expected ready after required options")
	}
	st.FocusFirstIncomplete()
	cur, _ = st.Current()
	if cur.Kind != cli.KindRun {
		t.Fatalf("run should be focused once required options are filled, got %s", cur.Label)
	}
}

func TestJobCreateRequiresNameInTree(t *testing.T) {
	st := cli.NewInteractive(cli.NewRoot(), cli.SessionContext{
		BaseURL:        "https://na1.polyapi.io",
		APIKey:         "k123456789",
		HasCredentials: true,
	})
	focusNamed(t, st, "job")
	st.Activate()
	focusNamed(t, st, "create")
	st.Activate()
	if st.ReadyToRun() {
		t.Fatal("job create should not be ready without --name and --function")
	}
	cur, _ := st.Current()
	if cur.Kind == cli.KindRun {
		t.Fatal("run should not be focused while --name is empty")
	}
	var name, functionFlag cli.Row
	for _, r := range st.Rows() {
		switch r.Name {
		case "name":
			name = r
		case "function":
			functionFlag = r
		}
	}
	if !name.Required {
		t.Fatal("--name should be required on job create")
	}
	if !functionFlag.Required {
		t.Fatal("--function should be required on job create")
	}
	if !cur.Required || (cur.Name != "name" && cur.Name != "function" && cur.Name != "functions") {
		t.Fatalf("focus should start on a required create option, got %s", cur.Label)
	}
}

func TestSchemaCreateRequiresNameAndDefinitionInTree(t *testing.T) {
	st := cli.NewInteractive(cli.NewRoot(), cli.SessionContext{
		BaseURL:        "https://na1.polyapi.io",
		APIKey:         "k123456789",
		HasCredentials: true,
	})
	focusNamed(t, st, "schema")
	st.Activate()
	focusNamed(t, st, "create")
	st.Activate()
	if st.ReadyToRun() {
		t.Fatal("schema create should not be ready without --name and --definition")
	}
	var name, contextFlag, definition cli.Row
	for _, r := range st.Rows() {
		switch r.Name {
		case "name":
			name = r
		case "context":
			contextFlag = r
		case "definition":
			definition = r
		}
	}
	if !name.Required {
		t.Fatal("--name should be required on schema create")
	}
	if !contextFlag.Required {
		t.Fatal("--context should be required on schema create")
	}
	if !definition.Required {
		t.Fatal("--definition should be required on schema create")
	}
}

func TestSubscriptionCreateRequiresFlagsInTree(t *testing.T) {
	st := cli.NewInteractive(cli.NewRoot(), cli.SessionContext{
		BaseURL:        "https://na1.polyapi.io",
		APIKey:         "k123456789",
		HasCredentials: true,
	})
	focusNamed(t, st, "subscription")
	st.Activate()
	focusNamed(t, st, "create")
	st.Activate()
	if st.ReadyToRun() {
		t.Fatal("subscription create should not be ready without required flags")
	}
	required := map[string]bool{}
	for _, r := range st.Rows() {
		if r.Required {
			required[r.Name] = true
		}
	}
	for _, name := range []string{"name", "context", "type", "websocket-url", "function"} {
		if !required[name] {
			t.Fatalf("--%s should be required on subscription create", name)
		}
	}
	if !required["query"] && !required["query-file"] {
		t.Fatal("--query or --query-file should be required on subscription create")
	}
}

func TestSnippetAddRequiresNamePathAndContextInTree(t *testing.T) {
	st := cli.NewInteractive(cli.NewRoot(), cli.SessionContext{
		BaseURL:        "https://na1.polyapi.io",
		APIKey:         "k123456789",
		HasCredentials: true,
	})
	focusNamed(t, st, "snippet")
	st.Activate()
	focusNamed(t, st, "add")
	st.Activate()
	if st.ReadyToRun() {
		t.Fatal("snippet add should not be ready without name, path, and --context")
	}
	var name, path, contextFlag cli.Row
	for _, r := range st.Rows() {
		switch r.Name {
		case "name":
			name = r
		case "path":
			path = r
		case "context":
			contextFlag = r
		}
	}
	if !name.Required {
		t.Fatal("<name> should be required on snippet add")
	}
	if !path.Required {
		t.Fatal("<path> should be required on snippet add")
	}
	if !contextFlag.Required {
		t.Fatal("--context should be required on snippet add")
	}
}

func TestGenerateRunFocusedWhenNoRequiredLocals(t *testing.T) {
	st := cli.NewInteractive(cli.NewRoot(), cli.SessionContext{
		BaseURL:        "https://na1.polyapi.io",
		APIKey:         "k123456789",
		Language:       "typescript",
		LanguageSource: "detected",
		HasCredentials: true,
	})
	focusNamed(t, st, "generate")
	st.Activate()
	if !st.ReadyToRun() {
		t.Fatal("generate has no required locals")
	}
	cur, _ := st.Current()
	if cur.Kind != cli.KindRun {
		t.Fatalf("generate should focus run, got %s", cur.Label)
	}
	var contexts cli.Row
	for _, r := range st.Rows() {
		if r.Name == "contexts" {
			contexts = r
		}
	}
	if contexts.Required {
		t.Fatal("--contexts is optional")
	}
}

func TestParentCommandShowsChildren(t *testing.T) {
	st := cli.NewInteractive(cli.NewRoot(), cli.SessionContext{
		BaseURL:        "https://na1.polyapi.io",
		APIKey:         "k123456789",
		HasCredentials: true,
	})
	focusNamed(t, st, "function")
	if _, run := st.Activate(); run {
		t.Fatal("function should drill, not run")
	}
	if got := strings.Join(st.Selected(), " "); got != "function" {
		t.Fatalf("selected %q", got)
	}
	rows := st.Rows()
	if !hasRow(rows, "list") || !hasRow(rows, "add") {
		t.Fatalf("function children missing: %v", labels(rows))
	}
	cur, _ := st.Current()
	if cur.Kind != cli.KindCommand {
		t.Fatalf("focus should be a subcommand, got %s", cur.Label)
	}
	if hasRow(rows, "--server") || hasRow(rows, "<name>") {
		t.Fatalf("add flags should wait until add is opened:\n%s", labels(rows))
	}
}

func TestTypeToFilterAndEnterSelects(t *testing.T) {
	st := cli.NewInteractive(cli.NewRoot(), cli.SessionContext{
		BaseURL:        "https://na1.polyapi.io",
		APIKey:         "k123456789",
		HasCredentials: true,
	})
	st.TypeFilter("gen")
	if st.Filter != "gen" {
		t.Fatalf("filter %q", st.Filter)
	}
	cur, _ := st.Current()
	if cur.Label != "generate" {
		t.Fatalf("typing gen should highlight generate, got %s in %v", cur.Label, labels(st.Rows()))
	}
	if hasRow(st.Rows(), "function") {
		t.Fatalf("function should be filtered out: %v", labels(st.Rows()))
	}
	if _, run := st.Activate(); run {
		t.Fatal("enter should open generate")
	}
	if got := strings.Join(st.Selected(), " "); got != "generate" {
		t.Fatalf("selected %q", got)
	}
	if st.Filter != "" {
		t.Fatalf("filter should clear after selecting, got %q", st.Filter)
	}
	if !st.Back() {
		t.Fatal("back")
	}
	if st.Filter != "" {
		t.Fatalf("selecting should have discarded the filter, got %q", st.Filter)
	}
	cur, _ = st.Current()
	if cur.Label != "generate" {
		t.Fatalf("focus should stay on generate, got %s", cur.Label)
	}
	if !hasRow(st.Rows(), "function") {
		t.Fatalf("siblings should be visible after the filter clears: %v", labels(st.Rows()))
	}
}

func TestSelectingFilteredFlagClearsFilter(t *testing.T) {
	st := cli.NewInteractive(cli.NewRoot(), cli.SessionContext{
		Language:       "typescript",
		LanguageSource: "detected",
	})
	focusNamed(t, st, "generate")
	if _, run := st.Activate(); run {
		t.Fatal("open generate")
	}
	st.TypeFilter("con")
	cur, _ := st.Current()
	if cur.Name != "contexts" {
		t.Fatalf("expected --contexts, got %s in %v", cur.Label, labels(st.Rows()))
	}
	edit, _ := st.Activate()
	if edit.Name != "contexts" {
		t.Fatalf("edit %+v", edit)
	}
	if st.Filter != "" {
		t.Fatalf("filter should clear on select, got %q", st.Filter)
	}
	cur, _ = st.Current()
	if cur.Name != "contexts" {
		t.Fatalf("focus should stay on --contexts, got %s", cur.Label)
	}
	if !hasRow(st.Rows(), "--names") {
		t.Fatalf("sibling flags should be visible: %v", labels(st.Rows()))
	}
}

func TestEscapeRestoresPreviousFocus(t *testing.T) {
	st := cli.NewInteractive(cli.NewRoot(), cli.SessionContext{
		BaseURL:        "https://na1.polyapi.io",
		APIKey:         "k123456789",
		HasCredentials: true,
	})
	focusNamed(t, st, "function")
	if _, run := st.Activate(); run {
		t.Fatal("function should drill")
	}
	focusNamed(t, st, "add")
	if _, run := st.Activate(); run {
		t.Fatal("add should drill")
	}
	if !hasRow(st.Rows(), "--type") {
		t.Fatalf("add flags missing: %v", labels(st.Rows()))
	}
	if !st.Back() {
		t.Fatal("back from add")
	}
	cur, _ := st.Current()
	if cur.Label != "add" {
		t.Fatalf("escape should restore focus on add, got %s", cur.Label)
	}
	if hasRow(st.Rows(), "--type") {
		t.Fatalf("still showing add flags after back: %v", labels(st.Rows()))
	}
	if !st.Back() {
		t.Fatal("back from function")
	}
	cur, _ = st.Current()
	if cur.Label != "function" {
		t.Fatalf("escape should restore focus on function, got %s", cur.Label)
	}
	if st.Back() {
		t.Fatal("back at root should be a no-op")
	}
	cur, _ = st.Current()
	if cur.Label != "function" {
		t.Fatalf("root focus should stay on function, got %s", cur.Label)
	}
}

func TestFocusSkipsFilledOptionalGlobals(t *testing.T) {
	st := cli.NewInteractive(cli.NewRoot(), cli.SessionContext{
		BaseURL:        "https://na1.polyapi.io",
		URLSource:      "project",
		APIKey:         "k123456789",
		KeySource:      "project",
		HasCredentials: true,
	})
	cur, ok := st.Current()
	if !ok {
		t.Fatal("no cursor")
	}
	if cur.Name == "env" {
		t.Fatal("optional --env should not steal initial focus")
	}
	if cur.Kind != cli.KindCommand {
		t.Fatalf("want command, got %s", cur.Label)
	}
}

func TestWhoamiIncludesProjectAndLanguage(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	t.Setenv("NO_COLOR", "1")
	os.Unsetenv("POLY_API_KEY")
	os.Unsetenv("POLY_API_BASE_URL")
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname='x'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := run("--non-interactive", "auth", "login", "na1", "super-secret-key")
	if code != 0 {
		t.Fatalf("auth login %d %s %s", code, stdout, stderr)
	}
	stdout, stderr, code = run("--non-interactive", "auth", "whoami")
	if code != 0 {
		t.Fatalf("whoami %d %s", code, stderr)
	}
	if !strings.Contains(stdout, "python") {
		t.Fatalf("whoami missing language:\n%s", stdout)
	}
	if !strings.Contains(stdout, "project") {
		t.Fatalf("whoami missing project:\n%s", stdout)
	}
}

func TestBannerSkippedWithoutTTY(t *testing.T) {
	_, stderr, code := run("version")
	if code != exitcode.OK && code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.Contains(stderr, "lang=?") || strings.Contains(stderr, "(no credentials)") {
		t.Fatalf("banner should not print to a pipe:\n%s", stderr)
	}
}

func labels(rows []cli.Row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Label
	}
	return out
}

func hasRow(rows []cli.Row, label string) bool {
	for _, r := range rows {
		if r.Label == label {
			return true
		}
	}
	return false
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func focusNamed(t *testing.T, st *cli.Interactive, label string) {
	t.Helper()
	rows := st.Rows()
	for i, r := range rows {
		if r.Label == label {
			st.Cursor = i
			return
		}
	}
	t.Fatalf("no row %q in %v", label, labels(rows))
}
