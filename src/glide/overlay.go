package glide

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// NormalizeSnippetLanguage maps API / file-extension aliases onto a canonical
// lowercase language. Empty input stays empty.
func NormalizeSnippetLanguage(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "ts", "tsx", "mts", "cts", "typescript":
		return "typescript"
	case "js", "jsx", "mjs", "cjs", "javascript", "node", "nodejs":
		return "javascript"
	case "py", "python":
		return "python"
	case "json", "jsonc":
		return "json"
	default:
		return s
	}
}

// TargetInitLanguage is the snippet language a local init file of typ must
// match. Server/client functions follow funcLang (typescript by default);
// JSONC artifacts are json.
func TargetInitLanguage(typ, funcLang string) string {
	switch typ {
	case TypeServerFunction, TypeClientFunction:
		if NormalizeSnippetLanguage(funcLang) == "python" {
			return "python"
		}
		return "typescript"
	default:
		return "json"
	}
}

// SnippetLanguageMatches reports whether a fetched snippet's language is
// acceptable for a target init language. typescript and javascript are
// interchangeable; json and jsonc already collapse via NormalizeSnippetLanguage.
func SnippetLanguageMatches(snippetLang, targetLang string) bool {
	s := NormalizeSnippetLanguage(snippetLang)
	t := NormalizeSnippetLanguage(targetLang)
	if s == "" || t == "" {
		return false
	}
	if s == t {
		return true
	}
	return (s == "typescript" && t == "javascript") || (s == "javascript" && t == "typescript")
}

// OverlayJSONCIdentity validates src as a JSON object and overwrites identity
// fields for a local init of typ: name, polyType, and context (omitted for
// jobs, triggers, and applications). Extra keys from the snippet are kept.
func OverlayJSONCIdentity(src, typ, name, context string) (string, error) {
	stripped := strings.TrimSpace(StripJSONC(src))
	if stripped == "" {
		return "", fmt.Errorf("snippet code is empty")
	}
	var raw any
	if err := json.Unmarshal([]byte(stripped), &raw); err != nil {
		return "", fmt.Errorf("snippet code is not valid JSON: %w", err)
	}
	obj, ok := raw.(map[string]any)
	if !ok || obj == nil {
		return "", fmt.Errorf("snippet code must be a JSON object")
	}
	obj["name"] = name
	if typ != "" {
		obj["polyType"] = typ
	}
	switch typ {
	case TypeJob, TypeTrigger, TypeApplication:
		delete(obj, "context")
	default:
		obj["context"] = context
	}
	if typ == TypeApplication {
		if cfg, ok := obj["config"].(map[string]any); ok && cfg != nil {
			cfg["name"] = name
		}
	}
	b, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b) + "\n", nil
}

// OverlayFunctionIdentity overwrites name and context on a polyConfig object
// (language-light, not an AST) and renames a matching function / def. quote
// style in the polyConfig strings is preserved. snippetName is the fallback
// identifier when polyConfig has no name.
func OverlayFunctionIdentity(src, lang, name, context, snippetName string) string {
	oldName := ""
	start, end, ok := polyConfigObjectRange(src)
	if ok {
		block := src[start : end+1]
		oldName = objectStringValue(block, "name")
		block = replaceObjectStringValue(block, "name", name)
		block = replaceObjectStringValue(block, "context", context)
		src = src[:start] + block + src[end+1:]
	}
	if oldName == "" {
		oldName = snippetName
	}
	if oldName == "" || oldName == name {
		return src
	}
	if NormalizeSnippetLanguage(lang) == "python" {
		return replaceIdent(src, `(?m)(def\s+)`+regexp.QuoteMeta(oldName)+`\b`, `${1}`+name)
	}
	return replaceIdent(src, `(?m)((?:export\s+)?(?:async\s+)?function\s+|(?:export\s+)?(?:const|let|var)\s+)`+regexp.QuoteMeta(oldName)+`\b`, `${1}`+name)
}

func replaceIdent(src, pattern, repl string) string {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return src
	}
	return re.ReplaceAllString(src, repl)
}

func polyConfigObjectRange(src string) (int, int, bool) {
	loc := polyConfigAssign.FindStringIndex(src)
	if loc == nil {
		return 0, 0, false
	}
	rest := src[loc[0]:]
	eq := strings.Index(rest, "=")
	if eq < 0 {
		return 0, 0, false
	}
	i := loc[0] + eq + 1
	for i < len(src) {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			i++
		case c == '{':
			end, ok := matchingBrace(src, i)
			return i, end, ok
		default:
			return 0, 0, false
		}
	}
	return 0, 0, false
}

func matchingBrace(src string, open int) (int, bool) {
	if open < 0 || open >= len(src) || src[open] != '{' {
		return 0, false
	}
	depth := 0
	var inStr byte
	esc := false
	for i := open; i < len(src); i++ {
		c := src[i]
		if inStr != 0 {
			if esc {
				esc = false
				continue
			}
			if c == '\\' {
				esc = true
				continue
			}
			if c == inStr {
				inStr = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			inStr = c
			continue
		}
		switch c {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

// objectKeyPats match name/context inside a polyConfig object. RE2 has no
// backreferences, so each quote style is its own pattern. The leading
// delimiter keeps `functionName` from matching `name`.
var objectKeyPats = []objectKeyPat{
	{regexp.MustCompile(`(?m)(^|[,{\s])(name|context)(\s*:\s*)'([^']*)'`), false, `'`},
	{regexp.MustCompile(`(?m)(^|[,{\s])(name|context)(\s*:\s*)"([^"]*)"`), false, `"`},
	{regexp.MustCompile(`(?m)(^|[,{\s])'(name|context)'(\s*:\s*)'([^']*)'`), true, `'`},
	{regexp.MustCompile(`(?m)(^|[,{\s])"(name|context)"(\s*:\s*)"([^"]*)"`), true, `"`},
}

type objectKeyPat struct {
	re        *regexp.Regexp
	quotedKey bool
	quote     string
}

func objectStringValue(block, key string) string {
	for _, p := range objectKeyPats {
		for _, m := range p.re.FindAllStringSubmatch(block, -1) {
			if len(m) >= 5 && m[2] == key {
				return m[4]
			}
		}
	}
	return ""
}

func replaceObjectStringValue(block, key, value string) string {
	for _, p := range objectKeyPats {
		p := p
		block = p.re.ReplaceAllStringFunc(block, func(s string) string {
			m := p.re.FindStringSubmatch(s)
			if len(m) < 5 || m[2] != key {
				return s
			}
			esc := strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), p.quote, `\`+p.quote)
			if p.quotedKey {
				return m[1] + p.quote + key + p.quote + m[3] + p.quote + esc + p.quote
			}
			return m[1] + key + m[3] + p.quote + esc + p.quote
		})
	}
	return block
}
