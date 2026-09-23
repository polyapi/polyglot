package delegate

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// LocatedProject is the nearest language (or .poly) project above a start directory.
type LocatedProject struct {
	Root      string
	Languages []Language
	Found     bool
}

// LocateProject walks start and its parents for a language project.
//
// A directory is a project if it has a language manifest (package.json,
// pyproject.toml, requirements.txt, pom.xml, …) or a project config file
// under polyPath. The walk stops at the first match: a Node or Python app
// without the PolyAPI SDK is still that project — missing SDK is a later
// error, not a reason to keep walking.
func LocateProject(start, polyPath string) LocatedProject {
	if start == "" {
		start = "."
	}
	dir, err := filepath.Abs(start)
	if err != nil {
		dir = start
	}
	if polyPath == "" {
		polyPath = ".poly"
	}
	for {
		langs := DetectLanguages(dir)
		if len(langs) > 0 || polyConfigExists(dir, polyPath) {
			return LocatedProject{Root: dir, Languages: langs, Found: true}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return LocatedProject{}
		}
		dir = parent
	}
}

func polyConfigExists(root, polyPath string) bool {
	var configPath string
	if filepath.IsAbs(polyPath) {
		configPath = filepath.Join(polyPath, "config.toml")
	} else {
		configPath = filepath.Join(root, polyPath, "config.toml")
	}
	st, err := os.Stat(configPath)
	return err == nil && !st.IsDir()
}

// DetectLanguages infers candidate languages from project manifests.
// It does not require the PolyAPI SDK to be installed; that is a separate check.
func DetectLanguages(projectRoot string) []Language {
	var found []Language
	if isTypeScriptProject(projectRoot) {
		found = append(found, LangTypeScript)
	}
	if isPythonProject(projectRoot) {
		found = append(found, LangPython)
	}
	if isJavaProject(projectRoot) {
		found = append(found, LangJava)
	}
	return found
}

func isTypeScriptProject(root string) bool {
	for _, name := range []string{
		"package.json",
		"package-lock.json",
		"yarn.lock",
		"pnpm-lock.yaml",
		"bun.lock",
		"bun.lockb",
	} {
		if fileExists(filepath.Join(root, name)) {
			return true
		}
	}
	return false
}

func isPythonProject(root string) bool {
	for _, name := range []string{
		"pyproject.toml",
		"setup.py",
		"setup.cfg",
		"Pipfile",
		"poetry.lock",
		"uv.lock",
	} {
		if fileExists(filepath.Join(root, name)) {
			return true
		}
	}
	return hasRequirementsFile(root)
}

func isJavaProject(root string) bool {
	for _, name := range []string{"pom.xml", "build.gradle", "build.gradle.kts"} {
		if fileExists(filepath.Join(root, name)) {
			return true
		}
	}
	return false
}

func hasRequirementsFile(root string) bool {
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "requirements.txt" || (strings.HasPrefix(name, "requirements") && strings.HasSuffix(name, ".txt")) {
			return true
		}
	}
	return false
}

func typescriptAdapter(root string) (argv []string, installed bool) {
	mod := filepath.Join(root, "node_modules", "polyapi")
	st, err := os.Stat(mod)
	if err != nil || !st.IsDir() {
		return nil, false
	}
	js := filepath.Join(mod, "build", "adapter.js")
	if fileExists(js) {
		return []string{"node", js}, true
	}
	bin := filepath.Join(root, "node_modules", ".bin", "poly")
	if runtime.GOOS == "windows" {
		if fileExists(bin + ".cmd") {
			bin += ".cmd"
		}
	}
	if fileExists(bin) {
		return []string{bin, "adapter"}, true
	}
	return nil, true
}

func pythonAdapter(root string) (argv []string, installed bool) {
	if py, ok := venvPythonWithPolyapi(root); ok {
		return []string{py, "-m", "polyapi", "adapter"}, true
	}
	if localPolyapiPackage(root) {
		return []string{PythonLauncher(), "-m", "polyapi", "adapter"}, true
	}
	return nil, false
}

func localPolyapiPackage(root string) bool {
	for _, rel := range []string{"polyapi", filepath.Join("src", "polyapi")} {
		dir := filepath.Join(root, rel)
		st, err := os.Stat(dir)
		if err != nil || !st.IsDir() {
			continue
		}
		if fileExists(filepath.Join(dir, "__init__.py")) || fileExists(filepath.Join(dir, "__main__.py")) {
			return true
		}
	}
	return false
}

func venvPythonWithPolyapi(root string) (string, bool) {
	for _, name := range []string{".venv", "venv", ".virtualenv"} {
		venv := filepath.Join(root, name)
		py := venvPython(venv)
		if py == "" {
			continue
		}
		if venvHasPolyapi(venv) {
			return py, true
		}
	}
	return "", false
}

func venvPython(venv string) string {
	for _, rel := range []string{
		filepath.Join("bin", "python3"),
		filepath.Join("bin", "python"),
		filepath.Join("Scripts", "python.exe"),
		filepath.Join("Scripts", "python3.exe"),
	} {
		p := filepath.Join(venv, rel)
		if fileExists(p) {
			return p
		}
	}
	return ""
}

func venvHasPolyapi(venv string) bool {
	candidates := []string{
		filepath.Join(venv, "Lib", "site-packages", "polyapi"),
		filepath.Join(venv, "lib", "site-packages", "polyapi"),
	}
	lib := filepath.Join(venv, "lib")
	entries, err := os.ReadDir(lib)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() && strings.HasPrefix(e.Name(), "python") {
				candidates = append(candidates, filepath.Join(lib, e.Name(), "site-packages", "polyapi"))
			}
		}
	}
	for _, c := range candidates {
		st, err := os.Stat(c)
		if err == nil && st.IsDir() {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
