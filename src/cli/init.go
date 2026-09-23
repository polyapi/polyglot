package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/polyapi/polyglot/src/api"
	"github.com/polyapi/polyglot/src/config"
	"github.com/polyapi/polyglot/src/delegate"
	"github.com/polyapi/polyglot/src/exitcode"
	"github.com/polyapi/polyglot/src/projinit"
	"github.com/spf13/cobra"
)

func runInit(cmd *cobra.Command, _ []string) error {
	g := globalsFrom(cmd)
	existing, _ := cmd.Flags().GetBool("existing")
	force, _ := cmd.Flags().GetBool("force")
	noDeps, _ := cmd.Flags().GetBool("no-deps")
	noRuntime, _ := cmd.Flags().GetBool("no-runtime")
	name, _ := cmd.Flags().GetString("name")
	templateFlag, _ := cmd.Flags().GetString("template")
	provider, _ := cmd.Flags().GetString("provider")
	venvName, _ := cmd.Flags().GetString("venv")
	envMap, _ := cmd.Flags().GetStringArray("env-map")

	if provider != "" {
		switch strings.ToLower(provider) {
		case "github", "gitlab", "other":
		default:
			return failUsage("provider must be github, gitlab, or other")
		}
	}
	if provider == "" {
		provider = "github"
	}
	if venvName == "" {
		venvName = ".venv"
	}

	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()
	if g.Quiet {
		out = io.Discard
	}

	dest, err := projinit.ResolveDest(projectRoot(), g.PolyPath, existing)
	if err != nil {
		return fail(err)
	}
	if dest.Detected && dest.Root != projectRoot() {
		printOk(out, "detected project root "+displayInitPath(projectRoot(), dest.Root))
	}

	lang, err := resolveProjectLanguage(cmd, g, dest)
	if err != nil {
		return fail(err)
	}

	if !noRuntime {
		ask := func(kind projinit.RuntimeKind) (bool, error) {
			label := "Node.js is not installed. Install it now?"
			if kind == projinit.RuntimePython {
				label = "Python 3 is not installed. Install it now?"
			}
			if !canPrompt(cmd, g) && !g.Yes {
				return false, nil
			}
			return promptYes(cmd, label, true, g)
		}
		if _, err := projinit.EnsureRuntime(lang, ask, out, errOut); err != nil {
			return fail(err)
		}
	}

	src, err := resolveInitTemplate(cmd, g, dest, lang, templateFlag)
	if err != nil {
		return fail(err)
	}

	if src.Kind != projinit.KindNone {
		if err := applyTemplate(cmd, g, dest.Root, src, force, out); err != nil {
			return fail(err)
		}
	}

	if err := projinit.EnsureManifest(dest.Root, lang, name); err != nil {
		return fail(err)
	}

	if !existing {
		if _, err := projinit.GitInit(dest.Root, out); err != nil {
			return fail(err)
		}
	}

	switch act, err := projinit.EnsureIgnore(dest.Root, lang); {
	case err != nil:
		return fail(err)
	case act == config.GitignoreCreated:
		printOk(out, "created .gitignore")
	case act == config.GitignoreAppended:
		printOk(out, "updated .gitignore")
	case act == config.GitignoreSkippedNotGit:
		printWarning(out, "not a git repository; skipped .gitignore (.poly/ holds encrypted credentials)")
	}

	cfg, _ := config.Load(dest.Root, g.PolyPath, overlays(g), config.ProductionSecrets())
	baseURL := firstNonEmpty(g.BaseURL, cfg.BaseURL)
	apiVersion := cfg.APIVersion
	if apiVersion == "" {
		apiVersion = "1"
	}
	// Never persist POLY_API_KEY / --api-key; env is not written back to disk.
	if err := config.ApplyInit(dest.Root, g.PolyPath, config.InitSpec{
		Language:   lang,
		BaseURL:    baseURL,
		APIVersion: apiVersion,
		EnvMap:     envMap,
	}, config.ProductionSecrets()); err != nil {
		return fail(err)
	}
	printOk(out, "wrote "+filepath.ToSlash(filepath.Join(g.PolyPath, "config.toml")))

	var py string
	if lang == "python" && !noDeps {
		var err error
		py, err = projinit.EnsureVenv(dest.Root, venvName, out)
		if err != nil {
			return fail(err)
		}
		printOk(out, "virtualenv "+venvName)
		if err := projinit.InstallPyDeps(dest.Root, py, out); err != nil {
			return fail(err)
		}
		if err := projinit.InstallPreCommit(dest.Root, py, out); err != nil {
			printWarning(out, err.Error())
		}
	}
	if lang == "typescript" && !noDeps {
		if err := projinit.InstallJSDeps(dest.Root, out); err != nil {
			return fail(err)
		}
	}

	if _, err := projinit.EnsureHook(dest.Root, out); err != nil {
		printWarning(out, err.Error())
	}
	if _, err := projinit.EnsureCI(dest.Root, provider, out); err != nil {
		printWarning(out, err.Error())
	}

	loggedIn := baseURL != "" && cfg.APIKey != ""
	printOk(out, "initialized "+displayInitPath(projectRoot(), dest.Root))
	if !g.Quiet {
		fmt.Fprint(out, projinit.NextSteps(dest.Root, lang, provider, venvName, loggedIn))
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), displayInitPath(projectRoot(), dest.Root))
	}
	return nil
}

func resolveProjectLanguage(cmd *cobra.Command, g Globals, dest projinit.Destination) (string, error) {
	if g.Lang != "" {
		return projinit.LanguageFromRuntime(g.Lang)
	}
	var found []string
	for _, l := range dest.Langs {
		if l == delegate.LangTypeScript || l == delegate.LangPython {
			found = append(found, string(l))
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		if !canPrompt(cmd, g) {
			return "typescript", nil
		}
		idx, err := promptChoice(cmd, "Project language:", []string{"TypeScript", "Python"}, 0, g)
		if err != nil {
			return "", err
		}
		if idx == 1 {
			return "python", nil
		}
		return "typescript", nil
	default:
		if !canPrompt(cmd, g) {
			return "", failUsage("multiple languages detected (" + strings.Join(found, ", ") + "); pass --lang typescript|python")
		}
		idx, err := promptChoice(cmd, "Multiple languages detected. Choose one:", found, 0, g)
		if err != nil {
			return "", err
		}
		return found[idx], nil
	}
}

func resolveInitTemplate(cmd *cobra.Command, g Globals, dest projinit.Destination, lang, templateFlag string) (projinit.Source, error) {
	if cmd.Flags().Changed("template") || templateFlag != "" {
		return projinit.ParseSource(templateFlag)
	}
	if !dest.First {
		return projinit.None(), nil
	}
	if !canPrompt(cmd, g) {
		if g.Yes {
			return projinit.OfficialFor(lang), nil
		}
		return projinit.None(), failUsage("pass --template (none, a GitHub repo, a zip URL, or an official name) or run interactively")
	}

	choices, sources := templateChoices(cmd, g, dest.Root, lang)
	idx, err := promptChoice(cmd, "Use a project template?", choices, 1, g)
	if err != nil {
		return projinit.Source{}, err
	}
	if idx < 0 || idx >= len(sources) {
		return projinit.None(), nil
	}
	src := sources[idx]
	if src.Kind == projinit.KindGitHub && src.GitHub == "" && src.Label == "github-prompt" {
		line, err := promptString(cmd, "GitHub repository (owner/repo or URL)", "", g)
		if err != nil {
			return projinit.Source{}, err
		}
		gh, ok := projinit.ParseGitHub(line)
		if !ok {
			return projinit.Source{}, failUsage("not a GitHub repository; use owner/repo or https://github.com/owner/repo")
		}
		return gh, nil
	}
	return src, nil
}

func templateChoices(cmd *cobra.Command, g Globals, root, lang string) (labels []string, sources []projinit.Source) {
	labels = append(labels, "No (empty project)")
	sources = append(sources, projinit.None())

	official := projinit.OfficialFor(lang)
	labels = append(labels, official.Label)
	sources = append(sources, official)

	cat := loadCatalogue(cmd, g, root)
	for _, s := range cat.SourcesForLang(lang) {
		if s.URL == official.ZipURL() || strings.Contains(s.URL, official.GitHub) {
			continue
		}
		labels = append(labels, s.Label)
		sources = append(sources, s)
	}

	labels = append(labels, "Enter a GitHub repository")
	sources = append(sources, projinit.Source{Kind: projinit.KindGitHub, Label: "github-prompt"})
	return labels, sources
}

func loadCatalogue(cmd *cobra.Command, g Globals, root string) projinit.Catalogue {
	cfg, err := config.Load(root, g.PolyPath, overlays(g), config.ProductionSecrets())
	if err != nil {
		return projinit.Catalogue{}
	}
	url, key, err := cfg.RequireCredentials()
	if err != nil {
		if !canPrompt(cmd, g) {
			return projinit.Catalogue{}
		}
		ok, err := promptYes(cmd, "Log in to load tenant project templates from the API?", true, g)
		if err != nil || !ok {
			return projinit.Catalogue{}
		}
		url, err = takeURL(cmd, g, "", cfg)
		if err != nil {
			printWarning(cmd.ErrOrStderr(), "could not log in; continuing without tenant templates")
			return projinit.Catalogue{}
		}
		key, err = takeKey(cmd, g, "", cfg)
		if err != nil {
			printWarning(cmd.ErrOrStderr(), "could not log in; continuing without tenant templates")
			return projinit.Catalogue{}
		}
		if _, err := config.SaveProjectCredentials(root, g.PolyPath, url, key, "1", config.ProductionSecrets()); err != nil {
			printWarning(cmd.ErrOrStderr(), err.Error())
			return projinit.Catalogue{}
		}
		cfg, err = config.Load(root, g.PolyPath, overlays(g), config.ProductionSecrets())
		if err != nil {
			return projinit.Catalogue{}
		}
		url, key, err = cfg.RequireCredentials()
		if err != nil {
			return projinit.Catalogue{}
		}
	}
	client, err := api.NewHTTPClient(url, key, api.Options{APIVersion: cfg.APIVersion})
	if err != nil {
		return projinit.Catalogue{}
	}
	auth, err := client.Auth()
	if err != nil {
		printWarning(cmd.ErrOrStderr(), "could not load tenant templates: "+api.UserMessage(err))
		return projinit.Catalogue{}
	}
	if auth.Tenant == nil || auth.Environment == nil || auth.Tenant.ID == "" || auth.Environment.ID == "" {
		return projinit.Catalogue{}
	}
	raw, err := client.Get(
		"tenants/"+auth.Tenant.ID+"/environments/"+auth.Environment.ID+"/config-variables",
		"ProjectTemplates",
	)
	if err != nil {
		if isAPINotFound(err) {
			return projinit.Catalogue{}
		}
		printWarning(cmd.ErrOrStderr(), "could not load ProjectTemplates: "+api.UserMessage(err))
		return projinit.Catalogue{}
	}
	cat, err := projinit.ParseCatalogue(raw)
	if err != nil {
		printWarning(cmd.ErrOrStderr(), err.Error())
		return projinit.Catalogue{}
	}
	return cat
}

func isAPINotFound(err error) bool {
	var e *api.Error
	return errors.As(err, &e) && e != nil && e.StatusCode == 404
}

func applyTemplate(cmd *cobra.Command, g Globals, dest string, src projinit.Source, force bool, out io.Writer) error {
	printOk(out, "fetching template "+src.Label)
	data, err := projinit.FetchZip(src)
	if err != nil {
		return err
	}
	plan, err := projinit.PlanUnpack(data, dest)
	if err != nil {
		return err
	}
	overwrite := force
	if len(plan.Conflicts) > 0 && !force {
		fmt.Fprintln(out, "These files already exist and would be replaced:")
		for _, c := range plan.Conflicts {
			fmt.Fprintf(out, "  %s\n", c)
		}
		if canPrompt(cmd, g) {
			ok, err := promptYes(cmd, "Overwrite existing files?", false, g)
			if err != nil {
				return err
			}
			overwrite = ok
		} else if g.Yes {
			printWarning(out, "leaving existing files in place (pass --force to overwrite)")
		} else {
			return &ExitError{Code: exitcode.Usage, Msg: "template would overwrite existing files; pass --force or run interactively"}
		}
	}
	result, err := projinit.Unpack(data, dest, overwrite)
	if err != nil {
		return err
	}
	printOk(out, fmt.Sprintf("unpacked %d files", len(result.Wrote)))
	if len(result.Skipped) > 0 {
		printWarning(out, fmt.Sprintf("skipped %d existing files", len(result.Skipped)))
	}
	return nil
}
