package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/polyapi/polyglot/src/config"
	"github.com/spf13/cobra"
)

func canPrompt(cmd *cobra.Command, g Globals) bool {
	if g.NonInteractive {
		return false
	}
	return isInteractive(cmd.InOrStdin())
}

func promptYes(cmd *cobra.Command, label string, def bool, g Globals) (bool, error) {
	if g.Yes {
		return true, nil
	}
	if !canPrompt(cmd, g) {
		return def, nil
	}
	hint := "y/N"
	if def {
		hint = "Y/n"
	}
	line, err := promptString(cmd, label+" ["+hint+"]", "", g)
	if err != nil {
		return false, err
	}
	if line == "" {
		return def, nil
	}
	switch strings.ToLower(line) {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return def, nil
	}
}

func promptChoice(cmd *cobra.Command, label string, choices []string, def int, g Globals) (int, error) {
	if len(choices) == 0 {
		return -1, failUsage("no choices")
	}
	if def < 0 || def >= len(choices) {
		def = 0
	}
	if g.Yes {
		return def, nil
	}
	if !canPrompt(cmd, g) {
		return def, nil
	}
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, label)
	for i, c := range choices {
		fmt.Fprintf(out, "  %d) %s\n", i+1, c)
	}
	line, err := promptString(cmd, "Choice", strconv.Itoa(def+1), g)
	if err != nil {
		return 0, err
	}
	if line == "" {
		return def, nil
	}
	n, err := strconv.Atoi(line)
	if err != nil || n < 1 || n > len(choices) {
		return 0, &config.Error{Code: 2, Msg: fmt.Sprintf("choice must be 1–%d", len(choices))}
	}
	return n - 1, nil
}
