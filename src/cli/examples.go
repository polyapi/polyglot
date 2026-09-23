package cli

import "strings"

// ex is one Fang examples-block entry: a "# comment" line plus a command line.
type ex struct {
	comment string
	command string
}

// examples builds a Cobra Example string Fang can syntax-highlight.
// Comments must start with "# " so Fang styles them as comments; the
// program name "polyapi" is styled as the program, subcommands as
// commands, flags as flags, and the rest as arguments.
func examples(items ...ex) string {
	var b strings.Builder
	for i, item := range items {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("# ")
		b.WriteString(item.comment)
		b.WriteByte('\n')
		b.WriteString(item.command)
		b.WriteByte('\n')
	}
	return b.String()
}
