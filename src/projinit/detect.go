package projinit

import (
	"os"
	"path/filepath"

	"github.com/polyapi/polyglot/src/delegate"
)

// Destination is where init writes files.
type Destination struct {
	Root     string
	Detected bool // true when a language project or .poly config was found
	Existing bool // dest already has files besides . / ..
	First    bool // no .poly/config.toml yet
	Langs    []delegate.Language
}

// ResolveDest picks cwd, or the nearest project root when a manifest exists.
//
// A package.json or Python requirements/pyproject file above cwd is treated as
// the project root so a template unpacks next to the existing app, not in a
// nested subdirectory.
func ResolveDest(cwd, polyPath string, forceCWD bool) (Destination, error) {
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return Destination{}, fail("cannot resolve working directory")
		}
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return Destination{}, fail("cannot resolve working directory")
	}
	dest := Destination{Root: abs, First: true}
	if forceCWD {
		fillDest(&dest, abs, polyPath)
		return dest, nil
	}
	loc := delegate.LocateProject(abs, polyPath)
	root := abs
	if loc.Found {
		root = loc.Root
		dest.Detected = true
		dest.Langs = loc.Languages
	}
	fillDest(&dest, root, polyPath)
	return dest, nil
}

func fillDest(d *Destination, root, polyPath string) {
	d.Root = root
	if polyPath == "" {
		polyPath = ".poly"
	}
	cfg := filepath.Join(root, polyPath, "config.toml")
	if st, err := os.Stat(cfg); err == nil && !st.IsDir() {
		d.First = false
	}
	if len(d.Langs) == 0 {
		d.Langs = delegate.DetectLanguages(root)
	}
	if len(d.Langs) > 0 {
		d.Detected = true
	}
	d.Existing = dirHasEntries(root)
}

func dirHasEntries(root string) bool {
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	for _, e := range entries {
		name := e.Name()
		if name == "." || name == ".." {
			continue
		}
		return true
	}
	return false
}

// HasPolyConfig is true when projectRoot already has a config.toml under polyPath.
func HasPolyConfig(projectRoot, polyPath string) bool {
	if polyPath == "" {
		polyPath = ".poly"
	}
	st, err := os.Stat(filepath.Join(projectRoot, polyPath, "config.toml"))
	return err == nil && !st.IsDir()
}
