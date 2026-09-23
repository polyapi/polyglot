package projinit

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/polyapi/polyglot/src/exitcode"
)

const defaultVenv = ".venv"

// VenvDir returns an existing virtualenv under root, or "".
func VenvDir(root string) string {
	for _, name := range []string{defaultVenv, "venv", ".virtualenv"} {
		dir := filepath.Join(root, name)
		if pythonInVenv(dir) != "" {
			return dir
		}
	}
	return ""
}

func pythonInVenv(venv string) string {
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

// EnsureVenv creates root/.venv when missing and returns the venv python.
func EnsureVenv(root, name string, stdout io.Writer) (string, error) {
	if name == "" {
		name = defaultVenv
	}
	dir := filepath.Join(root, name)
	if py := pythonInVenv(dir); py != "" {
		return py, nil
	}
	if existing := VenvDir(root); existing != "" && name == defaultVenv {
		return pythonInVenv(existing), nil
	}
	launcher, prefix := pythonLauncher()
	if launcher == "" {
		return "", fail(MissingRuntimeMessage("python"))
	}
	if stdout != nil {
		fmt.Fprintf(stdout, "Creating virtual environment %s…\n", name)
	}
	args := append(append([]string{}, prefix...), "-m", "venv", dir)
	cmd := exec.Command(launcher, args...)
	cmd.Dir = root
	cmd.Stdout = stdout
	cmd.Stderr = stdout
	if err := RunCmd(cmd); err != nil {
		return "", wrap(exitcode.Failure, "python -m venv "+name, err)
	}
	py := pythonInVenv(dir)
	if py == "" {
		return "", fail("virtual environment was created but python was not found in " + dir)
	}
	return py, nil
}

// VenvPython is the interpreter inside venvName (default .venv).
func VenvPython(root, name string) string {
	if name == "" {
		name = defaultVenv
	}
	if py := pythonInVenv(filepath.Join(root, name)); py != "" {
		return py
	}
	return pythonInVenv(VenvDir(root))
}

// ActivateHint is the shell command to activate the venv.
func ActivateHint(root, name string) string {
	if name == "" {
		name = defaultVenv
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(root, name, "Scripts", "activate")
	}
	return "source " + filepath.ToSlash(filepath.Join(name, "bin", "activate"))
}

func pipInstall(python string, stdout io.Writer, args ...string) error {
	cmd := exec.Command(python, append([]string{"-m", "pip", "install"}, args...)...)
	cmd.Stdout = stdout
	cmd.Stderr = stdout
	if err := RunCmd(cmd); err != nil {
		return wrap(exitcode.Failure, "pip install", err)
	}
	return nil
}

func hasRequirements(root string) string {
	for _, name := range []string{"requirements.txt", "requirements-dev.txt"} {
		p := filepath.Join(root, name)
		if fileExists(p) {
			return p
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if strings.HasPrefix(n, "requirements") && strings.HasSuffix(n, ".txt") {
			return filepath.Join(root, n)
		}
	}
	return ""
}
