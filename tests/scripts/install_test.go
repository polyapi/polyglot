package scripts_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/polyapi/polyglot/src/version"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// installEnv builds a child environment that cannot touch the real HOME rc
// files. Explicit keys replace any parent value (duplicate HOME in the
// slice would otherwise be undefined).
func installEnv(overrides map[string]string) []string {
	skip := make(map[string]struct{}, len(overrides)+8)
	for k := range overrides {
		skip[k] = struct{}{}
	}
	for _, k := range []string{"HOME", "SHELL", "PATH", "POLYAPI_SKIP_PATH", "POLYAPI_INSTALL_DIR", "POLYAPI_VERSION", "POLYAPI_RELEASES_URL", "POLYAPI_DOWNLOAD_BASE", "POLYAPI_REPO"} {
		skip[k] = struct{}{}
	}
	var env []string
	for _, e := range os.Environ() {
		k, _, ok := strings.Cut(e, "=")
		if !ok {
			continue
		}
		if _, drop := skip[k]; drop {
			continue
		}
		if strings.HasPrefix(k, "POLYAPI_") {
			continue
		}
		env = append(env, e)
	}
	for k, v := range overrides {
		env = append(env, k+"="+v)
	}
	return env
}

func serveInstall(t *testing.T, tag string, payload []byte, checksums string) *httptest.Server {
	t.Helper()
	ver := version.NormalizeVersion(tag)
	asset := version.AssetName(ver, runtime.GOOS, runtime.GOARCH)
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": tag})
	})
	mux.HandleFunc("/download/"+tag+"/"+asset, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	})
	mux.HandleFunc("/download/"+tag+"/"+version.ChecksumsName, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(checksums))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func checksumsFor(t *testing.T, tag string, payload []byte) string {
	t.Helper()
	asset := version.AssetName(version.NormalizeVersion(tag), runtime.GOOS, runtime.GOARCH)
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]) + "  " + asset + "\n"
}

// absDir matches install.sh's `cd && pwd` so PATH assertions survive macOS
// /var → /private/var symlink resolution.
func absDir(t *testing.T, dir string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return dir
	}
	return resolved
}

func TestInstallScriptReplacesExisting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix install.sh")
	}
	root := repoRoot(t)
	script := filepath.Join(root, "scripts", "install.sh")
	home := t.TempDir()
	dir := filepath.Join(home, "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "polyapi")
	if err := os.WriteFile(dest, []byte("old-install"), 0o755); err != nil {
		t.Fatal(err)
	}

	payload := []byte("installed-from-script")
	tag := "v9.9.9"
	srv := serveInstall(t, tag, payload, checksumsFor(t, tag, payload))

	cmd := exec.Command("sh", script)
	cmd.Env = installEnv(map[string]string{
		"HOME":                  home,
		"SHELL":                 "/bin/zsh",
		"PATH":                  "/usr/bin:/bin",
		"POLYAPI_SKIP_PATH":     "1",
		"POLYAPI_INSTALL_DIR":   dir,
		"POLYAPI_RELEASES_URL":  srv.URL + "/releases/latest",
		"POLYAPI_DOWNLOAD_BASE": srv.URL + "/download",
	})
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install.sh: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "installed") {
		t.Fatalf("output:\n%s", out)
	}
	body, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != string(payload) {
		t.Fatalf("dest=%q", body)
	}
}

func TestInstallScriptChecksumMismatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix install.sh")
	}
	root := repoRoot(t)
	script := filepath.Join(root, "scripts", "install.sh")
	home := t.TempDir()
	dir := filepath.Join(home, "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "polyapi")
	if err := os.WriteFile(dest, []byte("old-install"), 0o755); err != nil {
		t.Fatal(err)
	}

	payload := []byte("installed-from-script")
	tag := "v9.9.9"
	asset := version.AssetName(version.NormalizeVersion(tag), runtime.GOOS, runtime.GOARCH)
	srv := serveInstall(t, tag, payload, strings.Repeat("0", 64)+"  "+asset+"\n")

	cmd := exec.Command("sh", script, tag)
	cmd.Env = installEnv(map[string]string{
		"HOME":                  home,
		"SHELL":                 "/bin/zsh",
		"PATH":                  "/usr/bin:/bin",
		"POLYAPI_SKIP_PATH":     "1",
		"POLYAPI_INSTALL_DIR":   dir,
		"POLYAPI_DOWNLOAD_BASE": srv.URL + "/download",
	})
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected checksum failure\n%s", out)
	}
	if !strings.Contains(string(out), "checksum") {
		t.Fatalf("output:\n%s", out)
	}
	body, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "old-install" {
		t.Fatalf("replaced dest despite checksum mismatch: %q", body)
	}
}

func TestInstallScriptAddsPathWhenMissing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix install.sh")
	}
	root := repoRoot(t)
	script := filepath.Join(root, "scripts", "install.sh")
	home := t.TempDir()
	dir := filepath.Join(home, "opt")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	dir = absDir(t, dir)

	payload := []byte("installed-from-script")
	tag := "v9.9.9"
	srv := serveInstall(t, tag, payload, checksumsFor(t, tag, payload))

	cmd := exec.Command("sh", script)
	cmd.Env = installEnv(map[string]string{
		"HOME":                  home,
		"SHELL":                 "/bin/zsh",
		"PATH":                  "/usr/bin:/bin",
		"POLYAPI_INSTALL_DIR":   dir,
		"POLYAPI_RELEASES_URL":  srv.URL + "/releases/latest",
		"POLYAPI_DOWNLOAD_BASE": srv.URL + "/download",
	})
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install.sh: %v\n%s", err, out)
	}
	got := string(out)
	if !strings.Contains(got, "PATH: adding") {
		t.Fatalf("expected PATH update:\n%s", got)
	}
	rc := filepath.Join(home, ".zshrc")
	body, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, ">>> polyapi PATH >>>") || !strings.Contains(text, dir) {
		t.Fatalf("rc missing PATH block:\n%s", text)
	}
	if !strings.Contains(text, `export PATH="`+dir+`:$PATH"`) {
		t.Fatalf("rc missing export:\n%s", text)
	}
}

func TestInstallScriptSkipsPathWhenPresent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix install.sh")
	}
	root := repoRoot(t)
	script := filepath.Join(root, "scripts", "install.sh")
	home := t.TempDir()
	dir := filepath.Join(home, "opt")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	dir = absDir(t, dir)

	payload := []byte("installed-from-script")
	tag := "v9.9.9"
	srv := serveInstall(t, tag, payload, checksumsFor(t, tag, payload))

	cmd := exec.Command("sh", script)
	cmd.Env = installEnv(map[string]string{
		"HOME":                  home,
		"SHELL":                 "/bin/zsh",
		"PATH":                  dir + ":/usr/bin:/bin",
		"POLYAPI_INSTALL_DIR":   dir,
		"POLYAPI_RELEASES_URL":  srv.URL + "/releases/latest",
		"POLYAPI_DOWNLOAD_BASE": srv.URL + "/download",
	})
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install.sh: %v\n%s", err, out)
	}
	got := string(out)
	if !strings.Contains(got, "polyapi is on PATH") {
		t.Fatalf("expected already-on-PATH:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(home, ".zshrc")); !os.IsNotExist(err) {
		t.Fatalf("wrote rc despite dest already on PATH: %v", err)
	}
}

func TestInstallScriptPathIdempotent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix install.sh")
	}
	root := repoRoot(t)
	script := filepath.Join(root, "scripts", "install.sh")
	home := t.TempDir()
	dir := filepath.Join(home, "opt")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	payload := []byte("installed-from-script")
	tag := "v9.9.9"
	srv := serveInstall(t, tag, payload, checksumsFor(t, tag, payload))
	env := installEnv(map[string]string{
		"HOME":                  home,
		"SHELL":                 "/bin/zsh",
		"PATH":                  "/usr/bin:/bin",
		"POLYAPI_INSTALL_DIR":   dir,
		"POLYAPI_RELEASES_URL":  srv.URL + "/releases/latest",
		"POLYAPI_DOWNLOAD_BASE": srv.URL + "/download",
	})

	run := func() string {
		cmd := exec.Command("sh", script)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("install.sh: %v\n%s", err, out)
		}
		return string(out)
	}
	if got := run(); !strings.Contains(got, "PATH: adding") {
		t.Fatalf("first run:\n%s", got)
	}
	if got := run(); !strings.Contains(got, "already in") {
		t.Fatalf("second run should reuse the rc block:\n%s", got)
	}
	body, err := os.ReadFile(filepath.Join(home, ".zshrc"))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(body), ">>> polyapi PATH >>>"); n != 1 {
		t.Fatalf("want 1 PATH block, got %d:\n%s", n, body)
	}
}

func TestGitignoreDoesNotIgnoreCmdPackage(t *testing.T) {
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line == "polyapi" || line == "polyapi/" || line == "**/polyapi" {
			t.Fatalf(".gitignore:%d %q ignores src/cmd/polyapi; use /polyapi for the root binary", i+1, line)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "src", "cmd", "polyapi", "main.go")); err != nil {
		t.Fatalf("command package missing: %v", err)
	}
}

func TestBuildScriptNative(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix build.sh")
	}
	root := repoRoot(t)
	outDir := t.TempDir()
	cmd := exec.Command("sh", filepath.Join(root, "scripts", "build.sh"), "--native")
	cmd.Env = append(os.Environ(),
		"POLYAPI_OUT="+outDir,
		"POLYAPI_VERSION=0.0.0-test",
		"POLYAPI_COMMIT=deadbeef",
		"POLYAPI_DATE=2026-01-01T00:00:00Z",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build.sh: %v\n%s", err, out)
	}
	asset := version.AssetName("0.0.0-test", runtime.GOOS, runtime.GOARCH)
	bin := filepath.Join(outDir, asset)
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("missing %s\n%s", bin, out)
	}
	sums := filepath.Join(outDir, version.ChecksumsNameFor("0.0.0-test"))
	if _, err := os.Stat(sums); err != nil {
		t.Fatalf("missing checksums: %v", err)
	}
	got, err := exec.Command(bin, "version").CombinedOutput()
	if err != nil {
		t.Fatalf("version: %v\n%s", err, got)
	}
	text := string(got)
	if !strings.Contains(text, "0.0.0-test") {
		t.Fatalf("stamped version missing:\n%s", text)
	}
	if !strings.Contains(text, "deadbeef") {
		t.Fatalf("stamped commit missing:\n%s", text)
	}
}
