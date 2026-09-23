package cli

import (
	"os"

	"github.com/polyapi/polyglot/src/config"
	"github.com/spf13/cobra"
)

// Globals are persistent flags plus env overlays.
type Globals struct {
	Env            string
	BaseURL        string
	APIKey         string
	Lang           string
	Adapter        string
	PolyPath       string
	NonInteractive bool
	Yes            bool
	Verbose        int
	Quiet          bool
	baseURLChanged bool
	apiKeyChanged  bool
}

func globalsFrom(cmd *cobra.Command) Globals {
	flags := cmd.Root().PersistentFlags()
	g := Globals{}
	g.Env, _ = flags.GetString("env")
	g.BaseURL, _ = flags.GetString("base-url")
	g.baseURLChanged = flags.Changed("base-url")
	g.APIKey, _ = flags.GetString("api-key")
	g.apiKeyChanged = flags.Changed("api-key")
	if f := flags.Lookup("lang"); f != nil {
		g.Lang = f.Value.String()
	}
	g.Adapter, _ = flags.GetString("adapter")
	g.PolyPath, _ = flags.GetString("poly-path")
	if g.PolyPath == "" {
		g.PolyPath = ".poly"
	}
	g.NonInteractive, _ = flags.GetBool("non-interactive")
	g.Yes, _ = flags.GetBool("yes")
	g.Verbose, _ = flags.GetCount("verbose")
	g.Quiet, _ = flags.GetBool("quiet")
	return g
}

func overlays(g Globals) config.Overlays {
	var o config.Overlays
	if g.baseURLChanged {
		o.BaseURL = &config.Overlay{Value: g.BaseURL, Source: config.SourceFlag}
	}
	if g.apiKeyChanged {
		o.APIKey = &config.Overlay{Value: g.APIKey, Source: config.SourceFlag}
	}
	return o
}

func projectRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

func loadFromGlobal(g Globals) (config.Resolved, error) {
	return config.Load(projectRoot(), g.PolyPath, overlays(g), config.ProductionSecrets())
}

func fail(err error) error {
	if err == nil {
		return nil
	}
	if ex, ok := err.(*ExitError); ok {
		return ex
	}
	type coder interface {
		ExitCode() int
		Error() string
	}
	if c, ok := err.(coder); ok {
		return &ExitError{Code: c.ExitCode(), Msg: c.Error()}
	}
	return &ExitError{Code: exitFail, Msg: err.Error()}
}

func failUsage(msg string) error {
	return &ExitError{Code: exitUsage, Msg: msg}
}
