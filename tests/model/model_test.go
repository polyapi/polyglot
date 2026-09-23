package model_test

import (
	"strings"
	"testing"

	"github.com/polyapi/polyglot/src/model"
)

func TestSlugifyTitle(t *testing.T) {
	got := model.Slugify("Fake json placeholder spec")
	if got != "fake-json-placeholder-spec" {
		t.Fatalf("%q", got)
	}
	if model.Slugify("  ") != "" {
		t.Fatalf("empty")
	}
}

func TestParseAndApplyRename(t *testing.T) {
	pair, err := model.ParseRename("foo:bar")
	if err != nil {
		t.Fatal(err)
	}
	in := `{ "name": "foo", "also": "{{foo}}", "skip": "foobaz" }`
	out := model.ApplyRename(in, []model.Rename{pair})
	if !strings.Contains(out, `"name": "bar"`) || !strings.Contains(out, `"{{bar}}"`) {
		t.Fatalf("%s", out)
	}
	if !strings.Contains(out, `"foobaz"`) {
		t.Fatalf("word boundary lost: %s", out)
	}
	if _, err := model.ParseRename("nocolon"); err == nil {
		t.Fatal("expected error")
	}
	phrase, err := model.ParseRename("Old key:New key")
	if err != nil {
		t.Fatal(err)
	}
	got := model.ApplyRename("Old key and other", []model.Rename{phrase})
	if got != "New key and other" {
		t.Fatalf("%q", got)
	}
}

func TestOrderSchemasDependenciesFirst(t *testing.T) {
	schemas := []map[string]any{
		{
			"name":    "C",
			"context": "mews",
			"definition": map[string]any{
				"type": "object",
				"allOf": []any{
					map[string]any{"x-poly-ref": map[string]any{"path": "mews.B"}},
				},
			},
		},
		{
			"name":    "B",
			"context": "mews",
			"definition": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"a": map[string]any{"x-poly-ref": map[string]any{"path": "mews.A"}},
				},
			},
		},
		{
			"name":    "A",
			"context": "mews",
			"definition": map[string]any{
				"type":       "object",
				"properties": map[string]any{"id": map[string]any{"type": "string"}},
			},
		},
	}
	ordered := model.OrderSchemas(schemas)
	var names []string
	for _, s := range ordered {
		names = append(names, s["name"].(string))
	}
	if strings.Join(names, ",") != "A,B,C" {
		t.Fatalf("%v", names)
	}
}

func TestOrderSchemasSkipsPublicNamespaceRefs(t *testing.T) {
	schemas := []map[string]any{
		{
			"name":    "Child",
			"context": "app",
			"definition": map[string]any{
				"properties": map[string]any{
					"ext": map[string]any{
						"x-poly-ref": map[string]any{"path": "std.Base", "publicNamespace": "poly"},
					},
				},
			},
		},
		{
			"name":       "Base",
			"context":    "std",
			"definition": map[string]any{"type": "object"},
		},
	}
	ordered := model.OrderSchemas(schemas)
	if ordered[0]["name"] != "Child" || ordered[1]["name"] != "Base" {
		t.Fatalf("%v %v", ordered[0]["name"], ordered[1]["name"])
	}
}

func TestOrderSchemasCycleKeepsOriginal(t *testing.T) {
	schemas := []map[string]any{
		{
			"name": "A",
			"definition": map[string]any{
				"allOf": []any{map[string]any{"x-poly-ref": map[string]any{"path": "B"}}},
			},
		},
		{
			"name": "B",
			"definition": map[string]any{
				"allOf": []any{map[string]any{"x-poly-ref": map[string]any{"path": "A"}}},
			},
		},
	}
	ordered := model.OrderSchemas(schemas)
	if ordered[0]["name"] != "A" || ordered[1]["name"] != "B" {
		t.Fatalf("%v %v", ordered[0]["name"], ordered[1]["name"])
	}
}

func TestEncodeOmitsTitle(t *testing.T) {
	raw, err := model.Encode(model.SpecInput{
		Title:     "Ignored",
		Functions: []map[string]any{{"name": "createPost"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if strings.Contains(s, "Ignored") || strings.Contains(s, `"title"`) {
		t.Fatalf("%s", s)
	}
	if !strings.Contains(s, `"functions"`) || !strings.Contains(s, `"webhooks"`) || !strings.Contains(s, `"schemas"`) {
		t.Fatalf("%s", s)
	}
}

func TestValidHostURL(t *testing.T) {
	if !model.ValidHostURL("https://api.example.com") {
		t.Fatal("https")
	}
	if model.ValidHostURL("not-a-url") || model.ValidHostURL("ftp://x") {
		t.Fatal("invalid accepted")
	}
}

func TestIsRemotePath(t *testing.T) {
	if !model.IsRemotePath("https://example.com/openapi.yaml") {
		t.Fatal("https")
	}
	if model.IsRemotePath("./openapi.yaml") || model.IsRemotePath("openapi.yaml") {
		t.Fatal("local")
	}
}
