package cli_test

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/polyapi/polyglot/src/exitcode"
	"github.com/polyapi/polyglot/src/projinit"
)

func isolateInit(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	t.Setenv("HOME", dir)
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CLICOLOR_FORCE", "")
	t.Setenv("POLY_API_KEY", "")
	t.Setenv("POLY_API_BASE_URL", "")
	os.Unsetenv("POLY_API_KEY")
	os.Unsetenv("POLY_API_BASE_URL")
	return dir
}

func runInit(args ...string) (stdout, stderr string, code int) {
	all := append([]string{"--non-interactive", "init"}, args...)
	return run(all...)
}

func TestInitEmptyTypeScriptProject(t *testing.T) {
	dir := isolateInit(t)
	stdout, stderr, code := runInit("--lang", "typescript", "--template", "none", "--no-deps", "--no-runtime", "--name", "billing")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout + stderr)
	if !strings.Contains(got, "initialized") && !strings.Contains(got, "wrote") {
		t.Fatalf("output:\n%s", got)
	}
	pkg := filepath.Join(dir, "package.json")
	raw, err := os.ReadFile(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"name": "billing"`)) {
		t.Fatalf("package.json=%s", raw)
	}
	if !bytes.Contains(raw, []byte("polyapi")) {
		t.Fatalf("expected polyapi dep:\n%s", raw)
	}
	cfg, err := os.ReadFile(filepath.Join(dir, ".poly", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(cfg)
	if !strings.Contains(text, "typescript") {
		t.Fatalf("config=%s", text)
	}
	if !strings.Contains(text, "unallowed_branch") || !strings.Contains(text, "prod") {
		t.Fatalf("missing deploy policy:\n%s", text)
	}
	if _, err := os.Stat(filepath.Join(dir, "tsconfig.json")); err != nil {
		t.Fatal(err)
	}
}

func TestInitPythonWritesRequirements(t *testing.T) {
	dir := isolateInit(t)
	_, stderr, code := runInit("--lang", "python", "--template", "none", "--no-deps", "--no-runtime", "--name", "billing_py")
	if code != 0 {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "requirements.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("polyapi-python")) {
		t.Fatalf("%s", raw)
	}
	cfg, _ := os.ReadFile(filepath.Join(dir, ".poly", "config.toml"))
	if !bytes.Contains(cfg, []byte("python")) {
		t.Fatalf("%s", cfg)
	}
}

func TestInitExistingDoesNotClobberPackageJSON(t *testing.T) {
	dir := isolateInit(t)
	orig := []byte(`{"name":"keep-me","version":"1.2.3"}`)
	if err := os.WriteFile(filepath.Join(dir, "package.json"), orig, 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := runInit("--existing", "--lang", "typescript", "--template", "none", "--no-deps", "--no-runtime")
	if code != 0 {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "package.json"))
	if !bytes.Contains(got, []byte("keep-me")) {
		t.Fatalf("package.json clobbered: %s", got)
	}
	if !bytes.Contains(got, []byte("polyapi")) {
		t.Fatalf("expected polyapi to be added: %s", got)
	}
}

func TestInitUnpacksZipTemplate(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, _ := w.Create("demo-main/hello.txt")
	_, _ = f.Write([]byte("from-template\n"))
	f, _ = w.Create("demo-main/src/app.ts")
	_, _ = f.Write([]byte("export {}\n"))
	_ = w.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "application/zip")
		_, _ = rw.Write(buf.Bytes())
	}))
	t.Cleanup(srv.Close)

	dir := isolateInit(t)
	stdout, stderr, code := runInit(
		"--lang", "typescript",
		"--template", srv.URL+"/demo.zip",
		"--no-deps", "--no-runtime",
	)
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got, err := os.ReadFile(filepath.Join(dir, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "from-template\n" {
		t.Fatalf("%q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "src", "app.ts")); err != nil {
		t.Fatal(err)
	}
}

func TestInitTemplateConflictRequiresForce(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, _ := w.Create("README.md")
	_, _ = f.Write([]byte("template\n"))
	_ = w.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		_, _ = rw.Write(buf.Bytes())
	}))
	t.Cleanup(srv.Close)

	dir := isolateInit(t)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := runInit("--lang", "typescript", "--template", srv.URL+"/t.zip", "--no-deps", "--no-runtime")
	if code != exitcode.Usage {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
	if !strings.Contains(plain(stderr), "overwrite") && !strings.Contains(plain(stderr), "--force") {
		t.Fatalf("stderr=%s", stderr)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "README.md"))
	if string(got) != "mine\n" {
		t.Fatalf("should not overwrite: %q", got)
	}

	_, stderr, code = runInit("--lang", "typescript", "--template", srv.URL+"/t.zip", "--no-deps", "--no-runtime", "--force")
	if code != 0 {
		t.Fatalf("force exit %d stderr=%s", code, stderr)
	}
	got, _ = os.ReadFile(filepath.Join(dir, "README.md"))
	if string(got) != "template\n" {
		t.Fatalf("force wrote %q", got)
	}
}

func TestInitDoesNotWriteEnvAPIKey(t *testing.T) {
	dir := isolateInit(t)
	t.Setenv("POLY_API_KEY", "test-key-xxxx")
	t.Setenv("POLY_API_BASE_URL", "https://na1.polyapi.io")
	_, stderr, code := runInit("--lang", "typescript", "--template", "none", "--no-deps", "--no-runtime")
	if code != 0 {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
	cfg, err := os.ReadFile(filepath.Join(dir, ".poly", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(cfg, []byte("api_key")) {
		t.Fatalf("env API key must not be written to project config:\n%s", cfg)
	}
	if !bytes.Contains(cfg, []byte("na1.polyapi.io")) {
		t.Fatalf("expected base_url from env:\n%s", cfg)
	}
}

func TestInitUsesDetectedProjectRoot(t *testing.T) {
	dir := isolateInit(t)
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"app"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "src", "lib")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)
	_, stderr, code := runInit("--lang", "typescript", "--template", "none", "--no-deps", "--no-runtime")
	if code != 0 {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, ".poly", "config.toml")); err != nil {
		t.Fatal(err)
	}
}

func TestInitJavaRejected(t *testing.T) {
	isolateInit(t)
	_, stderr, code := runInit("--lang", "java", "--template", "none", "--no-deps", "--no-runtime")
	if code != exitcode.Usage {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
	if !strings.Contains(plain(stderr), "Java") {
		t.Fatalf("stderr=%s", stderr)
	}
}

func TestInitNonInteractiveNeedsTemplate(t *testing.T) {
	isolateInit(t)
	_, stderr, code := runInit("--lang", "typescript", "--no-deps", "--no-runtime")
	if code != exitcode.Usage {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
	if !strings.Contains(plain(stderr), "--template") {
		t.Fatalf("stderr=%s", stderr)
	}
}

func TestInitEnvMap(t *testing.T) {
	dir := isolateInit(t)
	_, stderr, code := runInit(
		"--lang", "typescript", "--template", "none", "--no-deps", "--no-runtime",
		"--env-map", "main=prod", "--env-map", "develop=dev",
	)
	if code != 0 {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
	cfg, _ := os.ReadFile(filepath.Join(dir, ".poly", "config.toml"))
	text := string(cfg)
	if !strings.Contains(text, "develop") || !strings.Contains(text, "dev") {
		t.Fatalf("%s", text)
	}
}

func TestInitWritesGitHubWorkflow(t *testing.T) {
	dir := isolateInit(t)
	_, stderr, code := runInit("--lang", "typescript", "--template", "none", "--no-deps", "--no-runtime", "--provider", "github")
	if code != 0 {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".github", "workflows", "poly-deploy.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("POLY_API_KEY_PROD")) {
		t.Fatalf("%s", raw)
	}
}

func TestInitMissingRuntimeMessage(t *testing.T) {
	msg := projinit.MissingRuntimeMessage("typescript")
	if !strings.Contains(msg, "nodejs.org") {
		t.Fatal(msg)
	}
}
