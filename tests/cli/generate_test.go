package cli_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func fixturesDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "fixtures", "delegate")
}

func onPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func writeTSProject(t *testing.T, root string) {
	t.Helper()
	fixtures := fixturesDir(t)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(filepath.Join(fixtures, "ts-project", "package.json"), filepath.Join(root, "package.json")); err != nil {
		t.Fatal(err)
	}
	js := filepath.Join(root, "node_modules", "polyapi", "build", "adapter.js")
	if err := os.MkdirAll(filepath.Dir(js), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(filepath.Join(fixtures, "adapter.js"), js); err != nil {
		t.Fatal(err)
	}
	vari := filepath.Join(root, "src", "billing", "vari")
	if err := os.MkdirAll(vari, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vari, "apiKey.ts"), []byte("export const polyConfig = { name: 'apiKey' };\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writePythonProject(t *testing.T, root string) {
	t.Helper()
	fixtures := fixturesDir(t)
	if err := os.MkdirAll(filepath.Join(root, "polyapi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(filepath.Join(fixtures, "python-project", "pyproject.toml"), filepath.Join(root, "pyproject.toml")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "polyapi", "__init__.py"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(filepath.Join(fixtures, "adapter.py"), filepath.Join(root, "polyapi", "__main__.py")); err != nil {
		t.Fatal(err)
	}
	vari := filepath.Join(root, "src", "billing", "vari")
	if err := os.MkdirAll(vari, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vari, "apiKey.py"), []byte("polyConfig = {'name': 'apiKey'}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

func generateIn(t *testing.T, dir string, extra ...string) (stdout, stderr string, code int) {
	t.Helper()
	t.Chdir(dir)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/auth" || strings.HasSuffix(r.URL.Path, "/auth"):
			io.WriteString(w, `{"permissions":{"libraryGenerate":true,"customDev":true}}`)
		case strings.HasSuffix(r.URL.Path, "/specs"):
			io.WriteString(w, `[{"id":"fn1","name":"echo","context":"billing","type":"serverFunction"}]`)
		default:
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"message":"not found"}`)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("POLY_API_KEY", "test-key-xxxx")
	t.Setenv("POLY_API_BASE_URL", srv.URL)
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CLICOLOR_FORCE", "")
	args := append([]string{"--non-interactive"}, extra...)
	if len(extra) == 0 || extra[len(extra)-1] != "generate" {
		args = append(args, "generate")
	}
	return run(args...)
}

func TestGenerateTSSampleProject(t *testing.T) {
	if !onPath("node") {
		t.Skip("node not on PATH")
	}
	dir := t.TempDir()
	writeTSProject(t, dir)
	stdout, stderr, code := generateIn(t, dir)
	if code != 0 {
		t.Fatalf("exit %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "generated") && !strings.Contains(stdout, "ok:") {
		t.Fatalf("%s", stdout)
	}
	if _, err := os.Stat(filepath.Join(dir, "generated", "sdk.txt")); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateFromNestedDirectory(t *testing.T) {
	if !onPath("node") {
		t.Skip("node not on PATH")
	}
	dir := t.TempDir()
	writeTSProject(t, dir)
	nested := filepath.Join(dir, "src", "billing")
	stdout, stderr, code := generateIn(t, nested)
	if code != 0 {
		t.Fatalf("exit %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "generated", "sdk.txt")); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateSDKNotInstalledIs10(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := generateIn(t, dir)
	if code != 10 {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
	if !strings.Contains(strings.ToLower(stderr), "not installed") {
		t.Fatalf("%s", stderr)
	}
}

func TestGeneratePythonSampleProject(t *testing.T) {
	if !onPath("python3") && !onPath("python") {
		t.Skip("python not on PATH")
	}
	dir := t.TempDir()
	writePythonProject(t, dir)
	stdout, stderr, code := generateIn(t, dir)
	if code != 0 {
		t.Fatalf("exit %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "generated", "sdk.txt")); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateMissingAdapterIs10(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"polyapi":"1.0.0"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := generateIn(t, dir, "--adapter", "polyapi-adapter-does-not-exist-xyz", "generate")
	if code != 10 {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
	low := strings.ToLower(stderr)
	if !strings.Contains(low, "adapter") && !strings.Contains(low, "not found") {
		t.Fatalf("%s", stderr)
	}
}

func TestGenerateWithoutCredentialsIs3(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("POLY_API_KEY", "")
	t.Setenv("POLY_API_BASE_URL", "")
	t.Setenv("NO_COLOR", "1")
	os.Unsetenv("POLY_API_KEY")
	os.Unsetenv("POLY_API_BASE_URL")
	_, stderr, code := run("--non-interactive", "--lang", "python", "generate")
	if code != 3 {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
}

func TestGenerateProtocolMismatchIs11(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		py, err = exec.LookPath("python")
	}
	if err != nil {
		t.Skip("python not on PATH")
	}
	dir := t.TempDir()
	adapter := filepath.Join(fixturesDir(t), "adapter.py")
	t.Setenv("POLY_FIXTURE_PROTOCOL", "0")
	_, stderr, code := generateIn(t, dir, "--adapter", py+" "+adapter, "generate")
	if code != 11 {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
}

func TestGenerateUndetectedLanguageIs2(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := generateIn(t, dir)
	if code != 2 {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
}

func TestAuthLoginAndWhoami(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	t.Setenv("NO_COLOR", "1")
	t.Setenv("POLY_API_KEY", "")
	t.Setenv("POLY_API_BASE_URL", "")
	os.Unsetenv("POLY_API_KEY")
	os.Unsetenv("POLY_API_BASE_URL")
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := run("--non-interactive", "auth", "login", "na1", "super-secret-key")
	if code != 0 {
		t.Fatalf("auth login exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "logged in") {
		t.Fatalf("%s", stdout)
	}
	disk, err := os.ReadFile(filepath.Join(dir, ".poly", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(disk), "super-secret-key") {
		t.Fatalf("plaintext key:\n%s", disk)
	}
	stdout, stderr, code = run("--non-interactive", "auth", "whoami")
	if code != 0 {
		t.Fatalf("whoami exit %d stderr=%s", code, stderr)
	}
	if strings.Contains(stdout, "super-secret-key") {
		t.Fatalf("leaked key:\n%s", stdout)
	}
	if !strings.Contains(stdout, "********t-key") && !strings.Contains(stdout, "********") {
		t.Fatalf("expected redaction:\n%s", stdout)
	}
	stdout, stderr, code = run("--non-interactive", "config", "set", "language", "python")
	if code != 0 {
		t.Fatalf("config set %d %s", code, stderr)
	}
	stdout, stderr, code = run("--non-interactive", "config", "get", "language")
	if code != 0 || !strings.Contains(stdout, "python") {
		t.Fatalf("config get %d %s %s", code, stdout, stderr)
	}
	stdout, _, code = run("--non-interactive", "config", "show")
	if code != 0 {
		t.Fatalf("config show exit %d", code)
	}
	if strings.Contains(stdout, "super-secret-key") {
		t.Fatalf("leaked:\n%s", stdout)
	}
	_, stderr, code = run("--non-interactive", "auth", "logout")
	if code != 0 {
		t.Fatalf("logout %d %s", code, stderr)
	}
	_, stderr, code = run("--non-interactive", "auth", "whoami")
	if code != 3 {
		t.Fatalf("whoami after logout %d %s", code, stderr)
	}
}
