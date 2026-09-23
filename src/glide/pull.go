package glide

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/polyapi/polyglot/src/exitcode"
)

func (e *Engine) Pull() ([]Result, error) {
	if e.Client == nil {
		return nil, failCode(exitcode.Auth, "no API client; configure credentials")
	}
	items, err := e.load()
	if err != nil {
		return nil, err
	}
	localByID := map[string]Item{}
	for _, it := range items {
		localByID[it.Identity()] = it
	}

	var results []Result
	pulled := 0
	lists := map[string][]map[string]any{}
	for _, typ := range e.Scope.Types {
		if IsFunction(typ) {
			results = append(results, Result{Type: typ, Action: ActionSkip, Error: "function pull is not implemented in v1"})
			continue
		}
		col := Collection(typ)
		if col == "" {
			continue
		}
		rows, err := e.listCol(lists, col)
		if err != nil {
			return results, err
		}
		remoteIDs := map[string]bool{}
		for _, row := range rows {
			name := stringField(row, "name")
			ctx := stringField(row, "context")
			if name == "" || !e.Scope.allowsContext(ctx) {
				continue
			}
			id := IdentityKey(typ, ctx, name)
			remoteIDs[id] = true
			payload := dropReadOnly(row)
			if typ == TypeSubscription {
				rid := stringField(row, "id")
				detail, err := e.hydrateDetail(typ, rid, row)
				if err != nil {
					results = append(results, Result{Type: typ, Name: name, Context: ctx, Action: ActionFailed, Error: err.Error()})
					continue
				}
				var existing map[string]any
				if loc, ok := localByID[id]; ok {
					existing = loc.Payload
				}
				payload = redactSubscriptionPull(detail, existing)
			}
			dest, err := e.pullPath(typ, ctx, name, localByID[id])
			if err != nil {
				results = append(results, Result{Type: typ, Name: name, Context: ctx, Action: ActionFailed, Error: err.Error()})
				continue
			}
			action, err := e.writeJSON(dest, payload, localByID[id])
			res := Result{Type: typ, Name: name, Context: ctx, File: dest, Action: action}
			if err != nil {
				res.Action = ActionFailed
				res.Error = err.Error()
			}
			results = append(results, res)
			pulled++
		}
		if e.DeleteOrphans {
			for _, it := range items {
				if it.Type != typ || it.Kind != KindJSON {
					continue
				}
				if remoteIDs[it.Identity()] {
					continue
				}
				if e.DryRun {
					results = append(results, Result{Type: it.Type, Name: it.Name, Context: it.Context, File: it.File, Action: ActionWouldDelete})
					continue
				}
				if err := os.Remove(it.AbsFile); err != nil {
					results = append(results, Result{Type: it.Type, Name: it.Name, Context: it.Context, File: it.File, Action: ActionFailed, Error: err.Error()})
					continue
				}
				results = append(results, Result{Type: it.Type, Name: it.Name, Context: it.Context, File: it.File, Action: ActionDeleted})
			}
		}
	}
	if len(items) == 0 && pulled == 0 {
		return results, e.nothingFound()
	}
	return results, nil
}

func (e *Engine) writeJSON(rel string, payload map[string]any, existing Item) (string, error) {
	abs, err := confine(e.Root, rel)
	if err != nil {
		return "", err
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	body = append(body, '\n')
	if _, err := os.Stat(abs); err == nil {
		old, _ := os.ReadFile(abs)
		if string(old) == string(body) && !e.Force {
			return ActionSkip, nil
		}
		if e.DryRun {
			return ActionWouldUpdate, nil
		}
	} else if e.DryRun {
		return ActionWouldCreate, nil
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(abs, body, 0o644); err != nil {
		return "", err
	}
	if existing.File != "" && existing.Hash != ContentHash(payload) {
		return ActionUpdate, nil
	}
	if existing.File == "" {
		return ActionCreate, nil
	}
	return ActionUpdate, nil
}

func (e *Engine) pullPath(typ, ctx, name string, existing Item) (string, error) {
	if existing.File != "" && existing.Kind == KindJSON {
		return existing.File, nil
	}
	if mapped := mapContextPath(e.Scope.ContextPathMap, ctx); mapped != "" {
		return filepath.ToSlash(filepath.Join(mapped, artefactsDir(typ), name+".json")), nil
	}
	ctxPath := strings.ReplaceAll(ctx, ".", string(filepath.Separator))
	if ctxPath == "" {
		ctxPath = "_"
	}
	return filepath.ToSlash(filepath.Join("src", ctxPath, "artifacts", artefactsDir(typ), name+".json")), nil
}

func artefactsDir(typ string) string {
	switch typ {
	case TypeVariable:
		return "vari"
	case TypeTable:
		return "tabi"
	case TypeWebhook:
		return "webhooks"
	case TypeJob:
		return "jobs"
	case TypeTrigger:
		return "triggers"
	case TypeSchema:
		return "schemas"
	case TypeSnippet:
		return "snippets"
	case TypeSubscription:
		return "subscriptions"
	case TypeApplication:
		return "applications"
	case TypeServerFunction:
		return "server"
	case TypeClientFunction:
		return "client"
	case TypeAIFunction:
		return "ai"
	case TypeAPIFunction:
		return "api"
	default:
		return typ
	}
}

func mapContextPath(m map[string]string, ctx string) string {
	if m == nil {
		return ""
	}
	if p, ok := m[ctx]; ok {
		return p
	}
	best, bestLen := "", -1
	for prefix, p := range m {
		if ctx == prefix || strings.HasPrefix(ctx, prefix+".") {
			if len(prefix) > bestLen {
				best, bestLen = p, len(prefix)
			}
		}
	}
	return best
}

func confine(root, rel string) (string, error) {
	abs := rel
	if !filepath.IsAbs(rel) {
		abs = filepath.Join(root, rel)
	}
	abs = filepath.Clean(abs)
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	abs, err = filepath.Abs(abs)
	if err != nil {
		return "", err
	}
	relOut, err := filepath.Rel(rootAbs, abs)
	if err != nil {
		return "", err
	}
	if relOut == ".." || strings.HasPrefix(relOut, ".."+string(filepath.Separator)) {
		return "", fail("pull path escapes project root: " + rel)
	}
	return abs, nil
}
