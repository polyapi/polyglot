package glide

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/polyapi/polyglot/src/delegate"
)

// polyConfigAssign is a language-light binding, not an AST.
// Matches `export const polyConfig =`, `polyConfig: PolyVariable =`, `polyConfig =`.
// A bare mention (e.g. `"export": "polyConfig"` in a fixture adapter) is not enough.
var polyConfigAssign = regexp.MustCompile(`(?m)(?:export\s+)?(?:const|let|var)\s+polyConfig\b|\bpolyConfig\s*(?::[^=\n]*)?=`)

func looksLikePolyConfig(src string) bool {
	return polyConfigAssign.MatchString(src)
}

// Dialect is the tiny language table the host is allowed to know.
type Dialect struct {
	Language    string
	Extensions  []string
	LineComment string
	Marker      string
}

// TypeScriptDialect is TS/JS source.
func TypeScriptDialect() Dialect {
	return Dialect{
		Language:    "typescript",
		Extensions:  []string{".ts", ".tsx", ".js", ".jsx", ".mts", ".cts", ".mjs", ".cjs"},
		LineComment: "//",
		Marker:      "polyConfig",
	}
}

// PythonDialect is Python source.
func PythonDialect() Dialect {
	return Dialect{
		Language:    "python",
		Extensions:  []string{".py"},
		LineComment: "#",
		Marker:      "polyConfig",
	}
}

// JSONDialect is JSON/JSONC artifacts (Go parses these).
func JSONDialect() Dialect {
	return Dialect{
		Language:    "json",
		Extensions:  []string{".json", ".jsonc"},
		LineComment: "//",
	}
}

func dialectForLang(lang delegate.Language) Dialect {
	switch lang {
	case delegate.LangPython:
		return PythonDialect()
	default:
		return TypeScriptDialect()
	}
}

func dialectForExt(ext string) (Dialect, bool) {
	ext = strings.ToLower(ext)
	for _, d := range []Dialect{TypeScriptDialect(), PythonDialect(), JSONDialect()} {
		for _, e := range d.Extensions {
			if e == ext {
				return d, true
			}
		}
	}
	return Dialect{}, false
}

func isCodeExt(ext string) bool {
	d, ok := dialectForExt(ext)
	return ok && d.Language != "json"
}

func commentPrefix(path string) string {
	d, ok := dialectForExt(filepath.Ext(path))
	if !ok || d.LineComment == "" {
		return "//"
	}
	return d.LineComment
}
