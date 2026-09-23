package cli_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/polyapi/polyglot/src/cli"
	"github.com/polyapi/polyglot/src/exitcode"
	"github.com/polyapi/polyglot/src/version"
)

func TestFetchLatestRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("missing User-Agent")
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"tag_name": "v0.2.0",
			"html_url": "https://github.com/polyapi/polyglot/releases/tag/v0.2.0",
			"name":     "0.2.0",
		})
	}))
	t.Cleanup(srv.Close)
	rel, err := cli.FetchLatestRelease(srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if rel.Tag != "v0.2.0" || !strings.Contains(rel.URL, "v0.2.0") {
		t.Fatalf("%+v", rel)
	}
}

func TestFetchLatestReleaseNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	_, err := cli.FetchLatestRelease(srv.Client(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "no GitHub releases") {
		t.Fatalf("%v", err)
	}
}

func TestUpdateCheckNewerRelease(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"tag_name": "v9.9.9",
			"html_url": "https://github.com/polyapi/polyglot/releases/tag/v9.9.9",
		})
	}))
	t.Cleanup(srv.Close)
	t.Setenv("POLY_UPDATE_API", srv.URL)
	stdout, stderr, code := run("update", "--check")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout)
	if !strings.Contains(got, "9.9.9") || !strings.Contains(got, "0.1.0") {
		t.Fatalf("expected newer-release notice:\n%s", got)
	}
	if !strings.Contains(got, "go install") {
		t.Fatalf("expected install instructions:\n%s", got)
	}
	if !strings.Contains(got, "scripts/install.sh") {
		t.Fatalf("expected install.sh:\n%s", got)
	}
}

func TestUpdateCheckUpToDate(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"tag_name": "v0.1.0",
			"html_url": "https://github.com/polyapi/polyglot/releases/tag/v0.1.0",
		})
	}))
	t.Cleanup(srv.Close)
	t.Setenv("POLY_UPDATE_API", srv.URL)
	stdout, stderr, code := run("update", "--check")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(plain(stdout), "up to date") {
		t.Fatalf("%s", stdout)
	}
}

func TestUpdateCheckNoReleases(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("POLY_UPDATE_API", srv.URL)
	stdout, stderr, code := run("update", "--check")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout)
	if !strings.Contains(got, "no GitHub releases") {
		t.Fatalf("%s", stdout)
	}
	if !strings.Contains(got, "go install") {
		t.Fatalf("expected install instructions:\n%s", got)
	}
}

func TestUpdateCheckNetworkIs4(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	t.Setenv("POLY_UPDATE_API", "http://127.0.0.1:1")
	_, stderr, code := run("update", "--check")
	if code != exitcode.Network {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
}

func TestUpdateHelp(t *testing.T) {
	stdout, _, code := run("update", "--help")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	got := plain(stdout)
	if !strings.Contains(got, "--check") || !strings.Contains(got, "polyapi update") {
		t.Fatalf("%s", got)
	}
}

func TestUpdateReplacesBinary(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	dest := filepath.Join(dir, "polyapi")
	if err := os.WriteFile(dest, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	payload := []byte("new-polyapi-binary")
	srv := releaseServer(t, "v9.9.9", payload, true)
	t.Setenv("POLY_UPDATE_API", srv.URL)
	t.Setenv("POLY_UPDATE_DEST", dest)
	stdout, stderr, code := run("update")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := plain(stdout)
	if !strings.Contains(got, "updated") || !strings.Contains(got, "9.9.9") {
		t.Fatalf("expected update notice:\n%s", got)
	}
	body, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != string(payload) {
		t.Fatalf("dest=%q", body)
	}
}

func TestUpdateChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	dest := filepath.Join(dir, "polyapi")
	if err := os.WriteFile(dest, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	srv := releaseServer(t, "v9.9.9", []byte("new-polyapi-binary"), false)
	t.Setenv("POLY_UPDATE_API", srv.URL)
	t.Setenv("POLY_UPDATE_DEST", dest)
	_, stderr, code := run("update")
	if code != exitcode.Failure {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
	if !strings.Contains(plain(stderr), "checksum") {
		t.Fatalf("stderr=%s", stderr)
	}
	body, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "old-binary" {
		t.Fatalf("replaced dest despite checksum mismatch: %q", body)
	}
}

func TestUpdateSkipsReplaceWhenCurrent(t *testing.T) {
	dir := t.TempDir()
	isolateCLI(t, dir)
	dest := filepath.Join(dir, "polyapi")
	if err := os.WriteFile(dest, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	payload := []byte("should-not-be-written")
	srv := releaseServer(t, "v0.1.0", payload, true)
	t.Setenv("POLY_UPDATE_API", srv.URL)
	t.Setenv("POLY_UPDATE_DEST", dest)
	stdout, stderr, code := run("update")
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(plain(stdout), "up to date") {
		t.Fatalf("%s", stdout)
	}
	body, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "old-binary" {
		t.Fatalf("replaced dest while up to date: %q", body)
	}
}

func releaseServer(t *testing.T, tag string, payload []byte, validChecksum bool) *httptest.Server {
	t.Helper()
	ver := version.NormalizeVersion(tag)
	asset := version.AssetName(ver, runtime.GOOS, runtime.GOARCH)
	sum := sha256.Sum256(payload)
	want := hex.EncodeToString(sum[:])
	if !validChecksum {
		want = strings.Repeat("0", 64)
	}
	checksums := want + "  " + asset + "\n"
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "/latest":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tag_name": tag,
				"html_url": srv.URL + "/tag/" + tag,
				"assets": []map[string]string{
					{"name": asset, "browser_download_url": srv.URL + "/dl/" + asset},
					{"name": version.ChecksumsName, "browser_download_url": srv.URL + "/dl/" + version.ChecksumsName},
				},
			})
		case "/dl/" + asset:
			_, _ = w.Write(payload)
		case "/dl/" + version.ChecksumsName:
			_, _ = w.Write([]byte(checksums))
		default:
			http.NotFound(w, r)
		}
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}
