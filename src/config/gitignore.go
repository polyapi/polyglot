package config

import (
	"os"
	"path/filepath"
	"strings"
)

const ignoreLine = ".poly/"

// GitignoreAction is what EnsurePolyGitignored did.
type GitignoreAction int

const (
	GitignoreCreated GitignoreAction = iota
	GitignoreAppended
	GitignoreAlreadyPresent
	GitignoreSkippedNotGit
)

// EnsurePolyGitignored idempotently lists .poly/ in .gitignore when projectRoot is a git repo.
func EnsurePolyGitignored(projectRoot string) (GitignoreAction, error) {
	if _, err := os.Stat(filepath.Join(projectRoot, ".git")); err != nil {
		return GitignoreSkippedNotGit, nil
	}
	gitignore := filepath.Join(projectRoot, ".gitignore")
	if _, err := os.Stat(gitignore); err == nil {
		contents, err := os.ReadFile(gitignore)
		if err != nil {
			return 0, ioError(gitignore, err)
		}
		text := string(contents)
		if ignoresPoly(text) {
			return GitignoreAlreadyPresent, nil
		}
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += ignoreLine + "\n"
		if err := os.WriteFile(gitignore, []byte(text), 0o644); err != nil {
			return 0, ioError(gitignore, err)
		}
		return GitignoreAppended, nil
	}
	if err := os.WriteFile(gitignore, []byte(ignoreLine+"\n"), 0o644); err != nil {
		return 0, ioError(gitignore, err)
	}
	return GitignoreCreated, nil
}

func ignoresPoly(contents string) bool {
	for _, line := range strings.Split(contents, "\n") {
		trimmed := strings.TrimSpace(line)
		switch trimmed {
		case ".poly/", ".poly", "/.poly/", "/.poly", "**/.poly/", "**/.poly":
			return true
		}
	}
	return false
}
