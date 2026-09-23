package glide

import (
	"fmt"
	"path/filepath"
	"strings"
)

// FunctionCodeRel is the default source path for a server or client function.
// Context dots become directories: billing.orders → src/billing/orders/server/<name>.ts
func FunctionCodeRel(typ, context, name, ext string) string {
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	dir := ArtifactDir(typ)
	ctxPath := strings.ReplaceAll(strings.TrimSpace(context), ".", string(filepath.Separator))
	if ctxPath == "" {
		return filepath.ToSlash(filepath.Join("src", dir, name+ext))
	}
	return filepath.ToSlash(filepath.Join("src", ctxPath, dir, name+ext))
}

// FunctionCodeExt is .ts or .py for a scaffold language.
func FunctionCodeExt(lang string) string {
	if lang == "python" {
		return ".py"
	}
	return ".ts"
}

// ScaffoldFunctionCode returns a TypeScript or Python module with a typed
// polyConfig and a function whose identifier is exactly name. Server and client
// only; api/ai stay JSONC.
func ScaffoldFunctionCode(typ, name, context, lang string) (string, error) {
	if typ != TypeServerFunction && typ != TypeClientFunction {
		return "", fmt.Errorf("code scaffold is only for server and client functions")
	}
	if err := ValidFunctionName(lang, name); err != nil {
		return "", err
	}
	switch lang {
	case "python":
		return scaffoldPythonFunction(typ, name, context), nil
	case "typescript", "javascript", "ts", "js", "":
		return scaffoldTypeScriptFunction(typ, name, context), nil
	default:
		return "", fmt.Errorf("function init supports typescript and python, not %s", lang)
	}
}

// ValidFunctionName reports whether name is a legal identifier in lang and not
// a reserved word. The deployable function, polyConfig.name, and --name must
// be this exact string.
func ValidFunctionName(lang, name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("name must not contain path separators")
	}
	if lang == "python" {
		if !isPythonIdent(name) {
			return fmt.Errorf("name %q is not a valid Python identifier; use letters, digits, and underscores, starting with a letter or underscore", name)
		}
		if pythonKeywords[name] {
			return fmt.Errorf("name %q is a reserved Python keyword", name)
		}
		return nil
	}
	if !isTypeScriptIdent(name) {
		return fmt.Errorf("name %q is not a valid TypeScript identifier; use letters, digits, underscores, and $, starting with a letter, underscore, or $", name)
	}
	if tsKeywords[name] {
		return fmt.Errorf("name %q is a reserved TypeScript keyword", name)
	}
	return nil
}

func isTypeScriptIdent(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if r != '_' && r != '$' && !asciiLetter(r) {
				return false
			}
			continue
		}
		if r != '_' && r != '$' && !asciiLetter(r) && !asciiDigit(r) {
			return false
		}
	}
	return true
}

func isPythonIdent(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if r != '_' && !asciiLetter(r) {
				return false
			}
			continue
		}
		if r != '_' && !asciiLetter(r) && !asciiDigit(r) {
			return false
		}
	}
	return true
}

func asciiLetter(r rune) bool {
	return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z'
}

func asciiDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func scaffoldTypeScriptFunction(typ, name, context string) string {
	typeName := "PolyServerFunction"
	if typ == TypeClientFunction {
		typeName = "PolyClientFunction"
	}
	var b strings.Builder
	b.WriteString("import { " + typeName + " } from 'polyapi';\n\n")
	b.WriteString("const polyConfig: " + typeName + " = {\n")
	b.WriteString("    name: " + jsSingle(name) + ",\n")
	b.WriteString("    context: " + jsSingle(context) + ",\n")
	if typ == TypeServerFunction {
		b.WriteString("    logsEnabled: true,\n")
	}
	b.WriteString("    visibility: 'ENVIRONMENT',\n")
	b.WriteString("};\n\n")
	b.WriteString("function " + name + "(): string {\n")
	b.WriteString("    return 'Hello Poly World!';\n")
	b.WriteString("}\n")
	return b.String()
}

func scaffoldPythonFunction(typ, name, context string) string {
	typeName := "PolyServerFunction"
	if typ == TypeClientFunction {
		typeName = "PolyClientFunction"
	}
	var b strings.Builder
	b.WriteString("from polyapi.typedefs import " + typeName + "\n\n")
	b.WriteString("polyConfig: " + typeName + " = {\n")
	b.WriteString("    'name': " + pySingle(name) + ",\n")
	b.WriteString("    'context': " + pySingle(context) + ",\n")
	if typ == TypeServerFunction {
		b.WriteString("    'logsEnabled': True,\n")
	}
	b.WriteString("    'visibility': 'ENVIRONMENT',\n")
	b.WriteString("}\n\n")
	b.WriteString("def " + name + "(first_name: str) -> str:\n")
	b.WriteString("    return f\"Hello {first_name}! I'm Poly, your helpful AI Assistant.\"\n")
	return b.String()
}

func jsSingle(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return "'" + s + "'"
}

func pySingle(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return "'" + s + "'"
}

var tsKeywords = map[string]bool{
	"await": true, "break": true, "case": true, "catch": true, "class": true,
	"const": true, "continue": true, "debugger": true, "default": true,
	"delete": true, "do": true, "else": true, "enum": true, "export": true,
	"extends": true, "false": true, "finally": true, "for": true,
	"function": true, "if": true, "implements": true, "import": true,
	"in": true, "instanceof": true, "interface": true, "let": true,
	"new": true, "null": true, "package": true, "private": true,
	"protected": true, "public": true, "return": true, "static": true,
	"super": true, "switch": true, "this": true, "throw": true, "true": true,
	"try": true, "typeof": true, "var": true, "void": true, "while": true,
	"with": true, "yield": true,
}

var pythonKeywords = map[string]bool{
	"False": true, "None": true, "True": true, "and": true, "as": true,
	"assert": true, "async": true, "await": true, "break": true, "class": true,
	"continue": true, "def": true, "del": true, "elif": true, "else": true,
	"except": true, "finally": true, "for": true, "from": true, "global": true,
	"if": true, "import": true, "in": true, "is": true, "lambda": true,
	"nonlocal": true, "not": true, "or": true, "pass": true, "raise": true,
	"return": true, "try": true, "while": true, "with": true, "yield": true,
}
