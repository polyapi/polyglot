package cli

import (
	"fmt"
	"strings"

	"github.com/polyapi/polyglot/src/api"
	"github.com/polyapi/polyglot/src/config"
	"github.com/polyapi/polyglot/src/delegate"
	"github.com/spf13/cobra"
)

func runGenerate(cmd *cobra.Command, _ []string) error {
	g := globalsFrom(cmd)
	cfg, err := loadDelegateConfig(g)
	if err != nil {
		return fail(err)
	}
	if _, _, err := cfg.RequireCredentials(); err != nil {
		return fail(err)
	}
	adapter, err := resolveAdapter(g, cfg)
	if err != nil {
		return failDelegate(cmd, err)
	}
	if g.Verbose > 0 {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("using %s adapter in %s (%s)", adapter.Lang, adapter.Root, strings.Join(adapter.Argv, " ")))
	}
	client, err := api.FromConfig(cfg)
	if err != nil {
		return fail(err)
	}
	contexts, _ := cmd.Flags().GetStringSlice("contexts")
	names, _ := cmd.Flags().GetStringSlice("names")
	ids, _ := cmd.Flags().GetStringSlice("ids")
	noTypes, _ := cmd.Flags().GetBool("no-types")
	return generateWithClient(cmd, g, adapter, client, contexts, names, ids, noTypes)
}

func generateWithClient(cmd *cobra.Command, g Globals, adapter delegate.ResolvedAdapter, client *api.HTTPClient, contexts, names, ids []string, noTypes bool) error {
	auth, err := client.Auth()
	if err != nil {
		return fail(err)
	}
	if err := api.RequirePermissions(auth, []api.PermissionReq{api.Requirement("libraryGenerate")}); err != nil {
		return fail(err)
	}
	specs, err := client.Specs(contexts, names, ids, noTypes)
	if err != nil {
		return fail(err)
	}
	del := delegate.New(adapter.Root, adapter.Lang, adapter.Argv, g.PolyPath)
	result, err := del.Generate(delegate.GenerateParams{
		Specs:   delegate.SpecsJSON(specs),
		NoTypes: noTypes,
	})
	if err != nil {
		return failDelegate(cmd, err)
	}
	if !g.Quiet {
		n := len(result.FilesWritten)
		plural := "s"
		if n == 1 {
			plural = ""
		}
		printOk(cmd.OutOrStdout(), fmt.Sprintf("generated %d file%s via %s adapter", n, plural, adapter.Lang))
		if g.Verbose > 0 {
			for _, file := range result.FilesWritten {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", file)
			}
		}
	}
	return nil
}

func runClear(cmd *cobra.Command, _ []string) error {
	g := globalsFrom(cmd)
	cfg, err := loadDelegateConfig(g)
	if err != nil {
		return fail(err)
	}
	adapter, err := resolveAdapter(g, cfg)
	if err != nil {
		return failDelegate(cmd, err)
	}
	del := delegate.New(adapter.Root, adapter.Lang, adapter.Argv, g.PolyPath)
	if _, err := del.Clear(); err != nil {
		return failDelegate(cmd, err)
	}
	if !g.Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("cleared generated SDK (%s)", adapter.Lang))
	}
	return nil
}

func loadDelegateConfig(g Globals) (config.Resolved, error) {
	start := projectRoot()
	loc := delegate.LocateProject(start, g.PolyPath)
	root := start
	if loc.Found {
		root = loc.Root
	}
	return config.Load(root, g.PolyPath, overlays(g), config.ProductionSecrets())
}

func resolveAdapter(g Globals, cfg config.Resolved) (delegate.ResolvedAdapter, error) {
	var langFlag delegate.Language
	if g.Lang != "" {
		if lang, ok := delegate.ParseLanguage(g.Lang); ok {
			langFlag = lang
		}
	}
	var configLang delegate.Language
	if cfg.Language != "" {
		if lang, ok := delegate.ParseLanguage(cfg.Language); ok {
			configLang = lang
		}
	}
	return delegate.Discover(delegate.DiscoverInput{
		ProjectRoot:    cfg.ProjectRoot,
		PolyPath:       g.PolyPath,
		LangFlag:       langFlag,
		AdapterFlag:    g.Adapter,
		ConfigLanguage: configLang,
		ConfigAdapter:  cfg.AdapterCommand,
		NonInteractive: g.NonInteractive,
	})
}

func failDelegate(cmd *cobra.Command, err error) error {
	if de, ok := err.(*delegate.Error); ok {
		for _, w := range de.Warnings {
			if w.Path == "" {
				printWarning(cmd.ErrOrStderr(), fmt.Sprintf("%s: %s", w.Code, w.Message))
			} else {
				printWarning(cmd.ErrOrStderr(), fmt.Sprintf("%s: %s (%s)", w.Code, w.Message, w.Path))
			}
		}
	}
	return fail(err)
}
