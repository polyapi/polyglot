package projinit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/polyapi/polyglot/src/exitcode"
)

// EnsureManifest writes a minimal package.json or requirements.txt when missing.
func EnsureManifest(root, lang, name string) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return wrap(exitcode.Failure, "create project directory", err)
	}
	if strings.EqualFold(lang, "python") {
		return ensurePythonManifest(root, name)
	}
	return ensureTSManifest(root, name)
}

func ensureTSManifest(root, name string) error {
	pkgPath := filepath.Join(root, "package.json")
	if !fileExists(pkgPath) {
		if name == "" {
			name = filepath.Base(root)
		}
		name = npmName(name)
		pkg := map[string]any{
			"name":    name,
			"version": "0.0.1",
			"private": true,
			"scripts": map[string]any{
				"test": "echo \"Error: no test specified\" && exit 2",
			},
			"dependencies": map[string]any{
				"polyapi":    "latest",
				"ts-node":    "^10.9.2",
				"typescript": "^5.5.4",
			},
		}
		raw, err := json.MarshalIndent(pkg, "", "  ")
		if err != nil {
			return wrap(exitcode.Failure, "serialize package.json", err)
		}
		raw = append(raw, '\n')
		if err := os.WriteFile(pkgPath, raw, 0o644); err != nil {
			return wrap(exitcode.Failure, "write package.json", err)
		}
	} else {
		if name != "" {
			if err := SetJSONName(pkgPath, npmName(name)); err != nil {
				return err
			}
		}
		if err := ensureNPMDep(pkgPath, npmPolyPackage); err != nil {
			return err
		}
	}
	tsconfig := filepath.Join(root, "tsconfig.json")
	if !fileExists(tsconfig) {
		raw := []byte("{\n  \"compilerOptions\": {\n    \"esModuleInterop\": true\n  }\n}\n")
		if err := os.WriteFile(tsconfig, raw, 0o644); err != nil {
			return wrap(exitcode.Failure, "write tsconfig.json", err)
		}
	}
	return nil
}

func ensurePythonManifest(root, name string) error {
	req := filepath.Join(root, "requirements.txt")
	if !fileExists(req) && !fileExists(filepath.Join(root, "pyproject.toml")) {
		if err := os.WriteFile(req, []byte(pipPolyPackage+"\n"), 0o644); err != nil {
			return wrap(exitcode.Failure, "write requirements.txt", err)
		}
	} else if fileExists(req) {
		if err := ensureRequirementsLine(req, pipPolyPackage); err != nil {
			return err
		}
	}
	pyproject := filepath.Join(root, "pyproject.toml")
	if fileExists(pyproject) && name != "" {
		_ = SetTOMLName(pyproject, npmName(name))
	}
	return nil
}

// SetJSONName replaces the top-level "name" in a JSON object file.
func SetJSONName(path, name string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return wrap(exitcode.Failure, "read "+filepath.Base(path), err)
	}
	var pkg map[string]any
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return fail(filepath.Base(path) + " is not valid JSON")
	}
	pkg["name"] = name
	out, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return wrap(exitcode.Failure, "serialize "+filepath.Base(path), err)
	}
	out = append(out, '\n')
	return os.WriteFile(path, out, 0o644)
}

var pyprojectName = regexp.MustCompile(`(?m)^name\s*=\s*"[^"]*"`)

// SetTOMLName replaces the first name = "…" in a TOML file (pyproject).
func SetTOMLName(path, name string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return wrap(exitcode.Failure, "read "+filepath.Base(path), err)
	}
	if !pyprojectName.Match(raw) {
		return nil
	}
	repl := []byte("name = \"" + name + "\"")
	out := pyprojectName.ReplaceAll(raw, repl)
	return os.WriteFile(path, out, 0o644)
}

func npmName(name string) string {
	s := strings.TrimSpace(strings.ToLower(name))
	s = strings.ReplaceAll(s, " ", "-")
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		}
	}
	out := strings.Trim(b.String(), "-._")
	if out == "" {
		return "poly-project"
	}
	return out
}
