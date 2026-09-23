package glide

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var tokenRe = regexp.MustCompile(`\{\{\s*([^{}\s]+)\s*\}\}`)

// Token is an unresolved {{NAME}} placeholder.
type Token struct {
	Name     string
	JSONPath string
	File     string
}

// ContentHash is a stable SHA-256 of canonical JSON.
func ContentHash(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// LoadDotEnv reads KEY=VALUE pairs from path (if present). Real env wins later.
func LoadDotEnv(path string) map[string]string {
	out := map[string]string{}
	raw, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		v = strings.Trim(v, `"'`)
		if k != "" {
			out[k] = v
		}
	}
	return out
}

// MergeEnv is .env values then the process environment (process wins).
func MergeEnv(projectRoot string) map[string]string {
	out := LoadDotEnv(filepath.Join(projectRoot, ".env"))
	for _, kv := range os.Environ() {
		k, v, ok := strings.Cut(kv, "=")
		if ok && k != "" {
			out[k] = v
		}
	}
	return out
}

func substitute(v any, env map[string]string, file, jsonPath string) (any, []Token) {
	var missing []Token
	out := walkSub(v, env, file, jsonPath, &missing)
	return out, missing
}

func walkSub(v any, env map[string]string, file, jsonPath string, missing *[]Token) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			path := jsonPath + "." + k
			if jsonPath == "$" {
				path = "$." + k
			}
			out[k] = walkSub(val, env, file, path, missing)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = walkSub(val, env, file, jsonPath+"["+strconv.Itoa(i)+"]", missing)
		}
		return out
	case string:
		return subString(t, env, file, jsonPath, missing)
	default:
		return v
	}
}

func subString(s string, env map[string]string, file, jsonPath string, missing *[]Token) string {
	return tokenRe.ReplaceAllStringFunc(s, func(m string) string {
		parts := tokenRe.FindStringSubmatch(m)
		if len(parts) < 2 {
			return m
		}
		name := parts[1]
		if val, ok := env[name]; ok {
			return val
		}
		*missing = append(*missing, Token{Name: name, JSONPath: jsonPath, File: file})
		return m
	})
}

func tokensIn(v any) []string {
	var names []string
	seen := map[string]bool{}
	var walk func(any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			for _, val := range t {
				walk(val)
			}
		case []any:
			for _, val := range t {
				walk(val)
			}
		case string:
			for _, m := range tokenRe.FindAllStringSubmatch(t, -1) {
				if len(m) > 1 && !seen[m[1]] {
					seen[m[1]] = true
					names = append(names, m[1])
				}
			}
		}
	}
	walk(v)
	return names
}
