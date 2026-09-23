package version

import (
	"fmt"
	"strings"
)

// ChecksumsName is the release-asset filename for SHA-256 sums.
const ChecksumsName = "checksums.txt"

// NormalizeVersion strips a leading "v" from a tag or version string.
func NormalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

// AssetName is the published binary filename for a version and GOOS/GOARCH.
// Windows assets use a .exe suffix; Unix assets have no suffix.
func AssetName(ver, goos, goarch string) string {
	name := fmt.Sprintf("polyapi-%s-%s-%s", NormalizeVersion(ver), goos, goarch)
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

// ChecksumsNameFor is the per-version checksums filename used in builds/
// so successive local builds can coexist.
func ChecksumsNameFor(ver string) string {
	return "checksums-" + NormalizeVersion(ver) + ".txt"
}
