package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/polyapi/polyglot/src/api"
	"github.com/polyapi/polyglot/src/config"
	"github.com/polyapi/polyglot/src/exitcode"
	"github.com/polyapi/polyglot/src/model"
	"github.com/spf13/cobra"
)

func addModelCommands(root *cobra.Command) {
	modelCmd := &cobra.Command{
		Use:   "model",
		Short: "OpenAPI → Spec Input → validate/train",
		Example: examples(
			ex{"From an OpenAPI document:", "polyapi model generate openapi.yaml spec.json --context billing"},
			ex{"Validate a spec input file:", "polyapi model validate spec.json"},
			ex{"Train API functions from a spec:", "polyapi model train spec.json"},
		),
	}

	gen := &cobra.Command{
		Use:   "generate <path> [destination]",
		Short: "Generate a Poly specification input from an OpenAPI document",
		Args:  cobra.RangeArgs(1, 2),
		RunE:  runModelGenerate,
		Example: examples(
			ex{"From an OpenAPI document:", "polyapi model generate openapi.yaml spec.json --context billing"},
			ex{"Hardcode the host URL:", "polyapi model generate openapi.yaml --host-url https://api.example.com"},
			ex{"Require host URL as an argument:", "polyapi model generate openapi.yaml --host-url-as-argument"},
			ex{"Rename generated operations:", "polyapi model generate openapi.yaml --rename foo:bar --disable-ai"},
		),
	}
	gen.Flags().String("context", "", "Context for all generated API functions")
	gen.Flags().String("host-url", "", "Hardcode the host URL for all API functions")
	gen.Flags().String("host-url-as-argument", "", "Require the host URL as a named argument (default name: `hostUrl`)")
	gen.Flags().Lookup("host-url-as-argument").NoOptDefVal = "hostUrl"
	gen.Flags().Bool("disable-ai", false, "Disable AI generation")
	gen.Flags().StringArray("rename", nil, "Name mappings, for example `foo:bar`")

	validate := &cobra.Command{
		Use:   "validate <path>",
		Short: "Validate a Poly specification input file",
		Args:  cobra.ExactArgs(1),
		RunE:  runModelValidate,
		Example: examples(
			ex{"Validate a spec input file:", "polyapi model validate spec.json"},
		),
	}

	train := &cobra.Command{
		Use:   "train <path>",
		Short: "Train API functions from a Poly specification input file",
		Args:  cobra.ExactArgs(1),
		RunE:  runModelTrain,
		Example: examples(
			ex{"Train API functions from a spec:", "polyapi model train spec.json"},
		),
	}

	modelCmd.AddCommand(gen, validate, train)
	root.AddCommand(modelCmd)
}

func runModelGenerate(cmd *cobra.Command, args []string) error {
	client, _, g, err := modelClient(cmd)
	if err != nil {
		return err
	}

	hostURL, _ := cmd.Flags().GetString("host-url")
	if hostURL != "" && !model.ValidHostURL(hostURL) {
		return failUsage(hostURL + " is not a valid url")
	}
	hostArg := ""
	if cmd.Flags().Changed("host-url-as-argument") {
		hostArg, _ = cmd.Flags().GetString("host-url-as-argument")
		if hostArg == "" {
			hostArg = "hostUrl"
		}
	}
	renames, err := parseRenameFlags(cmd)
	if err != nil {
		return err
	}
	context, _ := cmd.Flags().GetString("context")
	disableAI, _ := cmd.Flags().GetBool("disable-ai")
	dest := ""
	if len(args) == 2 {
		dest = args[1]
	}

	if !g.Quiet {
		fmt.Fprintln(cmd.OutOrStdout(), infoText("Translating specification into Poly specification input and generating context, names and descriptions for all resources..."))
		if model.IsRemotePath(args[0]) {
			fmt.Fprintln(cmd.OutOrStdout(), infoText("Fetching OpenAPI spec from provided url..."))
		}
		if disableAI {
			fmt.Fprintln(cmd.OutOrStdout(), infoText("AI generation is disabled."))
		}
	}

	result, err := model.Generate(client, model.GenerateOptions{
		Context:           context,
		HostURL:           hostURL,
		HostURLAsArgument: hostArg,
		DisableAI:         disableAI,
		Rename:            renames,
		SourcePath:        args[0],
		Destination:       dest,
		OnProgress: func(msg string) {
			if !g.Quiet {
				fmt.Fprintln(cmd.OutOrStdout(), infoText(msg))
			}
		},
	})
	if err != nil {
		return failModel(err)
	}
	if !g.Quiet {
		for _, w := range result.Warnings {
			printWarning(cmd.ErrOrStderr(), w)
		}
		printOk(cmd.OutOrStdout(), "wrote specification input to "+result.Path)
	}
	return nil
}

func runModelValidate(cmd *cobra.Command, args []string) error {
	client, _, g, err := modelClient(cmd)
	if err != nil {
		return err
	}
	if !g.Quiet {
		fmt.Fprintln(cmd.OutOrStdout(), infoText("Validating Poly specification input..."))
	}
	if err := model.Validate(client, args[0]); err != nil {
		return failModel(err)
	}
	if !g.Quiet {
		printOk(cmd.OutOrStdout(), "specification input is valid")
	}
	return nil
}

func runModelTrain(cmd *cobra.Command, args []string) error {
	client, cfg, g, err := modelClient(cmd)
	if err != nil {
		return err
	}
	if !g.Quiet {
		fmt.Fprintln(cmd.OutOrStdout(), infoText("Training Poly resources..."))
	}

	spec, err := model.Load(args[0])
	if err != nil {
		return failModel(err)
	}
	auth, err := client.Auth()
	if err != nil {
		return failModel(err)
	}
	if err := api.RequirePermissions(auth, model.TrainingRequirements(spec)); err != nil {
		return failModel(err)
	}

	result, err := model.TrainSpec(client, spec, func(msg string) {
		if !g.Quiet {
			fmt.Fprintln(cmd.OutOrStdout(), infoText(msg))
		}
	})
	if err != nil {
		return failModel(err)
	}

	if !g.Quiet {
		printTrainResult(cmd, result)
	}

	if result.CreatedCount() > 0 {
		generateAfterTrain(cmd, g, cfg)
	}

	if len(result.Failed) > 0 && result.CreatedCount() > 0 {
		return &ExitError{Code: exitcode.Partial, Msg: fmt.Sprintf("%d resource(s) failed to train", len(result.Failed))}
	}
	if len(result.Failed) > 0 {
		return &ExitError{Code: exitFail, Msg: fmt.Sprintf("%d resource(s) failed to train", len(result.Failed))}
	}
	return nil
}

func printTrainResult(cmd *cobra.Command, result model.TrainResult) {
	byKind := map[string][]model.TrainedResource{}
	for _, created := range result.Created {
		byKind[created.Kind] = append(byKind[created.Kind], created)
	}
	for _, kind := range []string{"api function", "webhook", "schema"} {
		created := byKind[kind]
		if len(created) == 0 {
			continue
		}
		printOk(cmd.OutOrStdout(), fmt.Sprintf("trained %ss:", kind))
		for i, item := range created {
			fmt.Fprintf(cmd.OutOrStdout(), "  %d. %s - %s\n", i+1, item.DisplayName(), item.ID)
		}
	}
	failedByKind := map[string][]model.FailedResource{}
	for _, failed := range result.Failed {
		failedByKind[failed.Kind] = append(failedByKind[failed.Kind], failed)
	}
	for _, kind := range []string{"api function", "webhook", "schema"} {
		failed := failedByKind[kind]
		if len(failed) == 0 {
			continue
		}
		printError(cmd.ErrOrStderr(), fmt.Sprintf("failed to train %ss:", kind))
		for _, item := range failed {
			fmt.Fprintf(cmd.ErrOrStderr(), "  %s with context %q and name %q at index %d - %s\n", firstLetterUpper(kind), item.Context, item.Name, item.Index, item.Reason)
		}
	}
}

func generateAfterTrain(cmd *cobra.Command, g Globals, cfg config.Resolved) {
	if _, _, err := cfg.RequireCredentials(); err != nil {
		printWarning(cmd.ErrOrStderr(), "SDK generate skipped: "+err.Error())
		return
	}
	g.NonInteractive = true
	adapter, err := resolveAdapter(g, cfg)
	if err != nil {
		printWarning(cmd.ErrOrStderr(), "SDK generate skipped: "+err.Error()+"; run polyapi generate after installing an SDK")
		return
	}
	if !g.Quiet {
		fmt.Fprintln(cmd.OutOrStdout(), infoText("Re-generating Poly library..."))
	}
	httpClient, err := api.FromConfig(cfg)
	if err != nil {
		printWarning(cmd.ErrOrStderr(), "SDK generate skipped: "+err.Error())
		return
	}
	if err := generateWithClient(cmd, g, adapter, httpClient, nil, nil, nil, false); err != nil {
		printWarning(cmd.ErrOrStderr(), "polyapi generate failed: "+err.Error())
	}
}

func parseRenameFlags(cmd *cobra.Command) ([]model.Rename, error) {
	raw, _ := cmd.Flags().GetStringArray("rename")
	out := make([]model.Rename, 0, len(raw))
	for _, item := range raw {
		pair, err := model.ParseRename(item)
		if err != nil {
			return nil, failUsage(err.Error())
		}
		out = append(out, pair)
	}
	return out, nil
}

func modelClient(cmd *cobra.Command) (*api.HTTPClient, config.Resolved, Globals, error) {
	g := globalsFrom(cmd)
	cfg, err := loadFromGlobal(g)
	if err != nil {
		return nil, cfg, g, fail(err)
	}
	if _, _, err := cfg.RequireCredentials(); err != nil {
		return nil, cfg, g, fail(err)
	}
	client, err := api.FromConfig(cfg)
	if err != nil {
		return nil, cfg, g, fail(err)
	}
	return client, cfg, g, nil
}

func failModel(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, model.ErrNotExist) || errors.Is(err, model.ErrDestinationExists) {
		return failUsage(err.Error())
	}
	var dup *model.DuplicateError
	if errors.As(err, &dup) {
		return fail(dup)
	}
	var ae *api.Error
	if errors.As(err, &ae) {
		return &ExitError{Code: ae.ExitCode(), Msg: api.UserMessage(ae)}
	}
	return fail(err)
}

func firstLetterUpper(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
