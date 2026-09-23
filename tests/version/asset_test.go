package version_test

import (
	"testing"

	"github.com/polyapi/polyglot/src/version"
)

func TestAssetName(t *testing.T) {
	cases := []struct {
		ver, goos, goarch, want string
	}{
		{"v0.2.0", "linux", "amd64", "polyapi-0.2.0-linux-amd64"},
		{"0.2.0", "darwin", "arm64", "polyapi-0.2.0-darwin-arm64"},
		{"v1.0.0", "windows", "amd64", "polyapi-1.0.0-windows-amd64.exe"},
		{"1.0.0", "windows", "arm64", "polyapi-1.0.0-windows-arm64.exe"},
	}
	for _, tc := range cases {
		got := version.AssetName(tc.ver, tc.goos, tc.goarch)
		if got != tc.want {
			t.Errorf("AssetName(%q,%q,%q)=%q want %q", tc.ver, tc.goos, tc.goarch, got, tc.want)
		}
	}
}

func TestChecksumsNameFor(t *testing.T) {
	if got := version.ChecksumsNameFor("v0.2.0"); got != "checksums-0.2.0.txt" {
		t.Fatalf("%s", got)
	}
	if version.ChecksumsName != "checksums.txt" {
		t.Fatalf("%s", version.ChecksumsName)
	}
}
