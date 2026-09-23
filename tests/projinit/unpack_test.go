package projinit_test

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/polyapi/polyglot/src/projinit"
)

func zipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestUnpackFlattensGitHubRoot(t *testing.T) {
	data := zipBytes(t, map[string]string{
		"repo-main/package.json": `{"name":"demo"}`,
		"repo-main/src/app.ts":   "export {}",
	})
	dir := t.TempDir()
	res, err := projinit.Unpack(data, dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Wrote) != 2 {
		t.Fatalf("wrote %v", res.Wrote)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("demo")) {
		t.Fatalf("package.json=%s", raw)
	}
	if _, err := os.Stat(filepath.Join(dir, "src", "app.ts")); err != nil {
		t.Fatal(err)
	}
}

func TestUnpackSkipsConflicts(t *testing.T) {
	data := zipBytes(t, map[string]string{"hello.txt": "from-zip\n"})
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := projinit.Unpack(data, dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Skipped) != 1 || len(res.Wrote) != 0 {
		t.Fatalf("%+v", res)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "hello.txt"))
	if string(got) != "keep\n" {
		t.Fatalf("got %q", got)
	}
	res, err = projinit.Unpack(data, dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Wrote) != 1 {
		t.Fatalf("%+v", res)
	}
	got, _ = os.ReadFile(filepath.Join(dir, "hello.txt"))
	if string(got) != "from-zip\n" {
		t.Fatalf("got %q", got)
	}
}

func TestUnpackRejectsZipSlip(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	h := &zip.FileHeader{Name: "../evil.txt", Method: zip.Store}
	f, err := w.CreateHeader(h)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("nope")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if _, err := projinit.Unpack(buf.Bytes(), dir, true); err == nil {
		t.Fatal("expected zip-slip error")
	}
}

func TestMissingRuntimeMessage(t *testing.T) {
	n := projinit.MissingRuntimeMessage("typescript")
	if !bytes.Contains([]byte(n), []byte(projinit.NodeDownloadURL)) {
		t.Fatalf("node message:\n%s", n)
	}
	p := projinit.MissingRuntimeMessage("python")
	if !bytes.Contains([]byte(p), []byte(projinit.PythonDownloadURL)) || !bytes.Contains([]byte(p), []byte(projinit.PythonVenvPrimer)) {
		t.Fatalf("python message:\n%s", p)
	}
}

func TestEnsureRuntimeMissingNoAsk(t *testing.T) {
	orig := projinit.LookPath
	extra := projinit.ExtraBinDirs
	t.Cleanup(func() {
		projinit.LookPath = orig
		projinit.ExtraBinDirs = extra
	})
	projinit.LookPath = func(string) (string, error) { return "", os.ErrNotExist }
	projinit.ExtraBinDirs = func(string) []string { return nil }
	_, err := projinit.EnsureRuntime("typescript", nil, nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !bytes.Contains([]byte(err.Error()), []byte(projinit.NodeDownloadURL)) {
		t.Fatalf("%v", err)
	}
}

func TestEnsureRuntimeDecline(t *testing.T) {
	orig := projinit.LookPath
	extra := projinit.ExtraBinDirs
	t.Cleanup(func() {
		projinit.LookPath = orig
		projinit.ExtraBinDirs = extra
	})
	projinit.LookPath = func(string) (string, error) { return "", os.ErrNotExist }
	projinit.ExtraBinDirs = func(string) []string { return nil }
	_, err := projinit.EnsureRuntime("python", func(projinit.RuntimeKind) (bool, error) {
		return false, nil
	}, nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("re-run `polyapi init`")) {
		t.Fatalf("%v", err)
	}
}
