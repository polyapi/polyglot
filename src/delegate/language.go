package delegate

import "strings"

// Language is a project language that can back a v1 adapter.
type Language string

const (
	LangTypeScript Language = "typescript"
	LangPython     Language = "python"
	LangJava       Language = "java"
)

// ParseLanguage accepts config / heuristic strings (typescript, ts, python, py, java).
func ParseLanguage(s string) (Language, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "typescript", "ts", "javascript", "js":
		return LangTypeScript, true
	case "python", "py":
		return LangPython, true
	case "java":
		return LangJava, true
	default:
		return "", false
	}
}

func (l Language) String() string { return string(l) }

// SDKInstallHint is shown when the adapter binary or SDK module is missing.
func (l Language) SDKInstallHint() string {
	switch l {
	case LangTypeScript:
		return "Install the PolyAPI TypeScript SDK (`npm install polyapi`) or pass --adapter."
	case LangPython:
		return "Install the PolyAPI Python SDK (`pip install polyapi-python`) or pass --adapter."
	case LangJava:
		return "The Java adapter is not implemented yet. Pass --adapter when one exists."
	default:
		return "Install the PolyAPI language SDK in this project, or pass --adapter."
	}
}
