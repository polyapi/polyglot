package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/polyapi/polyglot/src/config"
	"github.com/polyapi/polyglot/src/delegate"
	"github.com/spf13/cobra"
)

// SessionContext is the live identity + project snapshot shown in the banner and TUI.
type SessionContext struct {
	Cwd            string
	ProjectRoot    string
	ProjectFound   bool
	Languages      []string
	Language       string
	LanguageSource string
	Env            string
	Instance       string
	BaseURL        string
	URLSource      string
	APIKey         string
	KeySource      string
	Adapter        string
	PolyPath       string
	HasCredentials bool
}

// LoadSessionContext resolves credentials, the nearest project, and detected language.
func LoadSessionContext(g Globals) SessionContext {
	cwd := projectRoot()
	s := SessionContext{
		Cwd:      cwd,
		PolyPath: g.PolyPath,
		Env:      g.Env,
		Adapter:  g.Adapter,
	}
	if s.PolyPath == "" {
		s.PolyPath = ".poly"
	}
	loc := delegate.LocateProject(cwd, s.PolyPath)
	root := cwd
	if loc.Found {
		root = loc.Root
		s.ProjectFound = true
	}
	s.ProjectRoot = root
	for _, lang := range loc.Languages {
		s.Languages = append(s.Languages, lang.String())
	}

	cfg, err := config.Load(root, s.PolyPath, overlays(g), config.ProductionSecrets())
	if err == nil {
		s.Instance = cfg.Instance
		s.BaseURL = cfg.BaseURL
		s.URLSource = cfg.URLSource.String()
		s.APIKey = cfg.APIKey
		s.KeySource = cfg.KeySource.String()
		s.HasCredentials = cfg.BaseURL != "" && cfg.APIKey != ""
		if cfg.AdapterCommand != "" && s.Adapter == "" {
			s.Adapter = cfg.AdapterCommand
		}
		if cfg.Language != "" && g.Lang == "" {
			s.Language = cfg.Language
			s.LanguageSource = "config"
		}
	}

	switch {
	case g.Lang != "":
		s.Language = g.Lang
		s.LanguageSource = "flag"
	case s.Language != "":
		// config already set
	case len(s.Languages) == 1:
		s.Language = s.Languages[0]
		s.LanguageSource = "detected"
	}

	if s.URLSource == "(unset)" {
		s.URLSource = ""
	}
	if s.KeySource == "(unset)" {
		s.KeySource = ""
	}
	return s
}

// FormatBanner is the one-line context shown before human commands.
func FormatBanner(s SessionContext) string {
	var parts []string
	if s.Instance != "" {
		parts = append(parts, s.Instance)
	} else if s.BaseURL != "" {
		parts = append(parts, s.BaseURL)
	} else {
		parts = append(parts, "(no instance)")
	}
	if s.Env != "" {
		parts = append(parts, "env="+s.Env)
	}
	if s.HasCredentials {
		parts = append(parts, "key="+config.RedactSecret(s.APIKey))
	} else {
		parts = append(parts, "(no credentials)")
	}
	if s.Language != "" {
		parts = append(parts, s.Language)
	} else if len(s.Languages) > 0 {
		parts = append(parts, strings.Join(s.Languages, "|"))
	} else {
		parts = append(parts, "lang=?")
	}
	parts = append(parts, compactPath(s.ProjectRoot))
	return strings.Join(parts, "  ")
}

// CompactPath is the home-relative project path shown in context.
func CompactPath(path string) string { return compactPath(path) }

func compactPath(path string) string {
	if path == "" {
		return "."
	}
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if path == home {
			return "~"
		}
		sep := string(filepath.Separator)
		if strings.HasPrefix(path, home+sep) {
			return "~" + path[len(home):]
		}
	}
	return path
}

func printProjectContext(w io.Writer, s SessionContext) {
	proj := compactPath(s.ProjectRoot)
	if !s.ProjectFound {
		proj += " (no project)"
	}
	fmt.Fprintf(w, "%s:     %s\n", infoText("project"), proj)
	lang := s.Language
	if lang == "" && len(s.Languages) > 0 {
		lang = strings.Join(s.Languages, ", ")
	}
	if lang == "" {
		lang = "(none)"
	} else if s.LanguageSource != "" && s.Language != "" {
		lang = lang + " (" + s.LanguageSource + ")"
	}
	fmt.Fprintf(w, "%s:   %s\n", infoText("language"), lang)
}

func shouldShowBanner(cmd *cobra.Command, g Globals) bool {
	if g.Quiet || g.NonInteractive {
		return false
	}
	if !writerIsTTY(cmd.ErrOrStderr()) {
		return false
	}
	if skipBannerName(cmd) {
		return false
	}
	if f := cmd.Flags().Lookup("help"); f != nil && f.Changed {
		return false
	}
	return true
}

func skipBannerName(cmd *cobra.Command) bool {
	switch cmd.Name() {
	case "help", "completion", "man", "version", "doctor", "update", "polyapi":
		return true
	}
	for c := cmd; c != nil && c.Parent() != nil; c = c.Parent() {
		switch c.Name() {
		case "auth", "config":
			return true
		}
	}
	return false
}

func maybePrintBanner(cmd *cobra.Command) {
	g := globalsFrom(cmd)
	if !shouldShowBanner(cmd, g) {
		return
	}
	fmt.Fprintln(cmd.ErrOrStderr(), paint(Stone500, false, FormatBanner(LoadSessionContext(g))))
}

func shouldEnterTUI(cmd *cobra.Command) bool {
	g := globalsFrom(cmd)
	if g.NonInteractive {
		return false
	}
	if os.Getenv("CI") != "" {
		return false
	}
	in := cmd.InOrStdin()
	if in == nil {
		in = os.Stdin
	}
	out := cmd.OutOrStdout()
	if out == nil {
		out = os.Stdout
	}
	return isInteractive(in) && writerIsTTY(out)
}

func writerIsTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}
