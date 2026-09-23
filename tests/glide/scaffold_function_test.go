package glide_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/polyapi/polyglot/src/glide"
)

func TestValidFunctionName(t *testing.T) {
	ok := []struct{ lang, name string }{
		{"typescript", "helloWorld"},
		{"typescript", "hello_world"},
		{"typescript", "_hidden"},
		{"typescript", "$foo"},
		{"python", "hello_world"},
		{"python", "helloWorld"},
		{"python", "_hidden"},
	}
	for _, c := range ok {
		if err := glide.ValidFunctionName(c.lang, c.name); err != nil {
			t.Errorf("%s %q: %v", c.lang, c.name, err)
		}
	}
	bad := []struct{ lang, name string }{
		{"typescript", "hello-world"},
		{"typescript", "hello world"},
		{"typescript", "123abc"},
		{"typescript", "class"},
		{"typescript", "await"},
		{"typescript", "function"},
		{"python", "hello-world"},
		{"python", "123abc"},
		{"python", "class"},
		{"python", "def"},
		{"python", "$foo"},
		{"python", "True"},
	}
	for _, c := range bad {
		if err := glide.ValidFunctionName(c.lang, c.name); err == nil {
			t.Errorf("%s %q: expected error", c.lang, c.name)
		}
	}
}

func TestScaffoldFunctionCodeTypeScript(t *testing.T) {
	body, err := glide.ScaffoldFunctionCode(glide.TypeServerFunction, "helloWorld", "billing", "typescript")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"import { PolyServerFunction } from 'polyapi';",
		"const polyConfig: PolyServerFunction = {",
		"name: 'helloWorld'",
		"context: 'billing'",
		"logsEnabled: true",
		"function helloWorld(): string",
	} {
		if !strings.Contains(body, s) {
			t.Errorf("missing %q\n%s", s, body)
		}
	}
	if strings.Count(body, "helloWorld") < 2 {
		t.Fatalf("name must appear on polyConfig and the function:\n%s", body)
	}
}

func TestScaffoldFunctionCodePythonClient(t *testing.T) {
	body, err := glide.ScaffoldFunctionCode(glide.TypeClientFunction, "hello_world", "myContext", "python")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"from polyapi.typedefs import PolyClientFunction",
		"polyConfig: PolyClientFunction = {",
		"'name': 'hello_world'",
		"'context': 'myContext'",
		"def hello_world(first_name: str) -> str:",
	} {
		if !strings.Contains(body, s) {
			t.Errorf("missing %q\n%s", s, body)
		}
	}
	if strings.Contains(body, "logsEnabled") {
		t.Fatalf("client must not set logsEnabled:\n%s", body)
	}
}

func TestScaffoldFunctionCodeRejectsBadName(t *testing.T) {
	if _, err := glide.ScaffoldFunctionCode(glide.TypeServerFunction, "hello-world", "x", "typescript"); err == nil {
		t.Fatal("expected error")
	}
}

func TestScaffoldFunctionCodeIsDiscoverable(t *testing.T) {
	root := t.TempDir()
	body, err := glide.ScaffoldFunctionCode(glide.TypeServerFunction, "helloWorld", "billing", "typescript")
	if err != nil {
		t.Fatal(err)
	}
	rel := glide.FunctionCodeRel(glide.TypeServerFunction, "billing", "helloWorld", ".ts")
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	found, err := glide.Find(root, glide.MergeScope(nil, nil, nil, nil, nil), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Rel != rel {
		t.Fatalf("%+v", found)
	}
	if found[0].Kind != glide.KindCode || found[0].Guess != glide.TypeServerFunction {
		t.Fatalf("%+v", found[0])
	}
}
