package delegate

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
)

// AdapterSource is how the launch command was chosen.
type AdapterSource string

const (
	AdapterFlag      AdapterSource = "flag"
	AdapterConfig    AdapterSource = "config"
	AdapterHeuristic AdapterSource = "heuristic"
	AdapterPrompt    AdapterSource = "prompt"
)

// ResolvedAdapter is the resolved adapter launch plan.
type ResolvedAdapter struct {
	Lang   Language
	Argv   []string
	Source AdapterSource
	Root   string
}

// DiscoverInput is the input for Discover.
type DiscoverInput struct {
	ProjectRoot    string
	PolyPath       string
	LangFlag       Language
	AdapterFlag    string
	ConfigLanguage Language
	ConfigAdapter  string
	NonInteractive bool
	Stdin          io.Reader
	Stdout         io.Writer
}

// Discover resolves the project root, language, and adapter launch command.
//
// The project root is the nearest parent (including start) with a language
// manifest or .poly config. Order for the launch command: --adapter / --lang
// → .poly language / adapter.command → manifests → prompt.
func Discover(input DiscoverInput) (ResolvedAdapter, error) {
	start := input.ProjectRoot
	if start == "" {
		if wd, err := os.Getwd(); err == nil {
			start = wd
		} else {
			start = "."
		}
	}
	polyPath := input.PolyPath
	if polyPath == "" {
		polyPath = ".poly"
	}
	loc := LocateProject(start, polyPath)
	root := start
	if loc.Found {
		root = loc.Root
	}
	langs := loc.Languages

	finish := func(lang Language, argv []string, source AdapterSource) ResolvedAdapter {
		return ResolvedAdapter{Lang: lang, Argv: argv, Source: source, Root: root}
	}

	if raw := strings.TrimSpace(input.AdapterFlag); raw != "" {
		lang := input.LangFlag
		if lang == "" {
			lang = input.ConfigLanguage
		}
		if lang == "" {
			if len(langs) > 0 {
				lang = langs[0]
			} else {
				lang = LangTypeScript
			}
		}
		argv, err := splitCommandLine(raw)
		if err != nil {
			return ResolvedAdapter{}, err
		}
		return finish(lang, resolveArgv(root, argv), AdapterFlag), nil
	}

	if input.LangFlag != "" {
		argv, err := argvFor(root, input.LangFlag, input.ConfigAdapter)
		if err != nil {
			return ResolvedAdapter{}, err
		}
		return finish(input.LangFlag, argv, AdapterFlag), nil
	}

	if input.ConfigLanguage != "" {
		argv, err := argvFor(root, input.ConfigLanguage, input.ConfigAdapter)
		if err != nil {
			return ResolvedAdapter{}, err
		}
		return finish(input.ConfigLanguage, argv, AdapterConfig), nil
	}

	if raw := strings.TrimSpace(input.ConfigAdapter); raw != "" {
		var lang Language
		switch len(langs) {
		case 1:
			lang = langs[0]
		case 0:
			lang = LangTypeScript
		default:
			return ResolvedAdapter{}, ambiguous(langs)
		}
		argv, err := splitCommandLine(raw)
		if err != nil {
			return ResolvedAdapter{}, err
		}
		return finish(lang, resolveArgv(root, argv), AdapterConfig), nil
	}

	var lang Language
	source := AdapterHeuristic
	switch len(langs) {
	case 1:
		lang = langs[0]
	case 0:
		var err error
		lang, err = promptLanguage(input)
		if err != nil {
			return ResolvedAdapter{}, err
		}
		source = AdapterPrompt
	default:
		if input.NonInteractive || !isTerminal(input.Stdin) {
			return ResolvedAdapter{}, ambiguous(langs)
		}
		var err error
		lang, err = promptLanguage(input)
		if err != nil {
			return ResolvedAdapter{}, err
		}
		source = AdapterPrompt
	}
	argv, err := DefaultCommand(root, lang)
	if err != nil {
		return ResolvedAdapter{}, err
	}
	return finish(lang, argv, source), nil
}

func argvFor(root string, lang Language, configAdapter string) ([]string, error) {
	if raw := strings.TrimSpace(configAdapter); raw != "" {
		argv, err := splitCommandLine(raw)
		if err != nil {
			return nil, err
		}
		return resolveArgv(root, argv), nil
	}
	return DefaultCommand(root, lang)
}

// DefaultCommand is the default launch argv for a language in a project.
// The PolyAPI SDK must be installed in that project; npx/global fallbacks
// are not used when the library is missing.
func DefaultCommand(projectRoot string, lang Language) ([]string, error) {
	switch lang {
	case LangTypeScript:
		argv, installed := typescriptAdapter(projectRoot)
		if !installed {
			return nil, sdkNotInstalled(lang)
		}
		if len(argv) == 0 {
			expected := filepath.Join(projectRoot, "node_modules", "polyapi", "build", "adapter.js")
			return nil, adapterBinaryMissing(lang, expected)
		}
		return argv, nil
	case LangPython:
		argv, installed := pythonAdapter(projectRoot)
		if !installed {
			return nil, sdkNotInstalled(lang)
		}
		return argv, nil
	case LangJava:
		return nil, javaUnsupported()
	default:
		return nil, usageErr("unknown language")
	}
}

// PythonLauncher is python3 when on PATH, else python.
func PythonLauncher() string {
	if CommandOnPath("python3") != "" {
		return "python3"
	}
	return "python"
}

// SplitCommandLine splits --adapter / adapter.command into argv. Quotes are honored.
func SplitCommandLine(raw string) ([]string, error) {
	return splitCommandLine(raw)
}

func splitCommandLine(raw string) ([]string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, usageErr("empty adapter command; pass --adapter <command>")
	}
	var out []string
	var cur strings.Builder
	var quote rune
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		switch {
		case c == '\\':
			// Treat \ as an escape only for quotes, backslash, and
			// whitespace (POSIX `foo\ bar`). Anything else — including
			// Windows path letters — is a literal backslash so
			// `--adapter node C:\foo\adapter.js` stays intact.
			if i+1 < len(runes) {
				next := runes[i+1]
				if next == '"' || next == '\'' || next == '\\' || unicode.IsSpace(next) {
					i++
					cur.WriteRune(next)
					continue
				}
			}
			cur.WriteRune('\\')
		case quote == 0 && (c == '"' || c == '\''):
			quote = c
		case quote != 0 && c == quote:
			quote = 0
		case quote == 0 && unicode.IsSpace(c):
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(c)
		}
	}
	if quote != 0 {
		return nil, usageErr("unclosed quote in adapter command")
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	if len(out) == 0 {
		return nil, usageErr("empty adapter command; pass --adapter <command>")
	}
	return out, nil
}

func resolveArgv(projectRoot string, argv []string) []string {
	out := make([]string, len(argv))
	for i, s := range argv {
		p := s
		looksLikePath := strings.Contains(s, "/") || strings.Contains(s, `\`) || strings.HasPrefix(s, ".") || filepath.Ext(s) != ""
		if looksLikePath && !filepath.IsAbs(s) {
			abs := filepath.Join(projectRoot, s)
			if _, err := os.Stat(abs); err == nil {
				p = abs
			}
		}
		out[i] = p
	}
	return out
}

func promptLanguage(input DiscoverInput) (Language, error) {
	if input.NonInteractive || !isTerminal(input.Stdin) {
		return "", undetected()
	}
	in := input.Stdin
	if in == nil {
		in = os.Stdin
	}
	out := input.Stdout
	if out == nil {
		out = os.Stdout
	}
	fmt.Fprintln(out, "Select the project language")
	fmt.Fprintln(out, "  1) typescript")
	fmt.Fprintln(out, "  2) python")
	fmt.Fprint(out, "> ")
	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		return "", undetected()
	}
	choice := strings.TrimSpace(scanner.Text())
	switch choice {
	case "1", "typescript", "ts":
		return LangTypeScript, nil
	case "2", "python", "py":
		return LangPython, nil
	default:
		if lang, ok := ParseLanguage(choice); ok && lang != LangJava {
			return lang, nil
		}
		return "", undetected()
	}
}

func isTerminal(r io.Reader) bool {
	if r == nil {
		r = os.Stdin
	}
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}

// CommandOnPath locates an interpreter used by tests and default launchers.
func CommandOnPath(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return path
}
