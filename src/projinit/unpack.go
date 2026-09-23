package projinit

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/polyapi/polyglot/src/exitcode"
	"github.com/polyapi/polyglot/src/version"
)

const maxZipBytes = 50 << 20

// HTTPGet downloads url. Tests may override.
var HTTPGet = defaultHTTPGet

func defaultHTTPGet(rawURL string) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", "polyapi/"+version.Version)
	req.Header.Set("Accept", "*/*")
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxZipBytes+1))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if int64(len(body)) > maxZipBytes {
		return nil, resp.StatusCode, fmt.Errorf("download exceeds %d bytes", maxZipBytes)
	}
	return body, resp.StatusCode, nil
}

// FetchZip loads a zip from a local path or an http(s) URL.
func FetchZip(src Source) ([]byte, error) {
	switch src.Kind {
	case KindNone:
		return nil, nil
	case KindZip:
		return fetchZipURL(src.URL)
	case KindGitHub:
		data, err := fetchGitHubZip(src.GitHub, src.Ref)
		if err != nil {
			return nil, err
		}
		return data, nil
	default:
		return nil, usage("unknown template source")
	}
}

func fetchGitHubZip(ownerRepo, ref string) ([]byte, error) {
	urls := []string{GitHubZipURL(ownerRepo, ref)}
	if ref == "" || ref == "main" {
		urls = append(urls, GitHubZipURL(ownerRepo, "master"), FallbackZipURL(ownerRepo, "main"), FallbackZipURL(ownerRepo, "master"))
	} else {
		urls = append(urls, FallbackZipURL(ownerRepo, ref), "https://github.com/"+ownerRepo+"/archive/refs/tags/"+ref+".zip")
	}
	var last error
	seen := map[string]bool{}
	for _, u := range urls {
		if seen[u] {
			continue
		}
		seen[u] = true
		data, status, err := getBytes(u)
		if err == nil && status == http.StatusOK {
			return data, nil
		}
		if status == http.StatusNotFound || status == http.StatusMovedPermanently {
			last = fmt.Errorf("HTTP %d", status)
			continue
		}
		if err != nil {
			last = err
			continue
		}
		last = fmt.Errorf("HTTP %d", status)
	}
	msg := "could not download GitHub repository " + ownerRepo
	if ref != "" {
		msg += " @" + ref
	}
	msg += " (public repositories only)"
	return nil, network(msg, last)
}

func fetchZipURL(raw string) ([]byte, error) {
	if raw == "" {
		return nil, usage("empty template URL")
	}
	if strings.HasPrefix(raw, "file://") {
		p := strings.TrimPrefix(raw, "file://")
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, wrap(exitcode.Failure, "read template zip", err)
		}
		return data, nil
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		data, err := os.ReadFile(raw)
		if err != nil {
			return nil, wrap(exitcode.Failure, "read template zip", err)
		}
		return data, nil
	}
	data, status, err := getBytes(raw)
	if err != nil {
		return nil, network("download template", err)
	}
	if status != http.StatusOK {
		return nil, network(fmt.Sprintf("download template: HTTP %d", status), nil)
	}
	return data, nil
}

func getBytes(rawURL string) ([]byte, int, error) {
	return HTTPGet(rawURL)
}

// UnpackPlan is what would be written from a zip into dest.
type UnpackPlan struct {
	Files     []string // relative paths after flattening
	Conflicts []string // relative paths that already exist in dest
}

// PlanUnpack lists files and conflicts without writing.
func PlanUnpack(zipData []byte, dest string) (UnpackPlan, error) {
	entries, err := zipEntries(zipData)
	if err != nil {
		return UnpackPlan{}, err
	}
	var plan UnpackPlan
	for _, e := range entries {
		plan.Files = append(plan.Files, e.rel)
		target := filepath.Join(dest, filepath.FromSlash(e.rel))
		if st, err := os.Stat(target); err == nil && !st.IsDir() {
			plan.Conflicts = append(plan.Conflicts, e.rel)
		}
	}
	return plan, nil
}

type zipFile struct {
	rel  string
	mode os.FileMode
	data []byte
}

func zipEntries(zipData []byte) ([]zipFile, error) {
	if len(zipData) == 0 {
		return nil, fail("template zip is empty")
	}
	r, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, fail("template is not a zip archive")
	}
	prefix := commonRootPrefix(r.File)
	var out []zipFile
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		orig := strings.ReplaceAll(f.Name, "\\", "/")
		if err := safeRel(orig); err != nil {
			return nil, err
		}
		name := orig
		if prefix != "" {
			name = strings.TrimPrefix(name, prefix)
		}
		name = strings.TrimPrefix(name, "/")
		if name == "" || shouldSkipZipName(name) {
			continue
		}
		if err := safeRel(name); err != nil {
			return nil, err
		}
		if f.Mode()&os.ModeSymlink != 0 {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, wrap(exitcode.Failure, "read zip entry "+name, err)
		}
		data, err := io.ReadAll(io.LimitReader(rc, maxZipBytes+1))
		rc.Close()
		if err != nil {
			return nil, wrap(exitcode.Failure, "read zip entry "+name, err)
		}
		if int64(len(data)) > maxZipBytes {
			return nil, fail("zip entry " + name + " is too large")
		}
		mode := os.FileMode(0o644)
		if f.Mode()&0o111 != 0 {
			mode = 0o755
		}
		out = append(out, zipFile{rel: name, mode: mode, data: data})
	}
	if len(out) == 0 {
		return nil, fail("template zip has no files")
	}
	return out, nil
}

func shouldSkipZipName(name string) bool {
	if strings.HasPrefix(name, "__MACOSX/") || name == ".DS_Store" || strings.HasSuffix(name, "/.DS_Store") {
		return true
	}
	if strings.HasPrefix(name, ".git/") || strings.Contains(name, "/.git/") {
		return true
	}
	return false
}

func safeRel(name string) error {
	if path.IsAbs(name) || filepath.IsAbs(name) {
		return fail("refusing zip path " + name)
	}
	clean := path.Clean(name)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return fail("refusing zip path " + name)
	}
	for _, part := range strings.Split(clean, "/") {
		if part == ".." {
			return fail("refusing zip path " + name)
		}
	}
	return nil
}

func commonRootPrefix(files []*zip.File) string {
	var root string
	for _, f := range files {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		name = strings.TrimPrefix(name, "/")
		if name == "" || strings.HasPrefix(name, "__MACOSX/") {
			continue
		}
		first, _, _ := strings.Cut(name, "/")
		if first == "" || first == ".." {
			return ""
		}
		if root == "" {
			root = first
			continue
		}
		if first != root {
			return ""
		}
	}
	if root == "" {
		return ""
	}
	// Only flatten when every path is under that single directory.
	return root + "/"
}

// UnpackResult is what Unpack wrote.
type UnpackResult struct {
	Wrote   []string
	Skipped []string
}

// Unpack extracts zipData into dest. Existing files are skipped unless overwrite.
func Unpack(zipData []byte, dest string, overwrite bool) (UnpackResult, error) {
	entries, err := zipEntries(zipData)
	if err != nil {
		return UnpackResult{}, err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return UnpackResult{}, wrap(exitcode.Failure, "create project directory", err)
	}
	var result UnpackResult
	for _, e := range entries {
		target := filepath.Join(dest, filepath.FromSlash(e.rel))
		if err := safeJoin(dest, target); err != nil {
			return result, err
		}
		if st, err := os.Stat(target); err == nil && !st.IsDir() && !overwrite {
			result.Skipped = append(result.Skipped, e.rel)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return result, wrap(exitcode.Failure, "create "+filepath.Dir(target), err)
		}
		if err := os.WriteFile(target, e.data, e.mode); err != nil {
			return result, wrap(exitcode.Failure, "write "+e.rel, err)
		}
		result.Wrote = append(result.Wrote, e.rel)
	}
	return result, nil
}

func safeJoin(dest, target string) error {
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return fail("resolve destination")
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return fail("resolve zip target")
	}
	sep := string(os.PathSeparator)
	if absTarget != absDest && !strings.HasPrefix(absTarget, absDest+sep) {
		return fail("refusing zip path outside destination")
	}
	return nil
}
