package projinit_test

import (
	"strings"
	"testing"

	"github.com/polyapi/polyglot/src/projinit"
)

func TestParseSourceNone(t *testing.T) {
	for _, in := range []string{"", "none", "empty", "-", "NO"} {
		src, err := projinit.ParseSource(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if src.Kind != projinit.KindNone {
			t.Fatalf("%q kind=%v", in, src.Kind)
		}
	}
}

func TestParseGitHub(t *testing.T) {
	cases := []struct {
		in, repo, ref string
	}{
		{"acme/starter", "acme/starter", "main"},
		{"acme/starter@develop", "acme/starter", "develop"},
		{"https://github.com/acme/starter", "acme/starter", "main"},
		{"https://github.com/acme/starter.git", "acme/starter", "main"},
		{"https://github.com/acme/starter/tree/feature/x", "acme/starter", "feature/x"},
		{"git@github.com:acme/starter.git", "acme/starter", "main"},
		{"github.com/acme/starter#v1.2.3", "acme/starter", "v1.2.3"},
		{"polyapi/poly-glide-template-js", "polyapi/poly-glide-template-js", "main"},
	}
	for _, c := range cases {
		src, ok := projinit.ParseGitHub(c.in)
		if !ok {
			t.Fatalf("ParseGitHub(%q) failed", c.in)
		}
		if src.GitHub != c.repo || src.Ref != c.ref {
			t.Fatalf("%q → %s @ %s, want %s @ %s", c.in, src.GitHub, src.Ref, c.repo, c.ref)
		}
		if !strings.Contains(src.ZipURL(), c.repo) {
			t.Fatalf("zip url %s missing repo", src.ZipURL())
		}
	}
}

func TestParseGitHubRejects(t *testing.T) {
	for _, in := range []string{"not-a-repo", "https://gitlab.com/acme/starter", "https://example.com/x.zip"} {
		if _, ok := projinit.ParseGitHub(in); ok {
			t.Fatalf("expected reject %q", in)
		}
	}
}

func TestParseSourceOfficialAndZip(t *testing.T) {
	src, err := projinit.ParseSource("polyapi/poly-glide-template-py")
	if err != nil {
		t.Fatal(err)
	}
	if src.Kind != projinit.KindGitHub || src.GitHub != "polyapi/poly-glide-template-py" {
		t.Fatalf("%+v", src)
	}
	src, err = projinit.ParseSource("https://example.com/t.zip")
	if err != nil {
		t.Fatal(err)
	}
	if src.Kind != projinit.KindZip || src.URL != "https://example.com/t.zip" {
		t.Fatalf("%+v", src)
	}
}

func TestParseCatalogue(t *testing.T) {
	raw := map[string]any{
		"value": map[string]any{
			"templates": []any{
				map[string]any{
					"name":       "Hotel",
					"typescript": "https://example.com/hotel-ts.zip",
					"python":     "https://example.com/hotel-py.zip",
				},
				map[string]any{"name": "TS only", "typescript": "https://example.com/ts.zip"},
			},
		},
	}
	cat, err := projinit.ParseCatalogue(raw)
	if err != nil {
		t.Fatal(err)
	}
	ts := cat.SourcesForLang("typescript")
	if len(ts) != 2 || ts[0].Label != "Hotel" {
		t.Fatalf("ts=%+v", ts)
	}
	py := cat.SourcesForLang("python")
	if len(py) != 1 || py[0].URL != "https://example.com/hotel-py.zip" {
		t.Fatalf("py=%+v", py)
	}
}

func TestParseCatalogueJSONString(t *testing.T) {
	raw := map[string]any{
		"value": `{"templates":[{"name":"A","typescript":"https://x/a.zip"}]}`,
	}
	cat, err := projinit.ParseCatalogue(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Templates) != 1 || cat.Templates[0].Name != "A" {
		t.Fatalf("%+v", cat)
	}
}
