package cli

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func annotateRequiredHelp(cmd *cobra.Command) {
	annotateRequiredHelpFlags(cmd)
	for _, child := range cmd.Commands() {
		annotateRequiredHelp(child)
	}
}

func annotateRequiredHelpFlags(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		extra := requiredHelpNote(f)
		if extra == "" || strings.Contains(f.Usage, extra) {
			return
		}
		if strings.TrimSpace(f.Usage) == "" {
			f.Usage = extra
			return
		}
		f.Usage = strings.TrimRight(f.Usage, ".") + " " + extra
	})
}

func requiredHelpNote(f *pflag.Flag) string {
	if flagMarkedRequired(f) {
		return "(required)"
	}
	groups := f.Annotations[cobraOneRequiredAnnotation]
	if len(groups) == 0 {
		return ""
	}
	names := strings.Fields(groups[0])
	if len(names) == 0 {
		return "(required)"
	}
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, "--"+name)
	}
	return "(required: " + strings.Join(parts, " or ") + ")"
}
