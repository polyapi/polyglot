// Package cli is the Cobra command tree for polyapi.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/fang"
	"github.com/polyapi/polyglot/src/exitcode"
	"github.com/polyapi/polyglot/src/version"
	"github.com/spf13/cobra"
)

const (
	exitOK    = exitcode.OK
	exitFail  = exitcode.Failure
	exitUsage = exitcode.Usage
)

// ExitError is a command failure with a process exit code.
type ExitError struct {
	Code int
	Msg  string
}

func (e *ExitError) Error() string { return e.Msg }

func stub(path string) func(*cobra.Command, []string) error {
	return func(*cobra.Command, []string) error {
		return &ExitError{Code: exitFail, Msg: fmt.Sprintf("`polyapi %s` is not implemented yet", path)}
	}
}

// NewRoot builds the polyapi command tree.
func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:     "polyapi",
		Version: version.Version,
		Short:   "A single global CLI for PolyAPI.",
		Long:    "A single global CLI for PolyAPI. Use `polyapi` with no commands to enter interactive mode.",
		Example: examples(
			ex{"Open the interactive command browser:", "polyapi"},
			ex{"Log in to an instance:", "polyapi auth login na1"},
			ex{"Generate the local SDK:", "polyapi generate"},
			ex{"Dry-run a deploy:", "polyapi deploy plan --env prod"},
			ex{"Apply local deployables:", "polyapi deploy push --env prod"},
		),
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			maybePrintBanner(cmd)
			enableRequestWait(cmd)
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if shouldEnterTUI(cmd) {
				return runInteractive(cmd)
			}
			return cmd.Help()
		},
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.SetVersionTemplate(formatBuild())

	f := root.PersistentFlags()
	f.String("env", "", "Named environment from [environments.*] (for example prod)")
	f.String("base-url", "", "Instance base URL")
	f.String("api-key", "", "API key (never printed)")
	f.Var(&langValue{}, "lang", "Language adapter override (typescript|python|java)")
	f.String("adapter", "", "Adapter launch command override (for example: node ./adapter.js)")
	f.String("poly-path", ".poly", "Path to the project .poly directory")
	f.Bool("non-interactive", false, "Do not prompt; fail instead")
	f.BoolP("yes", "y", false, "Accept defaults / skip confirmation prompts")
	f.CountP("verbose", "v", "Increase logging verbosity (repeatable)")
	f.BoolP("quiet", "q", false, "Suppress non-error output")
	root.MarkFlagsMutuallyExclusive("verbose", "quiet")

	addMetaCommands(root)
	addGlideCommands(root)
	addResourceCommands(root)
	addModelCommands(root)
	annotateRequiredHelp(root)
	return root
}

// Execute runs the CLI with process args and stdio.
func Execute() int {
	return Run(os.Args[1:], os.Stdout, os.Stderr)
}

// Run executes the command tree with the given args and writers.
func Run(args []string, stdout, stderr io.Writer) int {
	return runIO(args, stdout, stderr, nil)
}

func runIO(args []string, stdout, stderr io.Writer, stdin io.Reader) int {
	defer disableRequestWait()
	cmd := NewRoot()
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	if stdin != nil {
		cmd.SetIn(stdin)
	}
	err := fang.Execute(context.Background(), cmd,
		fang.WithoutVersion(), // -v is verbose; cobra still exposes --version
		fang.WithColorSchemeFunc(polyColorScheme),
	)
	if err == nil {
		return exitOK
	}
	var ex *ExitError
	if errors.As(err, &ex) {
		return ex.Code
	}
	return exitUsage
}

type langValue struct{ value string }

func (l *langValue) String() string { return l.value }

func (l *langValue) Set(s string) error {
	switch strings.ToLower(s) {
	case "typescript", "ts", "javascript", "js":
		l.value = "typescript"
	case "python", "py":
		l.value = "python"
	case "java":
		l.value = "java"
	default:
		return fmt.Errorf(`invalid argument %q for "--lang" (typescript|python|java)`, s)
	}
	return nil
}

func (l *langValue) Type() string { return "language" }
