package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/polyapi/polyglot/src/config"
)

func TestResolveBaseURL(t *testing.T) {
	got, err := config.ResolveBaseURL("na1")
	if err != nil || got != "https://na1.polyapi.io" {
		t.Fatalf("na1: %q %v", got, err)
	}
	got, err = config.ResolveBaseURL("develop")
	if err != nil || got != "https://dev.polyapi.io" {
		t.Fatalf("develop: %q %v", got, err)
	}
	if config.InstanceName("develop") != "dev" {
		t.Fatalf("instance develop: %q", config.InstanceName("develop"))
	}
	got, err = config.ResolveBaseURL("https://na1.polyapi.io/")
	if err != nil || got != "https://na1.polyapi.io" {
		t.Fatalf("slash: %q %v", got, err)
	}
	if config.InstanceName("https://eu1.polyapi.io") != "eu1" {
		t.Fatalf("eu1 url: %q", config.InstanceName("https://eu1.polyapi.io"))
	}
	if _, err := config.ResolveBaseURL("na1.polyapi.io"); err == nil {
		t.Fatal("expected invalid URL")
	}
}

func TestEncryptRoundtrip(t *testing.T) {
	secret := bytes32(7)
	token, err := config.EncryptAPIKey("sk-test-secret-value", secret)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(token, "v1:") {
		t.Fatalf("token %q", token)
	}
	plain, err := config.DecryptAPIKey(token, secret)
	if err != nil || plain != "sk-test-secret-value" {
		t.Fatalf("plain %q %v", plain, err)
	}
}

func TestWrongDeviceSecretFails(t *testing.T) {
	token, err := config.EncryptAPIKey("sk-test", bytes32(1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.DecryptAPIKey(token, bytes32(2)); err == nil {
		t.Fatal("expected decrypt error")
	}
}

func TestDeviceSecretPersists(t *testing.T) {
	dir := t.TempDir()
	ctx := config.ForTests(dir)
	a, err := config.DeviceSecret(ctx)
	if err != nil {
		t.Fatal(err)
	}
	b, err := config.DeviceSecret(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 32 || string(a) != string(b) {
		t.Fatalf("a=%x b=%x", a, b)
	}
}

func TestRedactKeepsLastFour(t *testing.T) {
	if got := config.RedactSecret("abcdefghij"); got != "********ghij" {
		t.Fatalf("got %q", got)
	}
	if got := config.RedactSecret(""); got != "(unset)" {
		t.Fatalf("got %q", got)
	}
}

func TestEnvBeatsProject(t *testing.T) {
	dir := t.TempDir()
	secrets := config.ForTests(filepath.Join(dir, "user"))
	if _, err := config.SaveProjectCredentials(dir, ".poly", "na1", "project-key-xxxx", "1", secrets); err != nil {
		t.Fatal(err)
	}
	t.Setenv("POLY_API_BASE_URL", "eu1")
	t.Setenv("POLY_API_KEY", "env-key-zzzz")
	resolved, err := config.Load(dir, ".poly", config.Overlays{}, secrets)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.BaseURL != "https://eu1.polyapi.io" {
		t.Fatalf("url %q", resolved.BaseURL)
	}
	if resolved.APIKey != "env-key-zzzz" {
		t.Fatalf("key %q", resolved.APIKey)
	}
	if resolved.KeySource != config.SourceEnv || resolved.URLSource != config.SourceEnv {
		t.Fatalf("url_source=%q key_source=%q", resolved.URLSource, resolved.KeySource)
	}
}

func TestFlagBeatsProject(t *testing.T) {
	dir := t.TempDir()
	secrets := config.ForTests(filepath.Join(dir, "user"))
	if _, err := config.SaveProjectCredentials(dir, ".poly", "na1", "project-key-xxxx", "1", secrets); err != nil {
		t.Fatal(err)
	}
	resolved, err := config.Load(dir, ".poly", config.Overlays{
		BaseURL: &config.Overlay{Value: "eu1", Source: config.SourceFlag},
		APIKey:  &config.Overlay{Value: "flag-key-zzzz", Source: config.SourceFlag},
	}, secrets)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.BaseURL != "https://eu1.polyapi.io" {
		t.Fatalf("url %q", resolved.BaseURL)
	}
	if resolved.APIKey != "flag-key-zzzz" {
		t.Fatalf("key %q", resolved.APIKey)
	}
	if resolved.KeySource != config.SourceFlag {
		t.Fatalf("source %q", resolved.KeySource)
	}
	dump := resolved.RedactedTOML()
	if strings.Contains(dump, "flag-key-zzzz") {
		t.Fatalf("leaked key:\n%s", dump)
	}
	if !strings.Contains(dump, "********zzzz") {
		t.Fatalf("missing redaction:\n%s", dump)
	}
}

func TestProjectBeatsLegacy(t *testing.T) {
	dir := t.TempDir()
	secrets := config.ForTests(filepath.Join(dir, "user"))
	if err := os.MkdirAll(filepath.Join(dir, "node_modules", ".poly"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "node_modules", ".poly", ".config.env"), []byte("POLY_API_KEY=legacy-key-1111\nPOLY_API_BASE_URL=https://na1.polyapi.io\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := config.SaveProjectCredentials(dir, ".poly", "eu1", "project-key-2222", "1", secrets); err != nil {
		t.Fatal(err)
	}
	resolved, err := config.Load(dir, ".poly", config.Overlays{}, secrets)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.APIKey != "project-key-2222" {
		t.Fatalf("key %q", resolved.APIKey)
	}
	if resolved.KeySource != config.SourceProject {
		t.Fatalf("source %q", resolved.KeySource)
	}
	if resolved.Instance != "eu1" {
		t.Fatalf("instance %q", resolved.Instance)
	}
}

func TestCiphertextIsNotPlaintext(t *testing.T) {
	dir := t.TempDir()
	secrets := config.ForTests(filepath.Join(dir, "user"))
	path, err := config.SaveProjectCredentials(dir, ".poly", "na1", "super-secret-key", "1", secrets)
	if err != nil {
		t.Fatal(err)
	}
	disk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(disk), "super-secret-key") {
		t.Fatalf("plaintext on disk:\n%s", disk)
	}
	if !strings.Contains(string(disk), "api_key_encrypted") {
		t.Fatalf("missing ciphertext field:\n%s", disk)
	}
}

func TestLogoutClearsKey(t *testing.T) {
	dir := t.TempDir()
	secrets := config.ForTests(filepath.Join(dir, "user"))
	if _, err := config.SaveProjectCredentials(dir, ".poly", "na1", "to-be-removed-9999", "1", secrets); err != nil {
		t.Fatal(err)
	}
	if err := config.ClearStoredKeys(dir, ".poly", secrets); err != nil {
		t.Fatal(err)
	}
	resolved, err := config.Load(dir, ".poly", config.Overlays{}, secrets)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.APIKey != "" {
		t.Fatalf("key still set %q", resolved.APIKey)
	}
	if resolved.BaseURL != "https://na1.polyapi.io" {
		t.Fatalf("url %q", resolved.BaseURL)
	}
}

func TestLanguageAndAdapterRoundTrip(t *testing.T) {
	dir := t.TempDir()
	secrets := config.ForTests(filepath.Join(dir, "user"))
	poly := filepath.Join(dir, ".poly")
	if err := os.MkdirAll(poly, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(poly, "config.toml"), []byte("language = \"python\"\n\n[adapter]\ncommand = \"python -m polyapi adapter\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, err := config.Load(dir, ".poly", config.Overlays{}, secrets)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Language != "python" {
		t.Fatalf("lang %q", resolved.Language)
	}
	if resolved.AdapterCommand != "python -m polyapi adapter" {
		t.Fatalf("adapter %q", resolved.AdapterCommand)
	}
	if _, err := config.SaveProjectCredentials(dir, ".poly", "na1", "keep-adapter-xxxx", "1", secrets); err != nil {
		t.Fatal(err)
	}
	after, err := config.Load(dir, ".poly", config.Overlays{}, secrets)
	if err != nil {
		t.Fatal(err)
	}
	if after.Language != "python" || after.AdapterCommand != "python -m polyapi adapter" {
		t.Fatalf("after lang=%q adapter=%q", after.Language, after.AdapterCommand)
	}
}

func TestLegacyReaders(t *testing.T) {
	t.Run("ts", func(t *testing.T) {
		dir := t.TempDir()
		poly := filepath.Join(dir, "node_modules", ".poly")
		if err := os.MkdirAll(poly, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(poly, ".config.env"), []byte("POLY_API_KEY=ts-key-1234\nPOLY_API_BASE_URL=https://na1.polyapi.io\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		creds := config.ReadLegacy(dir)
		if creds == nil || creds.Source != config.SourceLegacyTS || creds.APIKey != "ts-key-1234" {
			t.Fatalf("%+v", creds)
		}
	})
	t.Run("python", func(t *testing.T) {
		dir := t.TempDir()
		poly := filepath.Join(dir, "polyapi")
		if err := os.MkdirAll(poly, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(poly, ".config.env"), []byte("[polyapi]\npoly_api_key = py-key\npoly_api_base_url = https://eu1.polyapi.io\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		creds := config.ReadLegacy(dir)
		if creds == nil || creds.Source != config.SourceLegacyPython || creds.BaseURL != "https://eu1.polyapi.io" {
			t.Fatalf("%+v", creds)
		}
	})
	t.Run("java", func(t *testing.T) {
		dir := t.TempDir()
		mvn := filepath.Join(dir, ".mvn")
		if err := os.MkdirAll(mvn, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(mvn, "settings.xml"), []byte("<settings><poly.hostUrl>https://na1.polyapi.io</poly.hostUrl><poly.apiKey>java-key</poly.apiKey></settings>"), 0o644); err != nil {
			t.Fatal(err)
		}
		creds := config.ReadLegacy(dir)
		if creds == nil || creds.Source != config.SourceLegacyJava || creds.APIKey != "java-key" {
			t.Fatalf("%+v", creds)
		}
	})
}

func TestGitignore(t *testing.T) {
	t.Run("skips non-git", func(t *testing.T) {
		dir := t.TempDir()
		act, err := config.EnsurePolyGitignored(dir)
		if err != nil || act != config.GitignoreSkippedNotGit {
			t.Fatalf("%v %v", act, err)
		}
		if _, err := os.Stat(filepath.Join(dir, ".gitignore")); !os.IsNotExist(err) {
			t.Fatal("should not create gitignore")
		}
	})
	t.Run("creates", func(t *testing.T) {
		dir := gitRepo(t)
		act, err := config.EnsurePolyGitignored(dir)
		if err != nil || act != config.GitignoreCreated {
			t.Fatalf("%v %v", act, err)
		}
		text, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
		if string(text) != ".poly/\n" {
			t.Fatalf("%q", text)
		}
		act, err = config.EnsurePolyGitignored(dir)
		if err != nil || act != config.GitignoreAlreadyPresent {
			t.Fatalf("%v %v", act, err)
		}
	})
	t.Run("appends", func(t *testing.T) {
		dir := gitRepo(t)
		if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("target/\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		act, err := config.EnsurePolyGitignored(dir)
		if err != nil || act != config.GitignoreAppended {
			t.Fatalf("%v %v", act, err)
		}
		text, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
		if string(text) != "target/\n.poly/\n" {
			t.Fatalf("%q", text)
		}
	})
}

func TestApplyInitWritesDeployPolicy(t *testing.T) {
	dir := t.TempDir()
	secrets := config.ForTests(filepath.Join(dir, "user"))
	err := config.ApplyInit(dir, ".poly", config.InitSpec{
		Language: "typescript",
		EnvMap:   []string{"main=prod", "develop=dev"},
	}, secrets)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := config.Load(dir, ".poly", config.Overlays{}, secrets)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Language != "typescript" {
		t.Fatalf("language %q", resolved.Language)
	}
	if resolved.File.Deploy == nil || resolved.File.Deploy.UnallowedBranch != "error" {
		t.Fatalf("deploy %+v", resolved.File.Deploy)
	}
	if len(resolved.File.Deploy.Targets) != 2 {
		t.Fatalf("targets %+v", resolved.File.Deploy.Targets)
	}
}

func TestPaths(t *testing.T) {
	got := config.ProjectConfigFile("/repo", ".poly")
	want := filepath.Join("/repo", ".poly", "config.toml")
	if got != want {
		t.Fatalf("%q != %q", got, want)
	}
	if config.PolyDir("/repo", "/tmp/custom-poly") != "/tmp/custom-poly" {
		t.Fatalf("%q", config.PolyDir("/repo", "/tmp/custom-poly"))
	}
	path := config.UserConfigFile()
	if filepath.Base(path) != "config.toml" {
		t.Fatalf("%q", path)
	}
}

func TestSettingAndSetProject(t *testing.T) {
	dir := t.TempDir()
	secrets := config.ForTests(filepath.Join(dir, "user"))
	if _, err := config.SaveProjectCredentials(dir, ".poly", "na1", "project-key-xxxx", "1", secrets); err != nil {
		t.Fatal(err)
	}
	if err := config.SetProjectSetting(dir, ".poly", "language", "typescript"); err != nil {
		t.Fatal(err)
	}
	if err := config.SetProjectSetting(dir, ".poly", "adapter", "node ./adapter.js"); err != nil {
		t.Fatal(err)
	}
	resolved, err := config.Load(dir, ".poly", config.Overlays{}, secrets)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Language != "typescript" {
		t.Fatalf("language %q", resolved.Language)
	}
	if resolved.AdapterCommand != "node ./adapter.js" {
		t.Fatalf("adapter %q", resolved.AdapterCommand)
	}
	got, err := resolved.Setting("language")
	if err != nil || got != "typescript" {
		t.Fatalf("setting language %q %v", got, err)
	}
	redacted, err := resolved.Setting("api_key")
	if err != nil || strings.Contains(redacted, "project-key-xxxx") {
		t.Fatalf("api_key %q %v", redacted, err)
	}
	if err := config.SetProjectSetting(dir, ".poly", "api_key", "nope"); err == nil {
		t.Fatal("expected secret setting to fail")
	}
}

func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func bytes32(b byte) []byte {
	out := make([]byte, 32)
	for i := range out {
		out[i] = b
	}
	return out
}
