package cli

import (
	"fmt"
	"io"

	"github.com/polyapi/polyglot/src/delegate"
	"github.com/polyapi/polyglot/src/version"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func addMetaCommands(root *cobra.Command) {
	auth := &cobra.Command{
		Use:   "auth",
		Short: "Credentials and identity",
		Example: examples(
			ex{"Log in with an instance shorthand:", "polyapi auth login na1"},
			ex{"Show the current identity:", "polyapi auth whoami"},
		),
	}

	login := &cobra.Command{
		Use:   "login [base-url] [api-key]",
		Short: "Log in and store credentials",
		Args:  cobra.MaximumNArgs(2),
		RunE:  runLogin,
		Example: examples(
			ex{"Prompt for URL and key:", "polyapi auth login"},
			ex{"Instance shorthand:", "polyapi auth login na1"},
			ex{"Pass a key without prompting:", "polyapi auth login na1 --api-key $POLY_API_KEY --non-interactive"},
			ex{"Full URL with flags:", "polyapi auth login --base-url https://eu1.polyapi.io --api-key $POLY_API_KEY --non-interactive"},
		),
	}
	login.Flags().String("api-version", "1", "API version to record in local config")

	logout := &cobra.Command{
		Use:   "logout",
		Short: "Forget stored credentials",
		Args:  cobra.NoArgs,
		RunE:  runLogout,
		Example: examples(
			ex{"Clear stored API keys (URLs are kept):", "polyapi auth logout"},
		),
	}

	whoami := &cobra.Command{
		Use:   "whoami",
		Short: "Print the current identity (redacted)",
		Args:  cobra.NoArgs,
		RunE:  runWhoami,
		Example: examples(
			ex{"Show instance and redacted key:", "polyapi auth whoami"},
		),
	}
	auth.AddCommand(login, logout, whoami)

	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Non-secret project and user settings (secrets redacted)",
		Args:  cobra.NoArgs,
		RunE:  runConfigShow,
		Example: examples(
			ex{"Print resolved config with secrets redacted:", "polyapi config show"},
			ex{"Read one setting:", "polyapi config get language"},
			ex{"Set the project language:", "polyapi config set language typescript"},
		),
	}
	configShow := &cobra.Command{
		Use:   "show",
		Short: "Print resolved configuration (secrets redacted)",
		Args:  cobra.NoArgs,
		RunE:  runConfigShow,
		Example: examples(
			ex{"Print resolved config with secrets redacted:", "polyapi config show"},
		),
	}
	configGet := &cobra.Command{
		Use:   "get <key>",
		Short: "Print one configuration value (secrets redacted)",
		Args:  cobra.ExactArgs(1),
		RunE:  runConfigGet,
		Example: examples(
			ex{"Read the detected language:", "polyapi config get language"},
			ex{"Read the instance URL:", "polyapi config get base_url"},
		),
	}
	configSet := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a non-secret project setting",
		Args:  cobra.ExactArgs(2),
		RunE:  runConfigSet,
		Example: examples(
			ex{"Pin the project language:", "polyapi config set language python"},
			ex{"Set an adapter launch command:", "polyapi config set adapter \"node node_modules/polyapi/build/adapter.js\""},
		),
	}
	configCmd.AddCommand(configShow, configGet, configSet)

	doctor := &cobra.Command{
		Use:   "doctor",
		Short: "Report binary, protocol, adapter, config, auth, and branch→env health",
		Args:  cobra.NoArgs,
		RunE:  runDoctor,
		Example: examples(
			ex{"Check CLI, adapter, config, auth, and deploy eligibility:", "polyapi doctor"},
			ex{"Skip live HTTP (adapter and config only):", "polyapi doctor --offline"},
			ex{"Include the selected environment in the deploy check:", "polyapi doctor --env prod"},
		),
	}
	doctor.Flags().Bool("offline", false, "Skip live HTTP (do not call GET /auth)")

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the CLI version and build metadata",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			printBuild(cmd.OutOrStdout())
			return nil
		},
		Example: examples(
			ex{"Print semver, commit, Go version, and protocol:", "polyapi version"},
		),
	}

	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "Replace this binary with the latest GitHub release",
		Args:  cobra.NoArgs,
		RunE:  runUpdate,
		Example: examples(
			ex{"Check whether a newer release exists:", "polyapi update --check"},
			ex{"Download and replace this binary:", "polyapi update"},
		),
	}
	updateCmd.Flags().Bool("check", false, "Only check for a newer release (fail on network errors)")

	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a new Poly project or adopt an existing repo",
		Long:  initLong,
		Args:  cobra.NoArgs,
		RunE:  runInit,
		Example: examples(
			ex{"Empty TypeScript project in this directory:", "polyapi init --lang typescript --template none --yes"},
			ex{"Official Glide Python template:", "polyapi init --lang python --template polyapi/poly-glide-template-py --yes"},
			ex{"Public GitHub repository:", "polyapi init --template https://github.com/acme/poly-starter --lang typescript"},
			ex{"Inject Poly into the current directory:", "polyapi init --existing --name billing --template none --yes"},
			ex{"Map the main branch to prod:", "polyapi init --existing --env-map main=prod --template none --yes"},
		),
	}
	initCmd.Flags().String("template", "", "none, official name, GitHub owner/repo, or a zip URL/path")
	initCmd.Flags().String("provider", "github", "Git host for CI (github|gitlab|other)")
	initCmd.Flags().String("name", "", "Project / package name")
	initCmd.Flags().Bool("existing", false, "Inject Poly into the current directory instead of creating a new project")
	initCmd.Flags().StringArray("env-map", nil, "Branch→environment mapping, for example `main=prod`")
	initCmd.Flags().Bool("force", false, "Overwrite existing files from the template")
	initCmd.Flags().Bool("no-deps", false, "Skip package install and Python virtualenv")
	initCmd.Flags().Bool("no-runtime", false, "Skip Node.js / Python detection and install")
	initCmd.Flags().String("venv", ".venv", "Python virtualenv directory")

	generate := &cobra.Command{
		Use:   "generate",
		Short: "Generate the local language SDK",
		Args:  cobra.NoArgs,
		RunE:  runGenerate,
		Example: examples(
			ex{"Generate the full local SDK:", "polyapi generate"},
			ex{"Limit to a context:", "polyapi generate --contexts billing"},
			ex{"Filter by name and ID:", "polyapi generate --names apiKey --ids abc,def --no-types"},
			ex{"Force the TypeScript adapter:", "polyapi generate --lang typescript"},
		),
	}
	generate.Flags().StringSlice("contexts", nil, "Contexts to generate (comma-separated)")
	generate.Flags().StringSlice("names", nil, "Resource names to generate (comma-separated)")
	generate.Flags().StringSlice("ids", nil, "Resource IDs to generate (comma-separated)")
	generate.Flags().Bool("no-types", false, "Skip generating type definitions")
	generate.Flags().SetNormalizeFunc(func(_ *pflag.FlagSet, name string) pflag.NormalizedName {
		if name == "function-ids" {
			return "ids"
		}
		return pflag.NormalizedName(name)
	})

	clear := &cobra.Command{
		Use:   "clear",
		Short: "Clear generated SDK sources",
		Args:  cobra.NoArgs,
		RunE:  runClear,
		Example: examples(
			ex{"Remove generated SDK sources:", "polyapi clear"},
		),
	}

	root.AddCommand(auth, configCmd, doctor, versionCmd, updateCmd, initCmd, generate, clear)
}

const initLong = `Scaffold a Poly project in this directory, or adopt an existing Node/Python app.

First-time setup can unpack a template: tenant ProjectTemplates from the API,
an official Glide repo (polyapi/poly-glide-template-js or -py), a public GitHub
repository, or a zip URL. The archive is extracted into the current directory,
or into the nearest project root when package.json or a Python requirements
file is detected.

Python creates a virtualenv (.venv by default) and installs dependencies
inside it. TypeScript runs npm/yarn/pnpm/bun install. If Node.js or Python is
missing, init asks to install it (Homebrew, winget, or the system package
manager). Declining prints install links and exits.

Writes .poly/config.toml (language, deploy.targets, environments), ensures
.poly/ is gitignored, installs a prepare commit hook, and writes a CI
workflow for --provider github or gitlab.`

func formatBuild() string {
	info := version.Current()
	return fmt.Sprintf(
		"polyapi %s\n  commit    %s\n  built     %s\n  go        %s\n  platform  %s\n  protocol  %d\n",
		info.Version,
		info.CommitDisplay(),
		info.DateDisplay(),
		info.Go,
		info.Platform(),
		delegate.Protocol,
	)
}

func printBuild(w io.Writer) {
	info := version.Current()
	fmt.Fprintln(w, header(fmt.Sprintf("polyapi %s", info.Version)))
	printMetaField(w, "commit", info.CommitDisplay())
	printMetaField(w, "built", info.DateDisplay())
	printMetaField(w, "go", info.Go)
	printMetaField(w, "platform", info.Platform())
	printMetaField(w, "protocol", fmt.Sprintf("%d", delegate.Protocol))
}

func printMetaField(w io.Writer, name, value string) {
	fmt.Fprintf(w, "  %s  %s\n", infoText(fmt.Sprintf("%-9s", name)), value)
}
