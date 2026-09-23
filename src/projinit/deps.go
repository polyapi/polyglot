package projinit

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/polyapi/polyglot/src/exitcode"
)

const (
	npmPolyPackage = "polyapi"
	pipPolyPackage = "polyapi-python"
)

// PackageManager is npm, yarn, pnpm, or bun.
func PackageManager(root string) string {
	switch {
	case fileExists(filepath.Join(root, "bun.lock")) || fileExists(filepath.Join(root, "bun.lockb")):
		return "bun"
	case fileExists(filepath.Join(root, "pnpm-lock.yaml")):
		return "pnpm"
	case fileExists(filepath.Join(root, "yarn.lock")):
		return "yarn"
	default:
		return "npm"
	}
}

// InstallJSDeps runs the package manager install, adding polyapi if missing.
func InstallJSDeps(root string, stdout io.Writer) error {
	pkgPath := filepath.Join(root, "package.json")
	if !fileExists(pkgPath) {
		return fail("package.json is missing; cannot install TypeScript dependencies")
	}
	if err := ensureNPMDep(pkgPath, npmPolyPackage); err != nil {
		return err
	}
	pm := PackageManager(root)
	bin := findBinary(pm)
	if bin == "" {
		return fail(pm + " is not installed. Install Node.js from " + NodeDownloadURL + " and re-run `polyapi init`.")
	}
	if stdout != nil {
		fmt.Fprintf(stdout, "Installing Node dependencies with %s…\n", pm)
	}
	var cmd *exec.Cmd
	switch pm {
	case "yarn":
		cmd = exec.Command(bin, "install")
	case "pnpm":
		cmd = exec.Command(bin, "install")
	case "bun":
		cmd = exec.Command(bin, "install")
	default:
		cmd = exec.Command(bin, "install")
	}
	cmd.Dir = root
	cmd.Stdout = stdout
	cmd.Stderr = stdout
	if err := RunCmd(cmd); err != nil {
		return wrap(exitcode.Failure, pm+" install", err)
	}
	return nil
}

func ensureNPMDep(pkgPath, name string) error {
	raw, err := os.ReadFile(pkgPath)
	if err != nil {
		return wrap(exitcode.Failure, "read package.json", err)
	}
	var pkg map[string]any
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return fail("package.json is not valid JSON")
	}
	if hasNPMDep(pkg, name) {
		return nil
	}
	deps, _ := pkg["dependencies"].(map[string]any)
	if deps == nil {
		deps = map[string]any{}
		pkg["dependencies"] = deps
	}
	deps[name] = "latest"
	out, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return wrap(exitcode.Failure, "serialize package.json", err)
	}
	out = append(out, '\n')
	return os.WriteFile(pkgPath, out, 0o644)
}

func hasNPMDep(pkg map[string]any, name string) bool {
	for _, key := range []string{"dependencies", "devDependencies"} {
		m, _ := pkg[key].(map[string]any)
		if m == nil {
			continue
		}
		if _, ok := m[name]; ok {
			return true
		}
	}
	return false
}

// InstallPyDeps pip-installs requirements or polyapi-python into the venv.
func InstallPyDeps(root, python string, stdout io.Writer) error {
	if python == "" {
		return fail("python interpreter is missing")
	}
	if stdout != nil {
		fmt.Fprintln(stdout, "Installing Python dependencies…")
	}
	req := hasRequirements(root)
	if req != "" {
		if err := ensureRequirementsLine(req, pipPolyPackage); err != nil {
			return err
		}
		return pipInstall(python, stdout, "-r", req)
	}
	if fileExists(filepath.Join(root, "pyproject.toml")) {
		if err := pipInstall(python, stdout, pipPolyPackage); err != nil {
			return err
		}
		return nil
	}
	return pipInstall(python, stdout, pipPolyPackage)
}

func ensureRequirementsLine(path, pkg string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return wrap(exitcode.Failure, "read "+filepath.Base(path), err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		ident := trim
		if i := strings.IndexAny(ident, "=<!["); i >= 0 {
			ident = ident[:i]
		}
		if strings.EqualFold(strings.TrimSpace(ident), pkg) {
			return nil
		}
	}
	text := string(raw)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += pkg + "\n"
	return os.WriteFile(path, []byte(text), 0o644)
}

// InstallPreCommit runs python -m pre_commit install when the config exists.
func InstallPreCommit(root, python string, stdout io.Writer) error {
	cfg := filepath.Join(root, ".pre-commit-config.yaml")
	if !fileExists(cfg) && !fileExists(filepath.Join(root, ".pre-commit-config.yml")) {
		return nil
	}
	if python == "" {
		return nil
	}
	if stdout != nil {
		fmt.Fprintln(stdout, "Installing pre-commit hooks…")
	}
	cmd := exec.Command(python, "-m", "pre_commit", "install")
	cmd.Dir = root
	cmd.Stdout = stdout
	cmd.Stderr = stdout
	if err := RunCmd(cmd); err != nil {
		return wrap(exitcode.Failure, "pre-commit install", err)
	}
	return nil
}
