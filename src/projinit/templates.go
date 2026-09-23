package projinit

import (
	"encoding/json"
	"strings"
)

// CatalogueTemplate is one entry from the ProjectTemplates config variable.
type CatalogueTemplate struct {
	Name       string `json:"name"`
	TypeScript string `json:"typescript,omitempty"`
	Python     string `json:"python,omitempty"`
	Java       string `json:"java,omitempty"`
}

// Catalogue is the ProjectTemplates config-variable value.
type Catalogue struct {
	Templates []CatalogueTemplate `json:"templates"`
}

// ParseCatalogue reads GET …/config-variables/ProjectTemplates (object or wrapped value).
func ParseCatalogue(raw any) (Catalogue, error) {
	if raw == nil {
		return Catalogue{}, nil
	}
	switch v := raw.(type) {
	case string:
		return parseCatalogueJSON([]byte(strings.TrimSpace(v)))
	case []byte:
		return parseCatalogueJSON(v)
	case map[string]any:
		if inner, ok := v["value"]; ok {
			return ParseCatalogue(inner)
		}
		if _, ok := v["templates"]; ok {
			b, err := json.Marshal(v)
			if err != nil {
				return Catalogue{}, fail("invalid ProjectTemplates payload")
			}
			return parseCatalogueJSON(b)
		}
		return Catalogue{}, nil
	default:
		b, err := json.Marshal(raw)
		if err != nil {
			return Catalogue{}, fail("invalid ProjectTemplates payload")
		}
		return parseCatalogueJSON(b)
	}
}

func parseCatalogueJSON(raw []byte) (Catalogue, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return Catalogue{}, nil
	}
	var wrapped struct {
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && len(wrapped.Value) > 0 && string(wrapped.Value) != "null" {
		return parseCatalogueJSON(wrapped.Value)
	}
	var cat Catalogue
	if err := json.Unmarshal(raw, &cat); err != nil {
		// value might be a JSON string of the catalogue
		var asString string
		if err2 := json.Unmarshal(raw, &asString); err2 == nil {
			return parseCatalogueJSON([]byte(asString))
		}
		return Catalogue{}, fail("invalid ProjectTemplates JSON")
	}
	return cat, nil
}

// SourcesForLang returns zip sources from the catalogue that have a URL for lang.
func (c Catalogue) SourcesForLang(lang string) []Source {
	lang = strings.ToLower(strings.TrimSpace(lang))
	var out []Source
	for _, t := range c.Templates {
		url := t.URLFor(lang)
		if url == "" {
			continue
		}
		name := strings.TrimSpace(t.Name)
		if name == "" {
			name = url
		}
		out = append(out, Source{Kind: KindZip, Label: name, URL: url})
	}
	return out
}

// URLFor is the zip URL for typescript, python, or java.
func (t CatalogueTemplate) URLFor(lang string) string {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "python", "py":
		return strings.TrimSpace(t.Python)
	case "java":
		return strings.TrimSpace(t.Java)
	default:
		return strings.TrimSpace(t.TypeScript)
	}
}
