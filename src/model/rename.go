package model

import (
	"fmt"
	"regexp"
	"strings"
)

// Rename is a whole-word replacement applied to the generated JSON text.
type Rename struct {
	From string
	To   string
}

// ParseRename parses `old:new` (split on the first colon).
func ParseRename(raw string) (Rename, error) {
	from, to, ok := strings.Cut(raw, ":")
	if !ok || from == "" || to == "" {
		return Rename{}, fmt.Errorf("invalid rename mapping %q; use old:new", raw)
	}
	return Rename{From: from, To: to}, nil
}

// ApplyRename replaces whole-word From tokens in contents (TS `--rename` semantics).
func ApplyRename(contents string, pairs []Rename) string {
	for _, pair := range pairs {
		if pair.From == "" {
			continue
		}
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(pair.From) + `\b`)
		contents = re.ReplaceAllString(contents, pair.To)
	}
	return contents
}
