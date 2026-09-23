package glide

import (
	"path/filepath"
	"regexp"
	"strings"
)

var uuidRe = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func (e *Engine) Validate() (Report, error) {
	items, err := e.load()
	if err != nil {
		return Report{}, err
	}
	if len(items) == 0 {
		return Report{}, e.nothingFound()
	}
	sortItems(items)
	rep := Report{Items: items, Eval: e.EvalMessage}
	cats := e.onlySet()

	if cats[CatDiscovery] {
		e.checkDiscovery(&rep, items)
	}
	if cats[CatMetadata] {
		e.checkMetadata(&rep, items)
	}
	if cats[CatEnv] {
		e.checkEnv(&rep, items)
	}
	if cats[CatCrossResource] {
		e.checkRefs(&rep, items)
	}
	if cats[CatConfig] {
		e.checkConfig(&rep)
	}
	return rep, nil
}

func (e *Engine) onlySet() map[string]bool {
	out := map[string]bool{}
	if len(e.Only) == 0 {
		for _, c := range []string{CatEnv, CatMetadata, CatDiscovery, CatCrossResource, CatConfig} {
			out[c] = true
		}
		return out
	}
	for _, raw := range e.Only {
		for _, p := range strings.Split(raw, ",") {
			p = strings.ToLower(strings.TrimSpace(p))
			switch p {
			case "env", "token", "tokens":
				out[CatEnv] = true
			case "metadata", "meta":
				out[CatMetadata] = true
			case "discovery", "discover":
				out[CatDiscovery] = true
			case "cross-resource", "cross", "refs", "ref":
				out[CatCrossResource] = true
			case "config":
				out[CatConfig] = true
			}
		}
	}
	return out
}

func (e *Engine) checkDiscovery(rep *Report, items []Item) {
	seen := map[string]string{}
	for _, it := range items {
		if it.LoadError != "" {
			rep.Issues = append(rep.Issues, Issue{
				Severity: SeverityError, Category: CatDiscovery,
				Path: it.File, Identity: it.Identity(),
				Message: it.LoadError,
			})
			continue
		}
		if Collection(it.Type) == "" {
			rep.Issues = append(rep.Issues, Issue{
				Severity: SeverityError, Category: CatDiscovery,
				Path: it.File, Identity: it.Identity(),
				Message: "unknown type " + it.Type,
			})
			continue
		}
		id := it.Identity()
		if prev, ok := seen[id]; ok {
			rep.Issues = append(rep.Issues, Issue{
				Severity: SeverityError, Category: CatDiscovery,
				Path: it.File, Identity: id,
				Message: "duplicate " + id + " (also " + prev + ")",
			})
		}
		seen[id] = it.File
	}
}

func (e *Engine) checkMetadata(rep *Report, items []Item) {
	for _, it := range items {
		if it.LoadError != "" {
			continue
		}
		for _, key := range requiredKeys[it.Type] {
			if strings.TrimSpace(stringField(it.Payload, key)) == "" {
				rep.Issues = append(rep.Issues, Issue{
					Severity: SeverityError, Category: CatMetadata,
					Path: it.File, Identity: it.Identity(),
					Message: "missing " + key,
				})
			}
		}
		if it.Type == TypeTable {
			cols, _ := it.Payload["columns"].([]any)
			if cols == nil {
				rep.Issues = append(rep.Issues, Issue{
					Severity: SeverityError, Category: CatMetadata,
					Path: it.File, Identity: it.Identity(),
					Message: "missing columns",
				})
			}
		}
		if it.Type == TypeJob {
			fns, _ := it.Payload["functions"].([]any)
			if len(fns) == 0 {
				rep.Issues = append(rep.Issues, Issue{
					Severity: SeverityError, Category: CatMetadata,
					Path: it.File, Identity: it.Identity(),
					Message: "missing functions",
				})
			}
			et := strings.ToLower(strings.TrimSpace(stringField(it.Payload, "executionType")))
			if et == "" {
				rep.Issues = append(rep.Issues, Issue{
					Severity: SeverityError, Category: CatMetadata,
					Path: it.File, Identity: it.Identity(),
					Message: "missing executionType",
				})
			} else if et != "sequential" && et != "parallel" {
				rep.Issues = append(rep.Issues, Issue{
					Severity: SeverityError, Category: CatMetadata,
					Path: it.File, Identity: it.Identity(),
					Message: "executionType must be sequential or parallel",
				})
			}
		}
		if it.Type == TypeSchema {
			if _, ok := asObject(it.Payload["definition"]); !ok {
				rep.Issues = append(rep.Issues, Issue{
					Severity: SeverityError, Category: CatMetadata,
					Path: it.File, Identity: it.Identity(),
					Message: "missing definition",
				})
			}
		}
		if it.Type == TypeSnippet {
			if strings.TrimSpace(stringField(it.Payload, "code")) == "" {
				rep.Issues = append(rep.Issues, Issue{
					Severity: SeverityError, Category: CatMetadata,
					Path: it.File, Identity: it.Identity(),
					Message: "missing code",
				})
			}
			if strings.TrimSpace(stringField(it.Payload, "language")) == "" {
				rep.Issues = append(rep.Issues, Issue{
					Severity: SeverityError, Category: CatMetadata,
					Path: it.File, Identity: it.Identity(),
					Message: "missing language",
				})
			}
		}
		if it.Type == TypeSubscription {
			e.checkSubscriptionMetadata(rep, it)
		}
		if it.Type == TypeApplication {
			cfg, ok := asObject(it.Payload["config"])
			if !ok {
				rep.Issues = append(rep.Issues, Issue{
					Severity: SeverityError, Category: CatMetadata,
					Path: it.File, Identity: it.Identity(),
					Message: "missing config",
				})
			} else if _, ok := cfg["collections"]; !ok {
				rep.Issues = append(rep.Issues, Issue{
					Severity: SeverityError, Category: CatMetadata,
					Path: it.File, Identity: it.Identity(),
					Message: "missing config.collections",
				})
			}
		}
		if it.Type != TypeJob && it.Type != TypeSchema && strings.TrimSpace(stringField(it.Payload, "description")) == "" {
			rep.Issues = append(rep.Issues, Issue{
				Severity: SeverityWarn, Category: CatMetadata,
				Path: it.File, Identity: it.Identity(),
				Message: "missing description",
			})
		}
	}
}

func (e *Engine) checkEnv(rep *Report, items []Item) {
	used := map[string]bool{}
	for _, it := range items {
		if it.LoadError != "" || IsFunction(it.Type) {
			continue
		}
		for _, tok := range it.Unresolved {
			used[tok.Name] = true
			rep.Issues = append(rep.Issues, Issue{
				Severity: SeverityError, Category: CatEnv,
				Path: it.File, Identity: it.Identity(),
				Message: "unresolved {{" + tok.Name + "}} at " + tok.JSONPath,
			})
		}
		for _, name := range tokensIn(it.Payload) {
			used[name] = true
		}
	}
	dot := LoadDotEnv(filepath.Join(e.Root, ".env"))
	for name := range dot {
		if !used[name] && !strings.HasPrefix(name, "POLY_") {
			rep.Issues = append(rep.Issues, Issue{
				Severity: SeverityWarn, Category: CatEnv,
				Message: "unused token " + name + " in .env",
			})
		}
	}
}

func (e *Engine) checkRefs(rep *Report, items []Item) {
	idx := newRefIndex(items)
	for _, it := range items {
		if it.LoadError != "" {
			continue
		}
		for _, ref := range collectRefs(it) {
			if uuidRe.MatchString(ref.Raw) {
				if e.Online && e.Client != nil {
					if _, err := e.Client.Get(ref.Collection, ref.Raw); err != nil {
						rep.Issues = append(rep.Issues, Issue{
							Severity: SeverityError, Category: CatCrossResource,
							Path: it.File, Identity: it.Identity(),
							Message: "remote id " + ref.Raw + " not found for " + ref.Field,
						})
					}
				}
				continue
			}
			if !idx.has(ref) {
				rep.Issues = append(rep.Issues, Issue{
					Severity: SeverityError, Category: CatCrossResource,
					Path: it.File, Identity: it.Identity(),
					Message: ref.Field + " references missing " + ref.Raw,
				})
			}
		}
	}
}

func (e *Engine) checkConfig(rep *Report) {
	if e.HasCredentials {
		return
	}
	rep.Issues = append(rep.Issues, Issue{
		Severity: SeverityWarn, Category: CatConfig,
		Message: "no API key or base URL configured; run `polyapi auth login` before push",
	})
}

type refWant struct {
	Raw        string
	Field      string
	Types      []string
	Collection string
}

type refIndex struct {
	byKey map[string]Item
}

func newRefIndex(items []Item) refIndex {
	idx := refIndex{byKey: map[string]Item{}}
	for _, it := range items {
		if it.LoadError != "" || it.Name == "" {
			continue
		}
		idx.byKey[it.Type+":"+DisplayName(it.Context, it.Name)] = it
		idx.byKey[DisplayName(it.Context, it.Name)] = it
	}
	return idx
}

func (idx refIndex) has(r refWant) bool {
	if r.Raw == "" {
		return false
	}
	body := strings.TrimSpace(r.Raw)
	body = strings.TrimPrefix(body, "{{")
	body = strings.TrimSuffix(body, "}}")
	body = strings.TrimSpace(body)
	for _, t := range r.Types {
		if _, ok := idx.byKey[t+":"+body]; ok {
			return true
		}
	}
	_, ok := idx.byKey[body]
	return ok
}

func collectRefs(it Item) []refWant {
	var refs []refWant
	switch it.Type {
	case TypeWebhook:
		if arr, ok := it.Payload["securityFunctions"].([]any); ok {
			for _, raw := range arr {
				if m, ok := asObject(raw); ok {
					id := stringField(m, "id")
					if id != "" {
						refs = append(refs, refWant{
							Raw: id, Field: "securityFunctions",
							Types:      []string{TypeServerFunction},
							Collection: "functions/server",
						})
					}
				}
			}
		}
	case TypeJob:
		if arr, ok := it.Payload["functions"].([]any); ok {
			for _, raw := range arr {
				m, ok := asObject(raw)
				if !ok {
					continue
				}
				id := stringField(m, "id")
				if id == "" {
					ctx, name := stringField(m, "functionContext"), stringField(m, "functionName")
					if ctx != "" && name != "" {
						id = ctx + "." + name
					}
				}
				if id != "" {
					refs = append(refs, refWant{
						Raw: id, Field: "functions",
						Types:      []string{TypeServerFunction},
						Collection: "functions/server",
					})
				}
			}
		}
	case TypeTrigger:
		if dest, ok := asObject(it.Payload["destination"]); ok {
			ctx, name := stringField(dest, "functionContext"), stringField(dest, "functionName")
			if ctx != "" && name != "" {
				refs = append(refs, refWant{
					Raw: ctx + "." + name, Field: "destination",
					Types:      []string{TypeServerFunction},
					Collection: "functions/server",
				})
			}
			if id := stringField(dest, "serverFunctionId"); id != "" {
				refs = append(refs, refWant{
					Raw: id, Field: "destination",
					Types:      []string{TypeServerFunction},
					Collection: "functions/server",
				})
			}
		}
		if src, ok := asObject(it.Payload["source"]); ok {
			ctx, name := stringField(src, "webhookContext"), stringField(src, "webhookName")
			if ctx != "" && name != "" {
				refs = append(refs, refWant{
					Raw: ctx + "." + name, Field: "source",
					Types:      []string{TypeWebhook},
					Collection: "webhooks",
				})
			}
			if id := stringField(src, "webhookHandleId"); id != "" {
				refs = append(refs, refWant{
					Raw: id, Field: "source",
					Types:      []string{TypeWebhook},
					Collection: "webhooks",
				})
			}
		}
	case TypeSubscription:
		if id := stringField(it.Payload, "functionId"); id != "" {
			refs = append(refs, refWant{
				Raw: id, Field: "functionId",
				Types:      []string{TypeServerFunction},
				Collection: "functions/server",
			})
		}
		if id := stringField(it.Payload, "paramsSfxId"); id != "" {
			refs = append(refs, refWant{
				Raw: id, Field: "paramsSfxId",
				Types:      []string{TypeServerFunction},
				Collection: "functions/server",
			})
		}
		if id := stringField(it.Payload, "paramsVariableId"); id != "" {
			refs = append(refs, refWant{
				Raw: id, Field: "paramsVariableId",
				Types:      []string{TypeVariable},
				Collection: "variables",
			})
		}
	case TypeSchema:
		if id := stringField(it.Payload, "schemaId"); id != "" {
			refs = append(refs, refWant{Raw: id, Field: "schemaId", Types: []string{TypeSchema}, Collection: "schemas"})
		}
		for _, path := range collectPolySchemaRefs(it.Payload["definition"]) {
			refs = append(refs, refWant{Raw: path, Field: "definition", Types: []string{TypeSchema}, Collection: "schemas"})
		}
	}
	return refs
}

func (e *Engine) checkSubscriptionMetadata(rep *Report, it Item) {
	typ := strings.ToUpper(strings.TrimSpace(stringField(it.Payload, "type")))
	if _, ok := NormalizeSubscriptionType(typ); !ok {
		rep.Issues = append(rep.Issues, Issue{
			Severity: SeverityError, Category: CatMetadata,
			Path: it.File, Identity: it.Identity(),
			Message: "type must be CUSTOM or OHIP",
		})
	}
	if strings.TrimSpace(stringField(it.Payload, "websocketUrl")) == "" {
		rep.Issues = append(rep.Issues, Issue{
			Severity: SeverityError, Category: CatMetadata,
			Path: it.File, Identity: it.Identity(),
			Message: "missing websocketUrl",
		})
	} else {
		url := stringField(it.Payload, "websocketUrl")
		if !strings.HasPrefix(url, "ws://") && !strings.HasPrefix(url, "wss://") {
			rep.Issues = append(rep.Issues, Issue{
				Severity: SeverityError, Category: CatMetadata,
				Path: it.File, Identity: it.Identity(),
				Message: "websocketUrl must start with ws:// or wss://",
			})
		}
	}
	if strings.TrimSpace(stringField(it.Payload, "query")) == "" {
		rep.Issues = append(rep.Issues, Issue{
			Severity: SeverityError, Category: CatMetadata,
			Path: it.File, Identity: it.Identity(),
			Message: "missing query",
		})
	}
	if strings.TrimSpace(stringField(it.Payload, "functionId")) == "" {
		rep.Issues = append(rep.Issues, Issue{
			Severity: SeverityError, Category: CatMetadata,
			Path: it.File, Identity: it.Identity(),
			Message: "missing functionId",
		})
	}
	proto := strings.ToUpper(strings.TrimSpace(stringField(it.Payload, "transportProtocol")))
	if proto != "" && proto != "WS" {
		rep.Issues = append(rep.Issues, Issue{
			Severity: SeverityError, Category: CatMetadata,
			Path: it.File, Identity: it.Identity(),
			Message: "transportProtocol must be WS",
		})
	}
	nParams := 0
	if strings.TrimSpace(stringField(it.Payload, "paramsVariableId")) != "" {
		nParams++
	}
	if strings.TrimSpace(stringField(it.Payload, "paramsSfxId")) != "" {
		nParams++
	}
	if !isPlaceholderParams(it.Payload["paramsObject"]) {
		nParams++
	}
	if nParams > 1 {
		rep.Issues = append(rep.Issues, Issue{
			Severity: SeverityError, Category: CatMetadata,
			Path: it.File, Identity: it.Identity(),
			Message: "only one of paramsVariableId, paramsSfxId, or paramsObject may be set",
		})
	}
	if typ == SubscriptionTypeCustom {
		if _, ok := it.Payload["ohipMaintainOffset"]; ok {
			rep.Issues = append(rep.Issues, Issue{
				Severity: SeverityError, Category: CatMetadata,
				Path: it.File, Identity: it.Identity(),
				Message: "OHIP fields are only valid on OHIP subscriptions",
			})
		}
	}
	if typ == SubscriptionTypeOHIP {
		params, _ := asObject(it.Payload["paramsObject"])
		var ohip map[string]any
		if params != nil {
			ohip, _ = asObject(params["ohip"])
		}
		if ohip == nil {
			rep.Issues = append(rep.Issues, Issue{
				Severity: SeverityError, Category: CatMetadata,
				Path: it.File, Identity: it.Identity(),
				Message: "OHIP subscriptions require paramsObject.ohip",
			})
		} else {
			for _, k := range []string{"hostName", "appKey", "enterpriseId", "clientId", "clientSecret"} {
				if _, ok := ohip[k]; !ok {
					rep.Issues = append(rep.Issues, Issue{
						Severity: SeverityError, Category: CatMetadata,
						Path: it.File, Identity: it.Identity(),
						Message: "paramsObject.ohip missing " + k,
					})
				}
			}
		}
		if it.Payload["ohipMaintainOffset"] == true && !strings.Contains(stringField(it.Payload, "query"), "offset") {
			rep.Issues = append(rep.Issues, Issue{
				Severity: SeverityError, Category: CatMetadata,
				Path: it.File, Identity: it.Identity(),
				Message: "ohipMaintainOffset requires the query to select metadata.offset",
			})
		}
	}
}

func collectPolySchemaRefs(node any) []string {
	switch n := node.(type) {
	case []any:
		var out []string
		for _, item := range n {
			out = append(out, collectPolySchemaRefs(item)...)
		}
		return out
	case map[string]any:
		var out []string
		if ref, ok := asObject(n["x-poly-ref"]); ok {
			if stringField(ref, "publicNamespace") == "" {
				if path := stringField(ref, "path"); path != "" {
					out = append(out, path)
				}
			}
		}
		for k, v := range n {
			if k == "x-poly-ref" {
				continue
			}
			out = append(out, collectPolySchemaRefs(v)...)
		}
		return out
	default:
		return nil
	}
}
