package config

import "strings"

// Instances is the canonical shorthand → URL map (RFC §5.3).
var Instances = []struct {
	Name string
	URL  string
}{
	{"na1", "https://na1.polyapi.io"},
	{"eu1", "https://eu1.polyapi.io"},
	{"na2", "https://na2.polyapi.io"},
	{"dev", "https://dev.polyapi.io"},
	{"local", "http://localhost:3000"},
}

// ResolveBaseURL expands a shorthand (na1) or accepts a full URL. develop aliases dev.
func ResolveBaseURL(input string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(input), "/")
	if trimmed == "" {
		return "", invalidURL(input)
	}
	if url, ok := lookupShorthand(trimmed); ok {
		return url, nil
	}
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return trimmed, nil
	}
	return "", invalidURL(input)
}

// InstanceName returns the canonical instance name for a URL or shorthand.
func InstanceName(input string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(input), "/")
	if name := canonicalShorthand(trimmed); name != "" {
		return name
	}
	for _, inst := range Instances {
		if inst.URL == trimmed {
			return inst.Name
		}
	}
	return ""
}

func lookupShorthand(name string) (string, bool) {
	canon := canonicalShorthand(name)
	if canon == "" {
		return "", false
	}
	for _, inst := range Instances {
		if inst.Name == canon {
			return inst.URL, true
		}
	}
	return "", false
}

func canonicalShorthand(name string) string {
	switch name {
	case "na1":
		return "na1"
	case "eu1":
		return "eu1"
	case "na2":
		return "na2"
	case "dev", "develop":
		return "dev"
	case "local":
		return "local"
	default:
		return ""
	}
}
