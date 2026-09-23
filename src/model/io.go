package model

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/polyapi/polyglot/src/version"
)

var (
	// ErrNotExist is a missing local OpenAPI or spec-input file.
	ErrNotExist = errors.New("file does not exist")
	// ErrDestinationExists is generate with an explicit path that already exists.
	ErrDestinationExists = errors.New("destination file already exists")
)

var fetchHTTP = &http.Client{Timeout: 30 * time.Second}

// IsRemotePath is true for http(s) URLs (OpenAPI fetched rather than read from disk).
func IsRemotePath(specPath string) bool {
	u, err := url.Parse(specPath)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

func readSpecSource(specPath string) (string, error) {
	if IsRemotePath(specPath) {
		return fetchURL(specPath)
	}
	raw, err := os.ReadFile(specPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotExist
		}
		return "", err
	}
	return string(raw), nil
}

func fetchURL(rawURL string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to fetch contents from url %q", rawURL)
	}
	req.Header.Set("Accept", "application/json, application/yaml, text/yaml, text/plain, */*")
	req.Header.Set("User-Agent", "polyapi/"+version.Version)
	resp, err := fetchHTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch contents from url %q", rawURL)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to fetch contents from url %q", rawURL)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("failed to fetch contents from url %q", rawURL)
	}
	return string(body), nil
}

func outputBaseDir(sourcePath string) string {
	if sourcePath == "" || IsRemotePath(sourcePath) {
		return "."
	}
	return filepath.Dir(filepath.Clean(sourcePath))
}

func resolveOutputPath(sourcePath, destination string) (string, error) {
	base := outputBaseDir(sourcePath)
	if destination != "" {
		path := destination
		if !filepath.IsAbs(path) {
			path = filepath.Join(base, destination)
		}
		if _, err := os.Stat(path); err == nil {
			return "", ErrDestinationExists
		} else if !os.IsNotExist(err) {
			return "", err
		}
		return path, nil
	}
	return "", fmt.Errorf("internal: destination required")
}

func uniqueOutputPath(dir, title string) (string, error) {
	slug := Slugify(title)
	if slug == "" {
		slug = "specification"
	}
	single := filepath.Join(dir, slug+".json")
	foundSingle := false
	if _, err := os.Stat(single); err == nil {
		foundSingle = true
	} else if !os.IsNotExist(err) {
		return "", err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return single, nil
		}
		return "", err
	}
	re := regexp.MustCompile(regexp.QuoteMeta(slug) + `-([0-9]+)`)
	lastCount := 0
	for _, entry := range entries {
		m := re.FindStringSubmatch(entry.Name())
		if len(m) < 2 {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if n > lastCount {
			lastCount = n
		}
	}
	count := lastCount + 1
	if lastCount == 0 && !foundSingle {
		count = 0
	}
	if count == 0 {
		return single, nil
	}
	return filepath.Join(dir, fmt.Sprintf("%s-%d.json", slug, count)), nil
}

func writeFile(path, contents string) error {
	return os.WriteFile(path, []byte(contents), 0o644)
}

// ValidHostURL is true for an absolute http(s) URL with a host.
func ValidHostURL(raw string) bool {
	return isValidHTTPURL(raw)
}

func isValidHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return u.Host != ""
}
