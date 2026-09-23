package projinit

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/polyapi/polyglot/src/delegate"
	"github.com/polyapi/polyglot/src/exitcode"
)

const (
	minNodeMajor = 20
	minPyMajor   = 3
	minPyMinor   = 10
)

// NodeDownloadURL is the public Node.js install page.
const NodeDownloadURL = "https://nodejs.org/en/download"

// NodePackageManagerURL is distro-specific Node install docs.
const NodePackageManagerURL = "https://nodejs.org/en/download/package-manager"

// PythonDownloadURL is the public Python install page.
const PythonDownloadURL = "https://www.python.org/downloads/"

// PythonVenvDocs is the stdlib venv documentation.
const PythonVenvDocs = "https://docs.python.org/3/library/venv.html"

// PythonVenvPrimer is the primer the Python SDK README recommends.
const PythonVenvPrimer = "https://realpython.com/python-virtual-environments-a-primer/"

// LookPath locates an executable. Tests may override.
var LookPath = exec.LookPath

// RunCmd runs a command. Tests may override.
var RunCmd = func(cmd *exec.Cmd) error { return cmd.Run() }

// RuntimeKind is node or python.
type RuntimeKind string

const (
	RuntimeNode   RuntimeKind = "node"
	RuntimePython RuntimeKind = "python"
)

// Runtime is a detected interpreter.
type Runtime struct {
	Kind    RuntimeKind
	Path    string
	Version string
	OK      bool
}

// DetectRuntime finds node (TypeScript) or python3 (Python) on PATH.
func DetectRuntime(lang string) Runtime {
	if strings.EqualFold(lang, "python") {
		return detectPython()
	}
	return detectNode()
}

func detectNode() Runtime {
	rt := Runtime{Kind: RuntimeNode}
	p := findBinary("node")
	if p == "" {
		return rt
	}
	rt.Path = p
	rt.Version = commandVersion(p, "--version")
	rt.OK = nodeVersionOK(rt.Version)
	return rt
}

func detectPython() Runtime {
	rt := Runtime{Kind: RuntimePython}
	name, prefix := pythonLauncher()
	if name == "" {
		return rt
	}
	rt.Path = name
	args := append(append([]string{}, prefix...), "--version")
	rt.Version = commandVersionArgs(name, args...)
	rt.OK = pythonVersionOK(rt.Version)
	return rt
}

func pythonLauncher() (string, []string) {
	if p := findBinary("python3"); p != "" {
		return p, nil
	}
	if p := findBinary("python"); p != "" {
		return p, nil
	}
	if runtime.GOOS == "windows" {
		if p := findBinary("py"); p != "" {
			return p, []string{"-3"}
		}
	}
	return "", nil
}

// ExtraBinDirs are extra locations searched after PATH. Tests may override.
var ExtraBinDirs = defaultExtraBinDirs

func defaultExtraBinDirs(name string) []string {
	if runtime.GOOS == "windows" {
		return windowsExtraDirs(name)
	}
	return []string{"/opt/homebrew/bin", "/usr/local/bin", "/usr/bin"}
}

func findBinary(name string) string {
	if p, err := LookPath(name); err == nil && p != "" {
		return p
	}
	dirs := ExtraBinDirs(name)
	for _, dir := range dirs {
		for _, cand := range []string{name, name + ".exe"} {
			p := filepath.Join(dir, cand)
			if fileExists(p) {
				return p
			}
		}
	}
	return ""
}

func windowsExtraDirs(name string) []string {
	var dirs []string
	if pf := os.Getenv("ProgramFiles"); pf != "" {
		if name == "node" {
			dirs = append(dirs, filepath.Join(pf, "nodejs"))
		}
		if name == "python" || name == "python3" {
			dirs = append(dirs, filepath.Join(pf, "Python312"), filepath.Join(pf, "Python311"))
		}
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		dirs = append(dirs, filepath.Join(local, "Programs", "Python", "Python312"))
		dirs = append(dirs, filepath.Join(local, "Programs", "Python", "Python311"))
		dirs = append(dirs, filepath.Join(local, "Programs", "nodejs"))
	}
	return dirs
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func commandVersion(bin string, args ...string) string {
	return commandVersionArgs(bin, args...)
}

func commandVersionArgs(bin string, args ...string) string {
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func nodeVersionOK(v string) bool {
	maj, _, _, ok := parseVersion(v)
	return ok && maj >= minNodeMajor
}

func pythonVersionOK(v string) bool {
	maj, min, _, ok := parseVersion(v)
	if !ok {
		return false
	}
	if maj > minPyMajor {
		return true
	}
	return maj == minPyMajor && min >= minPyMinor
}

func parseVersion(v string) (major, minor, patch int, ok bool) {
	s := strings.TrimSpace(v)
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "Python ")
	s = strings.TrimPrefix(s, "python ")
	if i := strings.IndexAny(s, " \t\n"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) == 0 || parts[0] == "" {
		return 0, 0, 0, false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, false
	}
	if len(parts) > 1 {
		minor, _ = strconv.Atoi(parts[1])
	}
	if len(parts) > 2 {
		patch, _ = strconv.Atoi(strings.TrimRight(parts[2], "abrcdev"))
	}
	return major, minor, patch, true
}

// MissingRuntimeMessage explains how to install a missing interpreter and re-run init.
func MissingRuntimeMessage(lang string) string {
	if strings.EqualFold(lang, "python") {
		return strings.Join([]string{
			"Python 3.10 or later is required to initialize a Python Poly project.",
			"",
			"Install Python, then re-run `polyapi init`:",
			"  " + PythonDownloadURL,
			"  venv docs: " + PythonVenvDocs,
			"  primer:    " + PythonVenvPrimer,
			"",
			"macOS (Homebrew):  brew install python3",
			"Windows (winget):  winget install Python.Python.3.12",
			"Debian/Ubuntu:     sudo apt-get install python3 python3-venv python3-pip",
		}, "\n")
	}
	return strings.Join([]string{
		"Node.js 20 or later is required to initialize a TypeScript Poly project.",
		"",
		"Install Node.js, then re-run `polyapi init`:",
		"  " + NodeDownloadURL,
		"  package managers: " + NodePackageManagerURL,
		"",
		"macOS (Homebrew):  brew install node",
		"Windows (winget):  winget install OpenJS.NodeJS.LTS",
		"Debian/Ubuntu:     see " + NodePackageManagerURL,
	}, "\n")
}

// Installer is a package-manager command that can install a runtime.
type Installer struct {
	Name string
	Argv []string
}

// FindInstaller locates brew / winget / apt-get / dnf / pacman for kind.
func FindInstaller(kind RuntimeKind) (Installer, bool) {
	switch runtime.GOOS {
	case "darwin":
		if brew := findBinary("brew"); brew != "" {
			pkg := "node"
			if kind == RuntimePython {
				pkg = "python3"
			}
			return Installer{Name: "Homebrew", Argv: []string{brew, "install", pkg}}, true
		}
	case "windows":
		if winget := findBinary("winget"); winget != "" {
			id := "OpenJS.NodeJS.LTS"
			if kind == RuntimePython {
				id = "Python.Python.3.12"
			}
			return Installer{Name: "winget", Argv: []string{
				winget, "install", "-e", "--id", id,
				"--accept-package-agreements", "--accept-source-agreements",
			}}, true
		}
	default:
		if brew := findBinary("brew"); brew != "" {
			pkg := "node"
			if kind == RuntimePython {
				pkg = "python3"
			}
			return Installer{Name: "Homebrew", Argv: []string{brew, "install", pkg}}, true
		}
		if apt := findBinary("apt-get"); apt != "" {
			if kind == RuntimePython {
				return Installer{Name: "apt", Argv: sudoPrefix(apt, "install", "-y", "python3", "python3-venv", "python3-pip")}, true
			}
			return Installer{Name: "apt", Argv: sudoPrefix(apt, "install", "-y", "nodejs", "npm")}, true
		}
		if dnf := findBinary("dnf"); dnf != "" {
			pkg := "nodejs"
			if kind == RuntimePython {
				pkg = "python3"
			}
			return Installer{Name: "dnf", Argv: sudoPrefix(dnf, "install", "-y", pkg)}, true
		}
		if pacman := findBinary("pacman"); pacman != "" {
			pkg := "nodejs"
			if kind == RuntimePython {
				pkg = "python"
			}
			return Installer{Name: "pacman", Argv: sudoPrefix(pacman, "-S", "--noconfirm", pkg)}, true
		}
	}
	return Installer{}, false
}

func sudoPrefix(bin string, args ...string) []string {
	if os.Geteuid() == 0 {
		return append([]string{bin}, args...)
	}
	if sudo := findBinary("sudo"); sudo != "" {
		return append([]string{sudo, bin}, args...)
	}
	return append([]string{bin}, args...)
}

// InstallRuntime runs the package-manager installer and re-detects the runtime.
func InstallRuntime(kind RuntimeKind, stdout, stderr io.Writer) (Runtime, error) {
	inst, ok := FindInstaller(kind)
	if !ok {
		return Runtime{Kind: kind}, fail(MissingRuntimeMessage(kindLang(kind)))
	}
	if stdout != nil {
		fmt.Fprintf(stdout, "Installing %s with %s…\n", kind, inst.Name)
	}
	cmd := exec.Command(inst.Argv[0], inst.Argv[1:]...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := RunCmd(cmd); err != nil {
		return Runtime{Kind: kind}, wrap(exitcode.Failure, inst.Name+" failed", err)
	}
	prependInstallerBins()
	rt := DetectRuntime(kindLang(kind))
	if rt.Path == "" {
		return rt, fail(MissingRuntimeMessage(kindLang(kind)))
	}
	return rt, nil
}

func kindLang(k RuntimeKind) string {
	if k == RuntimePython {
		return "python"
	}
	return "typescript"
}

func prependInstallerBins() {
	var extra []string
	switch runtime.GOOS {
	case "darwin":
		extra = []string{"/opt/homebrew/bin", "/usr/local/bin"}
	case "windows":
		if pf := os.Getenv("ProgramFiles"); pf != "" {
			extra = append(extra, filepath.Join(pf, "nodejs"))
		}
	}
	if len(extra) == 0 {
		return
	}
	path := os.Getenv("PATH")
	os.Setenv("PATH", strings.Join(extra, string(os.PathListSeparator))+string(os.PathListSeparator)+path)
}

// EnsureRuntime detects the interpreter, optionally installing it.
// ask is called when the runtime is missing; nil means do not install.
func EnsureRuntime(lang string, ask func(kind RuntimeKind) (bool, error), stdout, stderr io.Writer) (Runtime, error) {
	rt := DetectRuntime(lang)
	if rt.Path != "" {
		if !rt.OK {
			return rt, &Error{
				Code: exitcode.Failure,
				Msg:  outdatedRuntimeMessage(lang, rt.Version),
			}
		}
		return rt, nil
	}
	kind := RuntimeNode
	if strings.EqualFold(lang, "python") {
		kind = RuntimePython
	}
	if ask == nil {
		return rt, fail(MissingRuntimeMessage(lang))
	}
	ok, err := ask(kind)
	if err != nil {
		return rt, err
	}
	if !ok {
		return rt, fail(MissingRuntimeMessage(lang))
	}
	return InstallRuntime(kind, stdout, stderr)
}

func outdatedRuntimeMessage(lang, version string) string {
	if strings.EqualFold(lang, "python") {
		return fmt.Sprintf("Python %s is too old (need 3.10 or later).\n\n%s", strings.TrimSpace(version), MissingRuntimeMessage(lang))
	}
	return fmt.Sprintf("Node.js %s is too old (need 20 or later).\n\n%s", strings.TrimSpace(version), MissingRuntimeMessage(lang))
}

// LanguageFromRuntime maps a language string onto typescript or python.
func LanguageFromRuntime(s string) (string, error) {
	lang, ok := delegate.ParseLanguage(s)
	if !ok {
		return "", usage("language must be typescript or python")
	}
	switch lang {
	case delegate.LangTypeScript:
		return "typescript", nil
	case delegate.LangPython:
		return "python", nil
	case delegate.LangJava:
		return "", usage("Java project init is not implemented yet (POLY-CLI-13)")
	default:
		return "", usage("language must be typescript or python")
	}
}
