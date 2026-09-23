package glide_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/polyapi/polyglot/src/glide"
)

func TestNormalizeAndMatchSnippetLanguage(t *testing.T) {
	if glide.NormalizeSnippetLanguage("TS") != "typescript" {
		t.Fatal(glide.NormalizeSnippetLanguage("TS"))
	}
	if glide.NormalizeSnippetLanguage("jsonc") != "json" {
		t.Fatal(glide.NormalizeSnippetLanguage("jsonc"))
	}
	if !glide.SnippetLanguageMatches("javascript", "typescript") {
		t.Fatal("js should match ts")
	}
	if !glide.SnippetLanguageMatches("JSONC", "json") {
		t.Fatal("jsonc should match json")
	}
	if glide.SnippetLanguageMatches("python", "typescript") {
		t.Fatal("python must not match ts")
	}
	if glide.SnippetLanguageMatches("", "json") {
		t.Fatal("empty language must not match")
	}
	if glide.TargetInitLanguage(glide.TypeServerFunction, "python") != "python" {
		t.Fatal(glide.TargetInitLanguage(glide.TypeServerFunction, "python"))
	}
	if glide.TargetInitLanguage(glide.TypeVariable, "python") != "json" {
		t.Fatal("JSONC artifacts are json regardless of --lang")
	}
}

func TestOverlayJSONCIdentityOverwritesNameAndContext(t *testing.T) {
	src := `{
  "name": "template",
  "context": "old",
  "description": "keep me",
  "value": {"k": 1}
}`
	out, err := glide.OverlayJSONCIdentity(src, glide.TypeVariable, "apiKey", "billing")
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(glide.StripJSONC(out)), &obj); err != nil {
		t.Fatal(err)
	}
	if obj["name"] != "apiKey" || obj["context"] != "billing" || obj["polyType"] != "variable" {
		t.Fatalf("%+v", obj)
	}
	if obj["description"] != "keep me" {
		t.Fatalf("lost extra fields: %+v", obj)
	}
}

func TestOverlayJSONCIdentityApplicationAndJob(t *testing.T) {
	src := `{"name":"old","context":"stale","config":{"name":"old","collections":[]}}`
	out, err := glide.OverlayJSONCIdentity(src, glide.TypeApplication, "dashboard", "ignored")
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatal(err)
	}
	if _, ok := obj["context"]; ok {
		t.Fatalf("application must not keep context: %+v", obj)
	}
	if obj["name"] != "dashboard" {
		t.Fatalf("%+v", obj)
	}
	cfg, _ := obj["config"].(map[string]any)
	if cfg["name"] != "dashboard" {
		t.Fatalf("config.name=%v", cfg["name"])
	}

	job, err := glide.OverlayJSONCIdentity(`{"name":"old","context":"nope","enabled":true}`, glide.TypeJob, "nightly", "")
	if err != nil {
		t.Fatal(err)
	}
	var jobObj map[string]any
	if err := json.Unmarshal([]byte(job), &jobObj); err != nil {
		t.Fatal(err)
	}
	if _, ok := jobObj["context"]; ok {
		t.Fatalf("job must not keep context: %+v", jobObj)
	}
}

func TestOverlayJSONCIdentityRejectsNonObject(t *testing.T) {
	if _, err := glide.OverlayJSONCIdentity("[]", glide.TypeVariable, "x", "y"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := glide.OverlayJSONCIdentity("not json", glide.TypeVariable, "x", "y"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := glide.OverlayJSONCIdentity("  ", glide.TypeVariable, "x", "y"); err == nil {
		t.Fatal("expected empty error")
	}
}

func TestOverlayFunctionIdentityTypeScript(t *testing.T) {
	src := `import { PolyServerFunction } from 'polyapi';

const polyConfig: PolyServerFunction = {
    name: 'oldFn',
    context: 'oldCtx',
    visibility: 'ENVIRONMENT',
};

function oldFn(): string {
    return 'hi';
}
`
	got := glide.OverlayFunctionIdentity(src, "typescript", "helloWorld", "billing", "oldFn")
	for _, s := range []string{
		"name: 'helloWorld'",
		"context: 'billing'",
		"function helloWorld(): string",
	} {
		if !strings.Contains(got, s) {
			t.Errorf("missing %q\n%s", s, got)
		}
	}
	if strings.Contains(got, "oldFn") || strings.Contains(got, "oldCtx") {
		t.Fatalf("old identity still present:\n%s", got)
	}
}

func TestOverlayFunctionIdentityPythonAndDoubleQuotes(t *testing.T) {
	src := `from polyapi.typedefs import PolyServerFunction

polyConfig: PolyServerFunction = {
    'name': 'old_fn',
    'context': 'old_ctx',
    'visibility': 'ENVIRONMENT',
}

def old_fn(first_name: str) -> str:
    return first_name
`
	got := glide.OverlayFunctionIdentity(src, "python", "hello_world", "billing", "old_fn")
	for _, s := range []string{
		"'name': 'hello_world'",
		"'context': 'billing'",
		"def hello_world(first_name: str) -> str:",
	} {
		if !strings.Contains(got, s) {
			t.Errorf("missing %q\n%s", s, got)
		}
	}

	ts := "const polyConfig = {\n  name: \"oldFn\",\n  context: \"oldCtx\",\n};\nexport async function oldFn() {}\n"
	got = glide.OverlayFunctionIdentity(ts, "typescript", "n", "c", "oldFn")
	if !strings.Contains(got, `name: "n"`) || !strings.Contains(got, `context: "c"`) {
		t.Fatalf("%s", got)
	}
	if !strings.Contains(got, "export async function n()") {
		t.Fatalf("%s", got)
	}
}

func TestOverlayFunctionIdentityFallsBackToSnippetName(t *testing.T) {
	src := "function template() { return 1; }\n"
	got := glide.OverlayFunctionIdentity(src, "typescript", "helloWorld", "billing", "template")
	if !strings.Contains(got, "function helloWorld()") {
		t.Fatalf("%s", got)
	}
}
