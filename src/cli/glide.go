package cli

import (
	"fmt"
	"io"

	"github.com/polyapi/polyglot/src/api"
	"github.com/polyapi/polyglot/src/config"
	"github.com/polyapi/polyglot/src/delegate"
	"github.com/polyapi/polyglot/src/exitcode"
	"github.com/polyapi/polyglot/src/glide"
	"github.com/spf13/cobra"
)

func addGlideCommands(root *cobra.Command) {
	deploy := &cobra.Command{
		Use:   "deploy",
		Short: "Glide v2 pipeline for project deployables",
		Example: examples(
			ex{"Dry-run a push:", "polyapi deploy plan --env prod"},
			ex{"Apply local deployables:", "polyapi deploy push --env prod"},
		),
	}

	prepare := &cobra.Command{
		Use:   "prepare",
		Short: "Discover deployables and write auto-docs / AI fill-in",
		Args:  cobra.NoArgs,
		RunE:  runPrepare,
		Example: examples(
			ex{"Discover deployables and write auto-docs:", "polyapi deploy prepare"},
			ex{"Skip AI fill-in and generated docs:", "polyapi deploy prepare --disable-ai --disable-docs"},
		),
	}
	prepare.Flags().Bool("disable-docs", false, "Do not write docs / JSDoc / docstrings into deployable files")
	prepare.Flags().Bool("disable-ai", false, "Do not use AI to fill missing descriptions")

	validate := &cobra.Command{
		Use:   "validate",
		Short: "Preflight: env tokens, metadata, and cross-resource integrity",
		Args:  cobra.NoArgs,
		RunE:  runValidate,
		Example: examples(
			ex{"Run all preflight checks:", "polyapi deploy validate"},
			ex{"Env tokens only:", "polyapi deploy validate --only env"},
			ex{"Treat warnings as failures and confirm remote IDs:", "polyapi deploy validate --strict --online"},
		),
	}
	addValidateFlags(validate)

	envCheck := &cobra.Command{
		Use:   "env-check",
		Short: "Deprecated alias of `deploy validate --only env`",
		Args:  cobra.NoArgs,
		RunE:  runValidate,
		Example: examples(
			ex{"Deprecated alias of deploy validate --only env:", "polyapi deploy env-check"},
			ex{"Preferred form:", "polyapi deploy validate --only env"},
		),
	}
	addValidateFlags(envCheck)

	plan := &cobra.Command{
		Use:   "plan",
		Short: "Dry-run a push",
		Args:  cobra.NoArgs,
		RunE:  runPlan,
		Example: examples(
			ex{"Dry-run a push:", "polyapi deploy plan"},
			ex{"Limit to paths and types:", "polyapi deploy plan --path src/functions --types server,client"},
			ex{"Target a named environment:", "polyapi deploy plan --env prod"},
		),
	}
	addScopeFlags(plan)
	addSyncFlags(plan)
	plan.Flags().Bool("disable-ai", false, "Do not use AI during this run")

	push := &cobra.Command{
		Use:     "push",
		Aliases: []string{"sync"},
		Short:   "Apply local deployables to the remote instance (alias: sync)",
		Args:    cobra.NoArgs,
		RunE:    runPush,
		Example: examples(
			ex{"Apply local deployables:", "polyapi deploy push --env prod"},
			ex{"Same command under the sync alias:", "polyapi deploy sync --env prod"},
			ex{"Delete remotes with no local counterpart:", "polyapi deploy push --delete-orphans --yes --env prod"},
			ex{"Write in-file deploy receipts:", "polyapi deploy push --receipts --env prod"},
		),
	}
	addScopeFlags(push)
	addSyncFlags(push)
	push.Flags().Bool("disable-ai", false, "Do not use AI during this run")
	push.Flags().Bool("receipts", false, "Write in-file deploy receipts (off by default)")

	pull := &cobra.Command{
		Use:   "pull",
		Short: "Fetch remote resources into the local project",
		Args:  cobra.NoArgs,
		RunE:  runPull,
		Example: examples(
			ex{"Fetch remote resources:", "polyapi deploy pull"},
			ex{"Show file creates/updates without writing:", "polyapi deploy pull --dry-run"},
			ex{"Limit by type and context:", "polyapi deploy pull --types server --contexts billing"},
			ex{"Delete locals with no remote counterpart:", "polyapi deploy pull --delete-orphans"},
		),
	}
	addScopeFlags(pull)
	pull.Flags().Bool("delete-orphans", false, "Delete local JSON artifacts with no remote counterpart")
	pull.Flags().Bool("dry-run", false, "Show file creates/updates; write nothing")
	pull.Flags().Bool("force", false, "Overwrite local JSON even when it already matches")

	deploy.AddCommand(prepare, validate, envCheck, plan, push, pull)
	root.AddCommand(deploy)
}

func addValidateFlags(cmd *cobra.Command) {
	cmd.Flags().StringSlice("only", nil, "Run only these check categories (env|metadata|discovery|cross-resource|config)")
	cmd.Flags().Bool("strict", false, "Treat warnings as failures")
	cmd.Flags().Bool("online", false, "Confirm referenced remote IDs still exist")
}

func addScopeFlags(cmd *cobra.Command) {
	cmd.Flags().StringArray("path", nil, "Include only these repo paths (repeatable)")
	cmd.Flags().StringArray("exclude-path", nil, "Always skip these paths (repeatable)")
	cmd.Flags().StringSlice("types", nil, "Artifact types to include (comma-separated)")
	cmd.Flags().StringSlice("contexts", nil, "Context-prefix filter (comma-separated)")
}

func addSyncFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("force", false, "Update even when the content hash matches")
	cmd.Flags().Bool("resume", false, "Skip deployables already recorded in .poly/resume.json")
	cmd.Flags().Bool("allow-unresolved", false, "Deploy even if artifacts still contain {{TOKEN}} placeholders")
	cmd.Flags().Bool("delete-orphans", false, "Delete remote resources with no local counterpart")
}

func runPrepare(cmd *cobra.Command, _ []string) error {
	eng, err := newGlideEngine(cmd, false)
	if err != nil {
		return err
	}
	changed, skipped, err := eng.Prepare()
	if !globalsFrom(cmd).Quiet {
		for _, f := range changed {
			printOk(cmd.OutOrStdout(), "rewrote "+f)
		}
		if globalsFrom(cmd).Verbose > 0 {
			for _, f := range skipped {
				fmt.Fprintf(cmd.OutOrStdout(), "  skipped %s\n", f)
			}
		}
		if err == nil && len(changed) == 0 {
			printOk(cmd.OutOrStdout(), "prepare made no file changes")
		}
	}
	return fail(err)
}

func runValidate(cmd *cobra.Command, _ []string) error {
	eng, err := newGlideEngine(cmd, flagBool(cmd, "online"))
	if err != nil {
		return err
	}
	if cmd.Name() == "env-check" && len(eng.Only) == 0 {
		eng.Only = []string{glide.CatEnv}
	}
	rep, err := eng.Validate()
	if err != nil {
		return fail(err)
	}
	printValidate(cmd.OutOrStdout(), cmd.ErrOrStderr(), rep, globalsFrom(cmd).Quiet)
	if rep.Failed(eng.Strict) {
		errN, warnN, _ := rep.Counts()
		msg := fmt.Sprintf("validate failed (%d error(s), %d warning(s))", errN, warnN)
		return &ExitError{Code: exitcode.Failure, Msg: msg}
	}
	return nil
}

func runPlan(cmd *cobra.Command, _ []string) error {
	eng, err := newGlideEngine(cmd, true)
	if err != nil {
		return err
	}
	results, err := eng.Plan()
	if err != nil {
		return fail(err)
	}
	printResults(cmd.OutOrStdout(), results, globalsFrom(cmd).Quiet)
	return resultsError(results)
}

func runPush(cmd *cobra.Command, _ []string) error {
	eng, err := newGlideEngine(cmd, true)
	if err != nil {
		return err
	}
	results, err := eng.Push()
	if err != nil {
		return fail(err)
	}
	printResults(cmd.OutOrStdout(), results, globalsFrom(cmd).Quiet)
	return resultsError(results)
}

func runPull(cmd *cobra.Command, _ []string) error {
	eng, err := newGlideEngine(cmd, true)
	if err != nil {
		return err
	}
	results, err := eng.Pull()
	if err != nil {
		return fail(err)
	}
	printResults(cmd.OutOrStdout(), results, globalsFrom(cmd).Quiet)
	return resultsError(results)
}

func newGlideEngine(cmd *cobra.Command, needClient bool) (*glide.Engine, error) {
	g := globalsFrom(cmd)
	cfg, err := loadDelegateConfig(g)
	if err != nil {
		return nil, fail(err)
	}
	root := cfg.ProjectRoot
	if root == "" {
		root = projectRoot()
	}
	eval := config.EvaluateDeploy(root, cfg.File.Deploy, g.Env)
	envName := g.Env
	if envName == "" {
		envName = eval.Environment
	}
	cfg, err = cfg.WithEnvironment(envName)
	if err != nil {
		return nil, fail(err)
	}

	paths, _ := cmd.Flags().GetStringArray("path")
	exclude, _ := cmd.Flags().GetStringArray("exclude-path")
	types, _ := cmd.Flags().GetStringSlice("types")
	contexts, _ := cmd.Flags().GetStringSlice("contexts")
	scope := glide.MergeScope(cfg.File.Deploy, paths, exclude, types, contexts)

	eng := &glide.Engine{
		Root:            root,
		PolyPath:        g.PolyPath,
		Env:             glide.MergeEnv(root),
		Scope:           scope,
		Instance:        cfg.BaseURL,
		Receipts:        flagBool(cmd, "receipts") || cfg.File.Deploy.ReceiptsEnabled(),
		Force:           flagBool(cmd, "force"),
		DeleteOrphans:   flagBool(cmd, "delete-orphans"),
		Resume:          flagBool(cmd, "resume"),
		AllowUnresolved: flagBool(cmd, "allow-unresolved"),
		Online:          flagBool(cmd, "online"),
		Strict:          flagBool(cmd, "strict"),
		DisableAI:       flagBool(cmd, "disable-ai") || glide.DisableAIFromEnv(),
		DisableDocs:     flagBool(cmd, "disable-docs"),
		DryRun:          flagBool(cmd, "dry-run"),
		Yes:             g.Yes,
		NonInteractive:  g.NonInteractive,
		Production:      eval.Production,
		PushAllowed:     eval.Allowed,
		EvalStatus:      eval.Status,
		EvalMessage:     eval.Message,
		HasCredentials:  cfg.BaseURL != "" && cfg.APIKey != "",
		Verbose:         g.Verbose,
		Stdin:           cmd.InOrStdin(),
		Stderr:          cmd.ErrOrStderr(),
	}
	if only, err := cmd.Flags().GetStringSlice("only"); err == nil {
		eng.Only = only
	}

	if needClient {
		if _, _, err := cfg.RequireCredentials(); err != nil {
			return nil, fail(err)
		}
	}
	if client, err := api.FromConfig(cfg); err == nil {
		if needClient {
			eng.Client = client
		}
		eng.Describer = client
	} else if needClient {
		return nil, fail(err)
	}

	adapter, err := resolveAdapter(g, cfg)
	if err == nil {
		eng.Lang = adapter.Lang
		eng.Adapter = glide.DelegateAdapter{D: delegate.New(adapter.Root, adapter.Lang, adapter.Argv, g.PolyPath)}
	} else if loc := delegate.LocateProject(root, g.PolyPath); loc.Found && len(loc.Languages) == 1 {
		eng.Lang = loc.Languages[0]
	}
	return eng, nil
}

func flagBool(cmd *cobra.Command, name string) bool {
	if cmd.Flags().Lookup(name) == nil {
		return false
	}
	v, err := cmd.Flags().GetBool(name)
	return err == nil && v
}

func printValidate(out, errw io.Writer, rep glide.Report, quiet bool) {
	if quiet {
		return
	}
	for _, iss := range rep.Issues {
		line := iss.Severity + "  " + iss.Category
		if iss.Path != "" {
			line += "  " + iss.Path
		}
		if iss.Identity != "" {
			line += "  " + iss.Identity
		}
		line += "  " + iss.Message
		switch iss.Severity {
		case glide.SeverityError:
			fmt.Fprintln(errw, failText(line))
		case glide.SeverityWarn:
			fmt.Fprintln(out, warnText(line))
		default:
			fmt.Fprintln(out, infoText(line))
		}
	}
	errN, warnN, infoN := rep.Counts()
	summary := fmt.Sprintf("validate  %d error(s)  %d warning(s)  %d info  %d deployable(s)", errN, warnN, infoN, len(rep.Items))
	if errN > 0 {
		fmt.Fprintln(errw, failText(summary))
	} else if warnN > 0 {
		fmt.Fprintln(out, warnText(summary))
	} else {
		printOk(out, summary)
	}
}

func printResults(out io.Writer, results []glide.Result, quiet bool) {
	if quiet {
		return
	}
	for _, r := range results {
		tag, ok := LookupStatusTag(r.Action)
		label := r.Action
		if ok {
			label = tag.Symbol + "  " + tag.Label
			painted := paint(tag.Color, tag.Bold, fmt.Sprintf("%-14s", tag.Label))
			line := painted + "  " + r.Type + "  " + r.Label()
			if r.File != "" {
				line += "  " + r.File
			}
			if r.Error != "" {
				line += "  " + r.Error
			}
			fmt.Fprintln(out, line)
			continue
		}
		fmt.Fprintf(out, "%s  %s  %s  %s\n", label, r.Type, r.Label(), r.File)
	}
}

func resultsError(results []glide.Result) error {
	var failed, blocked int
	for _, r := range results {
		switch r.Action {
		case glide.ActionFailed:
			failed++
		case glide.ActionBlocked:
			blocked++
		}
	}
	if failed > 0 {
		return &ExitError{Code: exitcode.Partial, Msg: fmt.Sprintf("%d deployable(s) failed", failed)}
	}
	if blocked > 0 {
		return &ExitError{Code: exitcode.Failure, Msg: fmt.Sprintf("%d deployable(s) blocked", blocked)}
	}
	return nil
}

func doctorValidateCheck(g Globals, cfg config.Resolved, cfgOK bool) doctorCheck {
	c := doctorCheck{Name: "validate", Code: exitcode.Failure}
	if !cfgOK {
		c.Status = "skip"
		c.Detail = "skipped (config failed)"
		c.Code = 0
		return c
	}
	root := cfg.ProjectRoot
	if root == "" {
		root = projectRoot()
	}
	eng := &glide.Engine{
		Root:           root,
		PolyPath:       g.PolyPath,
		Env:            glide.MergeEnv(root),
		Scope:          glide.MergeScope(cfg.File.Deploy, nil, nil, nil, nil),
		HasCredentials: cfg.BaseURL != "" && cfg.APIKey != "",
		EvalMessage:    config.EvaluateDeploy(root, cfg.File.Deploy, g.Env).Message,
	}
	if adapter, err := resolveAdapter(g, cfg); err == nil {
		eng.Lang = adapter.Lang
		eng.Adapter = glide.DelegateAdapter{D: delegate.New(adapter.Root, adapter.Lang, adapter.Argv, g.PolyPath)}
	}
	rep, err := eng.Validate()
	if err != nil {
		if glide.IsNothingFound(err) {
			c.Status = "skip"
			c.Detail = "no deployables found"
			c.Code = 0
			return c
		}
		c.Status = "fail"
		c.Detail = err.Error()
		return c
	}
	errN, warnN, _ := rep.Counts()
	detail := fmt.Sprintf("%d deployable(s), %d error(s), %d warning(s)", len(rep.Items), errN, warnN)
	switch {
	case errN > 0:
		c.Status = "fail"
		c.Detail = detail
	case warnN > 0:
		c.Status = "warn"
		c.Detail = detail
		c.Code = 0
	default:
		c.Status = "ok"
		c.Detail = detail
		c.Code = 0
	}
	return c
}
