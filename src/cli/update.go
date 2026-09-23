package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/polyapi/polyglot/src/exitcode"
	"github.com/polyapi/polyglot/src/version"
	"github.com/spf13/cobra"
)

const (
	defaultReleaseAPI   = "https://api.github.com/repos/polyapi/polyglot/releases/latest"
	defaultDownloadBase = "https://github.com/polyapi/polyglot/releases/download"
	installScriptURL    = "https://raw.githubusercontent.com/polyapi/polyglot/main/scripts/install.sh"
	installModule       = "github.com/polyapi/polyglot/src/cmd/polyapi@latest"
	releasesPage        = "https://github.com/polyapi/polyglot/releases"
	maxAssetBytes       = 50 << 20
	maxChecksumBytes    = 1 << 20
)

// ErrNoReleases is returned when GitHub has no published releases yet.
var ErrNoReleases = errors.New("no GitHub releases published yet")

// Release is a GitHub release summary.
type Release struct {
	Tag    string
	URL    string
	Name   string
	Assets []ReleaseAsset
}

// ReleaseAsset is a downloadable file on a GitHub release.
type ReleaseAsset struct {
	Name string
	URL  string
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Name    string `json:"name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func runUpdate(cmd *cobra.Command, _ []string) error {
	checkOnly, _ := cmd.Flags().GetBool("check")
	info := version.Current()
	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()

	client := &http.Client{Timeout: 2 * time.Minute}
	rel, err := FetchLatestRelease(client, releaseAPIURL())
	if err != nil {
		if errors.Is(err, ErrNoReleases) {
			printOk(out, fmt.Sprintf("polyapi %s; no GitHub releases published yet", info.Version))
			printInstallInstructions(out)
			return nil
		}
		printWarning(errOut, "could not check GitHub releases: "+err.Error())
		printInstallInstructions(out)
		if checkOnly {
			return &ExitError{Code: exitcode.Network, Msg: err.Error()}
		}
		return nil
	}

	cmp := cmpSemver(rel.Tag, info.Version)
	switch {
	case cmp > 0:
		printOk(out, fmt.Sprintf("polyapi %s is available (current %s)", strings.TrimPrefix(rel.Tag, "v"), info.Version))
		if rel.URL != "" {
			fmt.Fprintf(out, "  %s\n", rel.URL)
		}
		if checkOnly {
			printInstallInstructions(out)
			return nil
		}
		dest, err := updateDest()
		if err != nil {
			printInstallInstructions(out)
			return &ExitError{Code: exitcode.Failure, Msg: "could not locate this binary: " + err.Error()}
		}
		if err := applyRelease(client, rel, dest); err != nil {
			printWarning(errOut, err.Error())
			printInstallInstructions(out)
			code := exitcode.Failure
			if isNetworkErr(err) {
				code = exitcode.Network
			}
			return &ExitError{Code: code, Msg: err.Error()}
		}
		printOk(out, fmt.Sprintf("updated %s to %s", dest, version.NormalizeVersion(rel.Tag)))
	case cmp < 0:
		printOk(out, fmt.Sprintf("polyapi %s is newer than the latest GitHub release (%s)", info.Version, rel.Tag))
	default:
		printOk(out, fmt.Sprintf("polyapi %s is up to date", info.Version))
	}
	return nil
}

func printInstallInstructions(w io.Writer) {
	fmt.Fprintln(w, "Install:")
	fmt.Fprintf(w, "  curl -fsSL %s | sh\n", installScriptURL)
	fmt.Fprintf(w, "  go install %s\n", installModule)
	fmt.Fprintf(w, "  # or download a tagged binary from %s\n", releasesPage)
}

func releaseAPIURL() string {
	if u := strings.TrimSpace(os.Getenv("POLY_UPDATE_API")); u != "" {
		return u
	}
	return defaultReleaseAPI
}

func downloadBase() string {
	if u := strings.TrimSpace(os.Getenv("POLY_UPDATE_DOWNLOAD")); u != "" {
		return strings.TrimRight(u, "/")
	}
	return defaultDownloadBase
}

func updateDest() (string, error) {
	if d := strings.TrimSpace(os.Getenv("POLY_UPDATE_DEST")); d != "" {
		return d, nil
	}
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved, nil
	}
	return p, nil
}

func applyRelease(client *http.Client, rel Release, dest string) error {
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	ver := version.NormalizeVersion(rel.Tag)
	asset := version.AssetName(ver, runtime.GOOS, runtime.GOARCH)
	sums, err := downloadBody(client, rel.assetURL(version.ChecksumsName), maxChecksumBytes)
	if err != nil {
		return fmt.Errorf("download checksums: %w", err)
	}
	want, err := checksumFor(string(sums), asset)
	if err != nil {
		return err
	}
	body, err := downloadBody(client, rel.assetURL(asset), maxAssetBytes)
	if err != nil {
		return fmt.Errorf("download %s: %w", asset, err)
	}
	got := sha256Hex(body)
	if got != want {
		return fmt.Errorf("checksum mismatch for %s", asset)
	}
	if err := replaceExecutable(dest, body); err != nil {
		return fmt.Errorf("replace %s: %w", dest, err)
	}
	return nil
}

func (r Release) assetURL(name string) string {
	for _, a := range r.Assets {
		if a.Name == name && a.URL != "" {
			return a.URL
		}
	}
	return downloadBase() + "/" + r.Tag + "/" + name
}

func downloadBody(client *http.Client, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "polyapi/"+version.Version)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("response too large")
	}
	return data, nil
}

func checksumFor(list, filename string) (string, error) {
	for _, line := range strings.Split(list, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if name == filename {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("no checksum for %s", filename)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func replaceExecutable(dest string, data []byte) error {
	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".polyapi-update-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o755); err != nil && runtime.GOOS != "windows" {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		old := dest + ".old"
		_ = os.Remove(old)
		if _, err := os.Stat(dest); err == nil {
			if err := os.Rename(dest, old); err != nil {
				return err
			}
		}
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return err
	}
	ok = true
	return nil
}

func isNetworkErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "download") || strings.Contains(msg, "http ") || strings.Contains(msg, "connection")
}

// FetchLatestRelease loads the latest GitHub release. Tests pass a client and URL.
func FetchLatestRelease(client *http.Client, apiURL string) (Release, error) {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if apiURL == "" {
		apiURL = defaultReleaseAPI
	}
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("User-Agent", "polyapi/"+version.Version)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Release{}, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return Release{}, ErrNoReleases
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Release{}, fmt.Errorf("GitHub releases HTTP %d", resp.StatusCode)
	}
	var raw githubRelease
	if err := json.Unmarshal(body, &raw); err != nil {
		return Release{}, err
	}
	if strings.TrimSpace(raw.TagName) == "" {
		return Release{}, ErrNoReleases
	}
	rel := Release{Tag: raw.TagName, URL: raw.HTMLURL, Name: raw.Name}
	for _, a := range raw.Assets {
		if a.Name == "" {
			continue
		}
		rel.Assets = append(rel.Assets, ReleaseAsset{Name: a.Name, URL: a.URL})
	}
	return rel, nil
}

func cmpSemver(a, b string) int {
	as := semverParts(a)
	bs := semverParts(b)
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		var ai, bi int
		if i < len(as) {
			ai = as[i]
		}
		if i < len(bs) {
			bi = bs[i]
		}
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	return 0
}

func semverParts(s string) []int {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	var parts []int
	for _, p := range strings.Split(s, ".") {
		n, _ := strconv.Atoi(leadingDigits(p))
		parts = append(parts, n)
	}
	return parts
}

func leadingDigits(s string) string {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return "0"
	}
	return s[:i]
}
