package config

import (
	"os"
	"path/filepath"
	"strings"
)

// CurrentBranch walks start and its parents for a git checkout and reads HEAD.
//
// The second return is false when no repository is found. Detached HEAD is
// reported as "detached@<shortsha>" so deploy matching still has a name.
func CurrentBranch(start string) (string, bool) {
	gitDir, ok := findGitDir(start)
	if !ok {
		return "", false
	}
	raw, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return "", false
	}
	line := strings.TrimSpace(string(raw))
	const heads = "ref: refs/heads/"
	if strings.HasPrefix(line, heads) {
		name := strings.TrimPrefix(line, heads)
		if name != "" {
			return name, true
		}
	}
	if strings.HasPrefix(line, "ref: ") {
		ref := strings.TrimSpace(strings.TrimPrefix(line, "ref: "))
		if ref != "" {
			return ref, true
		}
	}
	if line == "" {
		return "", false
	}
	short := line
	if len(short) > 7 {
		short = short[:7]
	}
	return "detached@" + short, true
}

func findGitDir(start string) (string, bool) {
	dir, err := filepath.Abs(start)
	if err != nil {
		dir = start
	}
	for {
		p := filepath.Join(dir, ".git")
		st, err := os.Stat(p)
		if err == nil {
			if st.IsDir() {
				return p, true
			}
			if gitDir, ok := parseGitFile(p, dir); ok {
				return gitDir, true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func parseGitFile(path, dir string) (string, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if !strings.HasPrefix(lower, "gitdir:") {
			continue
		}
		gitdir := strings.TrimSpace(trimmed[len("gitdir:"):])
		if gitdir == "" {
			continue
		}
		if !filepath.IsAbs(gitdir) {
			gitdir = filepath.Join(dir, gitdir)
		}
		st, err := os.Stat(gitdir)
		if err == nil && st.IsDir() {
			return gitdir, true
		}
	}
	return "", false
}
