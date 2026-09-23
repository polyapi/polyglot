package api

import "strings"

// Collections are known Poly collection paths used by early resource packs.
var Collections = []string{
	"variables",
	"tables",
	"schemas",
	"functions/ai",
	"functions/api",
	"functions/client",
	"functions/server",
	"webhooks",
	"triggers",
	"jobs",
	"snippets",
	"subscriptions/graphql",
	"applications",
}

// IsSafeResource is true when resource is a relative collection path (no scheme, no ..).
func IsSafeResource(resource string) bool {
	trimmed := strings.TrimLeft(strings.TrimSpace(resource), "/")
	if trimmed == "" {
		return false
	}
	if strings.Contains(trimmed, "://") || strings.HasPrefix(trimmed, `\`) {
		return false
	}
	for _, seg := range strings.Split(trimmed, "/") {
		if seg == "" || seg == "." || seg == ".." || strings.Contains(seg, `\`) {
			return false
		}
	}
	return true
}
