package config

import (
	"os"
	"path/filepath"
	"strings"
)

// LegacyCreds are credentials found in a language-SDK file.
type LegacyCreds struct {
	APIKey  string
	BaseURL string
	Source  Source
}

// ReadLegacy searches cwd for TS / Python / Java credential files. Does not write them.
func ReadLegacy(projectRoot string) *LegacyCreds {
	if creds := readTypeScript(projectRoot); creds != nil {
		return creds
	}
	if creds := readPython(projectRoot); creds != nil {
		return creds
	}
	return readJava(projectRoot)
}

func readTypeScript(root string) *LegacyCreds {
	path := filepath.Join(root, "node_modules", ".poly", ".config.env")
	text, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	apiKey, baseURL := parseDotenv(string(text))
	if apiKey == "" && baseURL == "" {
		return nil
	}
	return &LegacyCreds{APIKey: apiKey, BaseURL: baseURL, Source: SourceLegacyTS}
}

func readPython(root string) *LegacyCreds {
	path := filepath.Join(root, "polyapi", ".config.env")
	text, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	apiKey, baseURL := parseINIPolyapi(string(text))
	if apiKey == "" && baseURL == "" {
		return nil
	}
	return &LegacyCreds{APIKey: apiKey, BaseURL: baseURL, Source: SourceLegacyPython}
}

func readJava(root string) *LegacyCreds {
	path := filepath.Join(root, ".mvn", "settings.xml")
	text, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	apiKey := xmlTag(string(text), "poly.apiKey")
	baseURL := xmlTag(string(text), "poly.hostUrl")
	if apiKey == "" && baseURL == "" {
		return nil
	}
	return &LegacyCreds{APIKey: apiKey, BaseURL: baseURL, Source: SourceLegacyJava}
}

func parseDotenv(text string) (apiKey, baseURL string) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value := unquote(strings.TrimSpace(v))
		switch strings.TrimSpace(k) {
		case "POLY_API_KEY":
			apiKey = value
		case "POLY_API_BASE_URL":
			baseURL = value
		}
	}
	return apiKey, baseURL
}

func parseINIPolyapi(text string) (apiKey, baseURL string) {
	inSection := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			inSection = strings.EqualFold(line, "[polyapi]")
			continue
		}
		if !inSection {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "poly_api_key":
			apiKey = unquote(strings.TrimSpace(v))
		case "poly_api_base_url":
			baseURL = unquote(strings.TrimSpace(v))
		}
	}
	return apiKey, baseURL
}

func xmlTag(text, tag string) string {
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	start := strings.Index(text, open)
	if start < 0 {
		return ""
	}
	start += len(open)
	endRel := strings.Index(text[start:], close)
	if endRel < 0 {
		return ""
	}
	value := strings.TrimSpace(text[start : start+endRel])
	if value == "" || strings.HasPrefix(value, "POLYAPI_") {
		return ""
	}
	return value
}

func unquote(value string) string {
	if len(value) >= 2 {
		first, last := value[0], value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}
