package projinit

import (
	"net/url"
	"path"
	"strings"
)

// Kind is how a template is obtained.
type Kind int

const (
	KindNone Kind = iota
	KindZip
	KindGitHub
)

// Source is a resolved template location (empty project, zip URL, or GitHub repo).
type Source struct {
	Kind   Kind
	Label  string
	URL    string // direct zip / file URL when KindZip
	GitHub string // owner/repo when KindGitHub
	Ref    string // branch, tag, or SHA (GitHub); default main
}

// None is an empty-project source (no template unpack).
func None() Source {
	return Source{Kind: KindNone, Label: "empty project"}
}

// ParseSource accepts --template values: none, a zip URL/path, or a GitHub repo.
func ParseSource(raw string) (Source, error) {
	s := strings.TrimSpace(raw)
	if s == "" || isNoneToken(s) {
		return None(), nil
	}
	if src, ok := officialSource(s); ok {
		return src, nil
	}
	if looksLikeZipFile(s) {
		return Source{Kind: KindZip, Label: s, URL: s}, nil
	}
	if gh, ok := ParseGitHub(s); ok {
		return gh, nil
	}
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return Source{Kind: KindZip, Label: s, URL: s}, nil
	}
	if strings.Contains(s, "/") && !strings.Contains(s, "://") {
		if gh, ok := ParseGitHub(s); ok {
			return gh, nil
		}
	}
	return Source{}, usage("template must be none, a zip URL or path, or a GitHub repository (owner/repo or https://github.com/owner/repo)")
}

func isNoneToken(s string) bool {
	switch strings.ToLower(s) {
	case "none", "empty", "-", "no":
		return true
	default:
		return false
	}
}

func looksLikeZipFile(s string) bool {
	lower := strings.ToLower(s)
	if strings.HasSuffix(lower, ".zip") {
		return true
	}
	if strings.HasPrefix(s, "file:") {
		return true
	}
	// Local path without a scheme.
	if !strings.Contains(s, "://") && (strings.HasPrefix(s, ".") || strings.HasPrefix(s, "/") || strings.Contains(s, `\`) || strings.Contains(s, ":\\")) {
		return true
	}
	return false
}

// ParseGitHub turns owner/repo, github.com URLs, and git@ remotes into a Source.
func ParseGitHub(raw string) (Source, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return Source{}, false
	}
	ref := ""
	if i := strings.LastIndex(s, "@"); i > 0 && !strings.Contains(s[i:], "/") {
		// owner/repo@ref  (not git@github.com:…)
		if !strings.HasPrefix(s, "git@") {
			ref = s[i+1:]
			s = s[:i]
		}
	}
	if i := strings.Index(s, "#"); i >= 0 {
		ref = s[i+1:]
		s = s[:i]
	}
	s = strings.TrimSuffix(s, ".git")

	owner, repo, extraRef, ok := splitGitHub(s)
	if !ok {
		return Source{}, false
	}
	if extraRef != "" && ref == "" {
		ref = extraRef
	}
	if ref == "" {
		ref = "main"
	}
	return Source{
		Kind:   KindGitHub,
		Label:  owner + "/" + repo,
		GitHub: owner + "/" + repo,
		Ref:    ref,
	}, true
}

func splitGitHub(s string) (owner, repo, ref string, ok bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "/")
	if strings.HasPrefix(s, "git@github.com:") {
		rest := strings.TrimPrefix(s, "git@github.com:")
		return splitOwnerRepo(rest)
	}
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		u, err := url.Parse(s)
		if err != nil {
			return "", "", "", false
		}
		host := strings.ToLower(u.Hostname())
		if host != "github.com" && host != "www.github.com" {
			return "", "", "", false
		}
		parts := splitPath(u.Path)
		if len(parts) < 2 {
			return "", "", "", false
		}
		owner, repo = parts[0], parts[1]
		repo = strings.TrimSuffix(repo, ".git")
		if len(parts) >= 4 && (parts[2] == "tree" || parts[2] == "archive" || parts[2] == "blob") {
			ref = strings.Join(parts[3:], "/")
			if strings.HasSuffix(ref, ".zip") {
				ref = strings.TrimSuffix(ref, ".zip")
			}
			if strings.HasPrefix(ref, "refs/heads/") {
				ref = strings.TrimPrefix(ref, "refs/heads/")
			}
			if strings.HasPrefix(ref, "refs/tags/") {
				ref = strings.TrimPrefix(ref, "refs/tags/")
			}
		}
		return owner, repo, ref, owner != "" && repo != ""
	}
	s = strings.TrimPrefix(s, "github.com/")
	s = strings.TrimPrefix(s, "www.github.com/")
	return splitOwnerRepo(s)
}

func splitOwnerRepo(s string) (owner, repo, ref string, ok bool) {
	s = strings.Trim(s, "/")
	parts := splitPath(s)
	if len(parts) < 2 {
		return "", "", "", false
	}
	owner, repo = parts[0], strings.TrimSuffix(parts[1], ".git")
	if owner == "" || repo == "" || owner == "." || repo == "." {
		return "", "", "", false
	}
	if strings.ContainsAny(owner, " :") || strings.ContainsAny(repo, " :") {
		return "", "", "", false
	}
	return owner, repo, "", true
}

func splitPath(p string) []string {
	p = path.Clean("/" + strings.ReplaceAll(p, "\\", "/"))
	var out []string
	for _, part := range strings.Split(p, "/") {
		if part == "" || part == "." {
			continue
		}
		out = append(out, part)
	}
	return out
}

// ZipURL is the download URL for this source. GitHub uses the public archive zip.
func (s Source) ZipURL() string {
	switch s.Kind {
	case KindZip:
		return s.URL
	case KindGitHub:
		ref := s.Ref
		if ref == "" {
			ref = "main"
		}
		return GitHubZipURL(s.GitHub, ref)
	default:
		return ""
	}
}

// GitHubZipURL is the public archive zip for owner/repo at ref.
func GitHubZipURL(ownerRepo, ref string) string {
	ownerRepo = strings.Trim(ownerRepo, "/")
	if ref == "" {
		ref = "main"
	}
	// archive/<ref>.zip works for branches, tags, and full SHAs.
	return "https://github.com/" + ownerRepo + "/archive/" + ref + ".zip"
}

// FallbackZipURL tries the heads/ form when archive/<ref>.zip 404s.
func FallbackZipURL(ownerRepo, ref string) string {
	if ref == "" {
		ref = "main"
	}
	return "https://github.com/" + ownerRepo + "/archive/refs/heads/" + ref + ".zip"
}

func officialSource(s string) (Source, bool) {
	key := strings.ToLower(strings.TrimSpace(s))
	key = strings.TrimPrefix(key, "polyapi/")
	switch key {
	case "poly-glide-template-js", "glide-js", "glide-ts", "js", "ts", "typescript":
		return OfficialTypeScript(), true
	case "poly-glide-template-py", "glide-py", "python", "py":
		return OfficialPython(), true
	default:
		return Source{}, false
	}
}

// OfficialTypeScript is polyapi/poly-glide-template-js on main.
func OfficialTypeScript() Source {
	return Source{
		Kind:   KindGitHub,
		Label:  "Glide TypeScript (polyapi/poly-glide-template-js)",
		GitHub: "polyapi/poly-glide-template-js",
		Ref:    "main",
	}
}

// OfficialPython is polyapi/poly-glide-template-py on main.
func OfficialPython() Source {
	return Source{
		Kind:   KindGitHub,
		Label:  "Glide Python (polyapi/poly-glide-template-py)",
		GitHub: "polyapi/poly-glide-template-py",
		Ref:    "main",
	}
}

// OfficialFor returns the Glide template for typescript or python.
func OfficialFor(lang string) Source {
	if strings.EqualFold(lang, "python") {
		return OfficialPython()
	}
	return OfficialTypeScript()
}
