package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/polyapi/polyglot/src/exitcode"
)

func isolateCLI(t *testing.T, dir string) {
	t.Helper()
	t.Chdir(dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	t.Setenv("HOME", dir)
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CLICOLOR_FORCE", "")
	t.Setenv("POLY_API_KEY", "")
	t.Setenv("POLY_API_BASE_URL", "")
	os.Unsetenv("POLY_API_KEY")
	os.Unsetenv("POLY_API_BASE_URL")
}

func runDoctor(args ...string) (stdout, stderr string, code int) {
	all := append([]string{"--non-interactive", "doctor"}, args...)
	return run(all...)
}

func TestDoctorMissingKeyIs3(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	stdout, stderr, code := runDoctor("--offline")
	if code != exitcode.Auth {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout + stderr)
	if !strings.Contains(got, "no API key") && !strings.Contains(got, "auth login") {
		t.Fatalf("expected missing-key message:\n%s", got)
	}
	if !strings.Contains(plain(stdout), "config") {
		t.Fatalf("expected config check:\n%s", stdout)
	}
}

func TestDoctorMissingAdapterIs10(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"polyapi":"1.0.0"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("POLY_API_KEY", "test-key-xxxx")
	t.Setenv("POLY_API_BASE_URL", "http://127.0.0.1:9")
	stdout, stderr, code := runDoctor("--offline")
	if code != exitcode.AdapterMissing {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := strings.ToLower(plain(stdout + stderr))
	if !strings.Contains(got, "adapter") && !strings.Contains(got, "sdk") {
		t.Fatalf("expected adapter failure:\n%s", stdout+stderr)
	}
}

func TestDoctorAdapterOKOffline(t *testing.T) {
	if !onPath("node") {
		t.Skip("node not on PATH")
	}
	dir := t.TempDir()
	writeTSProject(t, dir)
	isolateCLI(t, dir)
	t.Setenv("POLY_API_KEY", "test-key-xxxx")
	t.Setenv("POLY_API_BASE_URL", "http://127.0.0.1:9")
	stdout, stderr, code := runDoctor("--offline")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout)
	if !strings.Contains(got, "adapter") || !strings.Contains(got, "typescript") {
		t.Fatalf("expected adapter ok:\n%s", got)
	}
	if !strings.Contains(got, "protocol") || !strings.Contains(got, "ok") {
		t.Fatalf("expected protocol/binary ok:\n%s", got)
	}
	if !strings.Contains(got, "skipped (--offline)") {
		t.Fatalf("expected offline auth skip:\n%s", got)
	}
	if !strings.Contains(got, "validate") {
		t.Fatalf("expected validate check:\n%s", got)
	}
}

func TestDoctorAuthOK(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key-xxxx" {
			t.Errorf("auth %s", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tenant":      map[string]string{"id": "ten-1"},
			"environment": map[string]string{"id": "env-1"},
			"permissions": map[string]bool{"libraryGenerate": true},
		})
	}))
	t.Cleanup(srv.Close)
	t.Setenv("POLY_API_KEY", "test-key-xxxx")
	t.Setenv("POLY_API_BASE_URL", srv.URL)
	stdout, stderr, code := runDoctor()
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout)
	if !strings.Contains(got, "tenant=ten-1") || !strings.Contains(got, "environment=env-1") {
		t.Fatalf("expected auth details:\n%s", got)
	}
}

func TestDoctorAuthRejectedIs3(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"nope"}`, http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("POLY_API_KEY", "bad-key")
	t.Setenv("POLY_API_BASE_URL", srv.URL)
	stdout, stderr, code := runDoctor()
	if code != exitcode.Auth {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := strings.ToLower(plain(stdout + stderr))
	if !strings.Contains(got, "401") && !strings.Contains(got, "api key") {
		t.Fatalf("expected 401 wording:\n%s", stdout+stderr)
	}
}

func TestDoctorDeployBranchAllowed(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	writeGitHEAD(t, dir, "main")
	writeDeployConfig(t, dir, "error", "main", "prod")
	t.Setenv("POLY_API_KEY", "test-key-xxxx")
	t.Setenv("POLY_API_BASE_URL", "http://127.0.0.1:9")
	stdout, stderr, code := runDoctor("--offline")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout)
	if !strings.Contains(got, "branch main") || !strings.Contains(got, "prod") || !strings.Contains(got, "push allowed") {
		t.Fatalf("expected deploy ok:\n%s", got)
	}
}

func TestDoctorDeployBranchBlocked(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	writeGitHEAD(t, dir, "feature/x")
	writeDeployConfig(t, dir, "error", "main", "prod")
	t.Setenv("POLY_API_KEY", "test-key-xxxx")
	t.Setenv("POLY_API_BASE_URL", "http://127.0.0.1:9")
	stdout, stderr, code := runDoctor("--offline")
	if code != exitcode.Failure {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout + stderr)
	if !strings.Contains(got, "feature/x") || !strings.Contains(got, "push blocked") {
		t.Fatalf("expected blocked branch:\n%s", got)
	}
}

func writeGitHEAD(t *testing.T, dir, branch string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/"+branch+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeDeployConfig(t *testing.T, dir, unallowed, branch, env string) {
	t.Helper()
	poly := filepath.Join(dir, ".poly")
	if err := os.MkdirAll(poly, 0o755); err != nil {
		t.Fatal(err)
	}
	text := "" +
		"instance = \"na1\"\n" +
		"base_url = \"https://na1.polyapi.io\"\n" +
		"\n" +
		"[deploy]\n" +
		"unallowed_branch = \"" + unallowed + "\"\n" +
		"\n" +
		"[[deploy.targets]]\n" +
		"branches = [\"" + branch + "\"]\n" +
		"environment = \"" + env + "\"\n"
	if err := os.WriteFile(filepath.Join(poly, "config.toml"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
