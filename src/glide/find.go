package glide

import (
	"encoding/json"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/polyapi/polyglot/src/config"
	"github.com/polyapi/polyglot/src/delegate"
)

var ignoreDirs = map[string]bool{
	".git": true, "node_modules": true, ".venv": true, "venv": true,
	"dist": true, "build": true, "generated": true, ".poly": true,
	"vendor": true, "__pycache__": true, "coverage": true, ".tox": true,
	"target": true, ".idea": true, ".next": true, ".turbo": true,
	".cache": true,
	"tests":  true, "test": true, "__tests__": true, "testdata": true,
	"fixtures": true, "spec": true, "mocks": true,
}

var skipJSONNames = map[string]bool{
	"package.json": true, "package-lock.json": true, "tsconfig.json": true,
	"jsconfig.json": true, "lerna.json": true, "nx.json": true,
	"turbo.json": true, "project.json": true, "compose.json": true,
}

// Scope is path / type / context filters for one run.
type Scope struct {
	Paths                 []string
	ExcludePaths          []string
	Types                 []string
	Contexts              []string
	ContextPathMap        map[string]string
	ExcludeOrphanContexts []string
}

// MergeScope overlays CLI flags on [deploy.scope].
func MergeScope(file *config.DeployFile, paths, exclude []string, types, contexts []string) Scope {
	s := Scope{}
	if file != nil && file.Scope != nil {
		sc := file.Scope
		s.Paths = append([]string{}, sc.Paths...)
		s.ExcludePaths = append([]string{}, sc.ExcludePaths...)
		s.Types = append([]string{}, sc.Types...)
		s.Contexts = append([]string{}, sc.Contexts...)
		s.ExcludeOrphanContexts = append([]string{}, sc.ExcludeOrphanContexts...)
		if sc.ContextPathMap != nil {
			s.ContextPathMap = sc.ContextPathMap
		}
	}
	s.Paths = append(s.Paths, paths...)
	s.ExcludePaths = append(s.ExcludePaths, exclude...)
	if len(types) > 0 {
		s.Types = types
	}
	if len(contexts) > 0 {
		s.Contexts = append(s.Contexts, contexts...)
	}
	s.Types = ExpandTypes(s.Types)
	return s
}

func (s Scope) allowsType(typ string) bool {
	if len(s.Types) == 0 {
		return true
	}
	for _, t := range s.Types {
		if t == typ {
			return true
		}
	}
	return false
}

func (s Scope) allowsContext(context string) bool {
	if len(s.Contexts) == 0 {
		return true
	}
	for _, p := range s.Contexts {
		if context == p || strings.HasPrefix(context, p+".") {
			return true
		}
	}
	return false
}

func (s Scope) allowsPath(rel string) bool {
	rel = filepath.ToSlash(rel)
	if len(s.Paths) > 0 {
		ok := false
		for _, p := range s.Paths {
			if matchPath(rel, p) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	for _, p := range s.ExcludePaths {
		if matchPath(rel, p) {
			return false
		}
	}
	return true
}

func matchPath(rel, pattern string) bool {
	pattern = filepath.ToSlash(strings.TrimSpace(pattern))
	rel = filepath.ToSlash(rel)
	if pattern == "" {
		return false
	}
	pattern = strings.TrimPrefix(pattern, "./")
	rel = strings.TrimPrefix(rel, "./")
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return rel == prefix || strings.HasPrefix(rel, prefix+"/")
	}
	if ok, _ := path.Match(pattern, rel); ok {
		return true
	}
	base := strings.TrimSuffix(pattern, "/")
	return rel == base || strings.HasPrefix(rel, base+"/")
}

// Find walks root for JSONC artifacts and code files that mention polyConfig.
func Find(root string, scope Scope, lang delegate.Language) ([]Candidate, error) {
	var found []Candidate
	code := dialectForLang(lang)
	err := filepath.WalkDir(root, func(abs string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if ignoreDirs[name] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if !scope.allowsPath(rel) {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(name))
		if ext == ".json" || ext == ".jsonc" {
			if skipJSONNames[name] || strings.HasPrefix(name, "tsconfig") || strings.HasPrefix(name, ".") {
				return nil
			}
			c, ok := jsonCandidate(abs, rel)
			if ok {
				found = append(found, c)
			}
			return nil
		}
		if !isCodeExt(ext) {
			return nil
		}
		if lang != "" {
			allowed := false
			for _, e := range code.Extensions {
				if e == ext {
					allowed = true
					break
				}
			}
			if !allowed {
				return nil
			}
		}
		raw, err := os.ReadFile(abs)
		if err != nil {
			return nil
		}
		if !looksLikePolyConfig(string(raw)) {
			return nil
		}
		dlect, _ := dialectForExt(ext)
		found = append(found, Candidate{
			Rel:     rel,
			Abs:     abs,
			Kind:    KindCode,
			Guess:   guessTypeFromPath(rel),
			Dialect: dlect,
		})
		return nil
	})
	return found, err
}

func jsonCandidate(abs, rel string) (Candidate, bool) {
	raw, err := os.ReadFile(abs)
	if err != nil {
		return Candidate{}, false
	}
	stripped := StripJSONC(string(raw))
	var v any
	if err := json.Unmarshal([]byte(stripped), &v); err != nil {
		if strings.Contains(rel, "artifacts/") {
			return Candidate{Rel: rel, Abs: abs, Kind: KindJSON, Dialect: JSONDialect()}, true
		}
		return Candidate{}, false
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return Candidate{}, false
	}
	_, hasName := obj["name"]
	_, hasContext := obj["context"]
	typ := guessTypeFromMap(obj)
	if typ == "" {
		typ = guessTypeFromPath(rel)
	}
	if !hasName && typ == "" {
		return Candidate{}, false
	}
	if !hasContext && typ == "" && !strings.Contains(rel, "artifacts/") {
		return Candidate{}, false
	}
	return Candidate{
		Rel:     rel,
		Abs:     abs,
		Kind:    KindJSON,
		Guess:   typ,
		Dialect: JSONDialect(),
	}, true
}

func guessTypeFromMap(obj map[string]any) string {
	for _, key := range []string{"polyType", "type", "kind", "artifact_type"} {
		if s, ok := obj[key].(string); ok {
			if t, ok := NormalizeType(s); ok {
				return t
			}
		}
	}
	return ""
}

func guessTypeFromPath(rel string) string {
	rel = filepath.ToSlash(rel)
	switch {
	case strings.Contains(rel, "/vari/") || strings.Contains(rel, "/variables/"):
		return TypeVariable
	case strings.Contains(rel, "/webhooks/"):
		return TypeWebhook
	case strings.Contains(rel, "/jobs/"):
		return TypeJob
	case strings.Contains(rel, "/tabi/") || strings.Contains(rel, "/tables/"):
		return TypeTable
	case strings.Contains(rel, "/triggers/"):
		return TypeTrigger
	case strings.Contains(rel, "/schemas/"):
		return TypeSchema
	case strings.Contains(rel, "/snippets/"):
		return TypeSnippet
	case strings.Contains(rel, "/subscriptions/") || strings.Contains(rel, "/graphql-subscriptions/"):
		return TypeSubscription
	case strings.Contains(rel, "/applications/") || strings.Contains(rel, "/apps/") || strings.Contains(rel, "/canopy/"):
		return TypeApplication
	case strings.Contains(rel, "/server/"):
		return TypeServerFunction
	case strings.Contains(rel, "/client/"):
		return TypeClientFunction
	case strings.Contains(rel, "/ai/"):
		return TypeAIFunction
	case strings.Contains(rel, "/api/"):
		return TypeAPIFunction
	default:
		return ""
	}
}

// StripJSONC removes // and /* */ comments outside of strings.
func StripJSONC(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	inString := false
	i := 0
	for i < len(text) {
		c := text[i]
		if inString {
			b.WriteByte(c)
			if c == '\\' && i+1 < len(text) {
				b.WriteByte(text[i+1])
				i += 2
				continue
			}
			if c == '"' {
				inString = false
			}
			i++
			continue
		}
		if c == '"' {
			inString = true
			b.WriteByte(c)
			i++
			continue
		}
		if c == '/' && i+1 < len(text) && text[i+1] == '/' {
			i += 2
			for i < len(text) && text[i] != '\n' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < len(text) && text[i+1] == '*' {
			i += 2
			for i+1 < len(text) && !(text[i] == '*' && text[i+1] == '/') {
				i++
			}
			i += 2
			continue
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}
