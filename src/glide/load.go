package glide

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
)

var readOnlyKeys = map[string]bool{
	"id": true, "createdAt": true, "createdBy": true,
	"updatedAt": true, "updatedBy": true, "lastUpdatedAt": true,
	"lastUpdatedBy": true, "ownerUserId": true, "_kind": true,
	"_id": true, "__v": true,
	// Local discovery keys (polyapi <resource> init). Never sent to the API.
	"polyType": true, "kind": true, "artifact_type": true,
}

var requiredKeys = map[string][]string{
	TypeVariable:       {"name", "context"},
	TypeTable:          {"name", "context"},
	TypeSchema:         {"name", "context"},
	TypeAIFunction:     {"name", "context"},
	TypeAPIFunction:    {"name", "context"},
	TypeClientFunction: {"name", "context"},
	TypeServerFunction: {"name", "context"},
	TypeWebhook:        {"name", "context"},
	TypeTrigger:        {"name"},
	TypeJob:            {"name"},
	TypeSnippet:        {"name", "context"},
	TypeSubscription:   {"name", "context"},
	TypeApplication:    {"name"},
}

func (e *Engine) load() ([]Item, error) {
	cands, err := Find(e.Root, e.Scope, e.Lang)
	if err != nil {
		return nil, wrap("discover", err)
	}
	var items []Item
	for _, c := range cands {
		item := e.loadOne(c)
		if item.Type != "" && !e.Scope.allowsType(item.Type) {
			continue
		}
		if item.Name != "" && !e.Scope.allowsContext(item.Context) {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func (e *Engine) loadOne(c Candidate) Item {
	item := Item{File: c.Rel, AbsFile: c.Abs, Kind: c.Kind, Type: c.Guess}
	var payload map[string]any
	var unresolved []Token
	switch c.Kind {
	case KindJSON:
		raw, err := os.ReadFile(c.Abs)
		if err != nil {
			item.LoadError = err.Error()
			return item
		}
		var v any
		if err := json.Unmarshal([]byte(StripJSONC(string(raw))), &v); err != nil {
			item.LoadError = "invalid JSON: " + err.Error()
			return item
		}
		obj, ok := asObject(v)
		if !ok {
			item.LoadError = "JSON artifact must be an object"
			return item
		}
		if !IsFunction(item.Type) {
			var miss []Token
			v, miss = substitute(obj, e.Env, c.Rel, "$")
			unresolved = miss
			obj, _ = asObject(v)
		}
		payload = obj
	case KindCode:
		if e.Adapter == nil {
			item.LoadError = "code deployable requires a language adapter (`extract`); install the project SDK or pass --adapter"
			return item
		}
		got, hash, err := e.Adapter.Extract(c.Rel, c.Guess)
		if err != nil {
			item.LoadError = err.Error()
			return item
		}
		obj, ok := asObject(got)
		if !ok {
			item.LoadError = "adapter extract did not return an object payload"
			return item
		}
		payload = obj
		item.Hash = hash
	}
	if payload == nil {
		if item.LoadError == "" {
			item.LoadError = "empty payload"
		}
		return item
	}
	if t := guessTypeFromMap(payload); t != "" {
		item.Type = t
	}
	item.Name = stringField(payload, "name")
	item.Context = stringField(payload, "context")
	if item.Name == "" {
		item.Name = baseName(c.Rel)
	}
	if item.Type == "" {
		item.Type = c.Guess
	}
	if item.Type == "" {
		item.LoadError = "unknown deployable type"
		return item
	}
	payload = blankSecrets(payload)
	if item.Type == TypeTable {
		payload = tableOutbound(payload)
	}
	if item.Type == TypeWebhook {
		payload = webhookOutbound(payload)
	}
	if item.Type == TypeJob {
		payload = jobOutbound(payload)
	}
	if item.Type == TypeSchema {
		payload = schemaOutbound(payload)
	}
	if item.Type == TypeSnippet {
		payload = snippetOutbound(payload)
	}
	if item.Type == TypeSubscription {
		payload = subscriptionOutbound(payload)
	}
	if item.Type == TypeApplication {
		payload = applicationOutbound(payload)
	}
	item.Payload = payload
	item.Unresolved = unresolved
	if item.Hash == "" {
		item.Hash = ContentHash(dropReadOnly(payload))
	}
	if e.Receipts {
		item.Receipt = e.lookupReceipt(item)
	}
	return item
}

func asObject(v any) (map[string]any, bool) {
	if v == nil {
		return nil, false
	}
	if m, ok := v.(map[string]any); ok {
		return m, true
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, false
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, false
	}
	return m, true
}

func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

func baseName(rel string) string {
	rel = strings.ReplaceAll(rel, "\\", "/")
	i := strings.LastIndex(rel, "/")
	name := rel
	if i >= 0 {
		name = rel[i+1:]
	}
	if d := strings.LastIndex(name, "."); d > 0 {
		name = name[:d]
	}
	return name
}

func dropReadOnly(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if readOnlyKeys[k] {
			continue
		}
		out[k] = v
	}
	return out
}

func blankSecrets(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	// Never deploy SECRET values from source. OBSCURED stays (readable at runtime; flow parity).
	if !isSecret(m) || isObscured(m) {
		return m
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	out["value"] = map[string]any{}
	return out
}

func isSecret(m map[string]any) bool {
	if m["secret"] == true {
		return true
	}
	switch strings.ToUpper(stringField(m, "secrecy")) {
	case "SECRET", "OBSCURED":
		return true
	}
	return false
}

func isObscured(m map[string]any) bool {
	return strings.ToUpper(stringField(m, "secrecy")) == "OBSCURED"
}

func project(remote, template map[string]any) map[string]any {
	if remote == nil {
		return nil
	}
	out := map[string]any{}
	for k, tv := range template {
		if readOnlyKeys[k] {
			continue
		}
		rv, ok := remote[k]
		if !ok {
			continue
		}
		out[k] = projectValue(rv, tv)
	}
	return out
}

func projectValue(remote, template any) any {
	tm, tOK := template.(map[string]any)
	rm, rOK := remote.(map[string]any)
	if tOK && rOK {
		return project(rm, tm)
	}
	return remote
}

var tablePayloadKeys = []string{"name", "context", "description", "visibility", "columns"}

func tableOutbound(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := map[string]any{}
	for _, k := range tablePayloadKeys {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	if cols, ok := out["columns"].([]any); ok {
		filtered := make([]any, 0, len(cols))
		for _, c := range cols {
			obj, ok := asObject(c)
			if !ok {
				filtered = append(filtered, c)
				continue
			}
			if managedTableColumn(stringField(obj, "name")) {
				continue
			}
			filtered = append(filtered, obj)
		}
		out["columns"] = filtered
	}
	return out
}

var webhookPayloadKeys = []string{
	"name", "context", "description", "visibility", "state",
	"eventPayload", "eventPayloadType", "eventPayloadTypeSchema",
	"responsePayload", "responseHeaders", "responseStatus",
	"slug", "subpath", "method", "requirePolyApiKey", "securityFunctions",
	"xmlParserOptions", "enabled",
}

func webhookOutbound(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := map[string]any{}
	for _, k := range webhookPayloadKeys {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	return out
}

var jobPayloadKeys = []string{"name", "schedule", "functions", "executionType", "enabled"}

func jobOutbound(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := map[string]any{}
	for _, k := range jobPayloadKeys {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	if _, ok := out["enabled"]; !ok {
		out["enabled"] = true
	}
	if v, ok := out["schedule"]; ok {
		out["schedule"] = normalizeJobSchedule(v)
	}
	return out
}

var schemaPayloadKeys = []string{"name", "context", "definition", "visibility"}

func schemaOutbound(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := map[string]any{}
	for _, k := range schemaPayloadKeys {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	return out
}

var snippetPayloadKeys = []string{"name", "context", "description", "code", "language", "visibility"}

func snippetOutbound(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := map[string]any{}
	for _, k := range snippetPayloadKeys {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	return out
}

var subscriptionPayloadKeys = []string{
	"name", "context", "description", "visibility", "type", "transportProtocol",
	"websocketUrl", "query", "functionId", "functionParams",
	"paramsVariableId", "paramsSfxId", "paramsObject",
	"enabled", "ohipMaintainOffset", "eventInactivityThresholdMs", "queueId",
}

func subscriptionOutbound(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := map[string]any{}
	for _, k := range subscriptionPayloadKeys {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	if _, ok := out["enabled"]; !ok {
		out["enabled"] = true
	}
	if _, ok := out["transportProtocol"]; !ok {
		out["transportProtocol"] = "WS"
	}
	return out
}

func isPlaceholderParams(v any) bool {
	if v == nil {
		return true
	}
	if s, ok := v.(string); ok {
		s = strings.TrimSpace(s)
		return s == "" || strings.EqualFold(s, "REDACTED_VALUE")
	}
	m, ok := asObject(v)
	if !ok || len(m) == 0 {
		return true
	}
	ohip, ok := asObject(m["ohip"])
	if !ok {
		return false
	}
	for _, k := range []string{"hostName", "appKey", "enterpriseId", "clientId", "clientSecret"} {
		s := strings.TrimSpace(stringField(ohip, k))
		if s != "" && !strings.HasPrefix(s, "{{") {
			return false
		}
	}
	return true
}

func projectSubscriptionSecrets(local, remote map[string]any) map[string]any {
	if local == nil {
		return local
	}
	if !isPlaceholderParams(local["paramsObject"]) {
		return local
	}
	out := map[string]any{}
	for k, v := range local {
		out[k] = v
	}
	if remote != nil {
		if v, ok := remote["paramsObject"]; ok {
			out["paramsObject"] = v
		} else {
			delete(out, "paramsObject")
		}
	}
	return out
}

func subscriptionPatch(local, remote map[string]any) map[string]any {
	out := map[string]any{}
	if local == nil {
		return out
	}
	for _, k := range subscriptionPayloadKeys {
		lv, lok := local[k]
		if !lok {
			continue
		}
		if k == "paramsObject" && isPlaceholderParams(lv) {
			continue
		}
		rv, rok := remote[k]
		if rok && ContentHash(map[string]any{"v": lv}) == ContentHash(map[string]any{"v": rv}) {
			continue
		}
		if !rok && isEmptySubscriptionValue(lv) {
			continue
		}
		out[k] = lv
	}
	return out
}

func isEmptySubscriptionValue(v any) bool {
	if v == nil {
		return true
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t) == ""
	case map[string]any:
		return len(t) == 0
	default:
		return false
	}
}

func redactSubscriptionPull(payload, existing map[string]any) map[string]any {
	out := subscriptionOutbound(dropReadOnly(payload))
	if out == nil {
		return out
	}
	params, ok := asObject(out["paramsObject"])
	if !ok || params == nil {
		return out
	}
	redacted := redactParamsObject(params, existing)
	out["paramsObject"] = redacted
	return out
}

func redactParamsObject(params, existing map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range params {
		out[k] = v
	}
	ohip, ok := asObject(out["ohip"])
	if !ok || ohip == nil {
		return out
	}
	cp := map[string]any{}
	for k, v := range ohip {
		cp[k] = v
	}
	var existingSecret string
	if existing != nil {
		if prev, ok := asObject(existing["paramsObject"]); ok {
			if prevOhip, ok := asObject(prev["ohip"]); ok {
				existingSecret = stringField(prevOhip, "clientSecret")
			}
		}
	}
	if strings.HasPrefix(existingSecret, "{{") {
		cp["clientSecret"] = existingSecret
	} else {
		cp["clientSecret"] = ""
	}
	out["ohip"] = cp
	return out
}

var applicationPayloadKeys = []string{"name", "description", "visibility", "config"}

func applicationOutbound(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := map[string]any{}
	for _, k := range applicationPayloadKeys {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	return out
}

func normalizeJobSchedule(v any) any {
	switch t := v.(type) {
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return v
		}
		return map[string]any{"type": "periodical", "value": s}
	case float64, int, int64, uint, uint64, json.Number:
		return map[string]any{"type": "interval", "value": t}
	default:
		return v
	}
}

func triggerComparable(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	src, _ := asObject(m["source"])
	dest, _ := asObject(m["destination"])
	flat := map[string]any{}
	if v, ok := m["name"]; ok {
		flat["name"] = v
	}
	if v, ok := m["enabled"]; ok {
		flat["enabled"] = v
	} else {
		flat["enabled"] = true
	}
	if v, ok := m["waitForResponse"]; ok {
		flat["waitForResponse"] = v
	}
	if id := firstNonEmptyField(m, dest, "serverFunctionId"); id != nil {
		flat["serverFunctionId"] = id
	}
	if id := firstNonEmptyField(m, src, "webhookHandleId"); id != nil {
		flat["webhookHandleId"] = id
	}
	if eh := firstMap(m, src, "errorHandler"); eh != nil {
		flat["errorHandler"] = eh
	}
	return dropReadOnly(flat)
}

func firstNonEmptyField(top, nested map[string]any, key string) any {
	if nested != nil {
		if s := stringField(nested, key); s != "" {
			return s
		}
	}
	if top != nil {
		if s := stringField(top, key); s != "" {
			return s
		}
	}
	return nil
}

func firstMap(top, nested map[string]any, key string) map[string]any {
	if nested != nil {
		if m, ok := asObject(nested[key]); ok {
			return m
		}
	}
	if top != nil {
		if m, ok := asObject(top[key]); ok {
			return m
		}
	}
	return nil
}

func errorHandlerPath(m map[string]any) string {
	eh, _ := asObject(m["errorHandler"])
	return stringField(eh, "path")
}

func triggerSourceDestChanged(local, remote map[string]any) bool {
	l := triggerComparable(local)
	r := triggerComparable(remote)
	lFn, rFn := stringField(l, "serverFunctionId"), stringField(r, "serverFunctionId")
	if lFn != "" && rFn != "" && lFn != rFn {
		return true
	}
	lWH, lPath := stringField(l, "webhookHandleId"), errorHandlerPath(l)
	rWH, rPath := stringField(r, "webhookHandleId"), errorHandlerPath(r)
	lHas, rHas := lWH != "" || lPath != "", rWH != "" || rPath != ""
	if !lHas || !rHas {
		// List DTOs may omit source; do not guess a change.
		return false
	}
	if (lWH != "") != (rWH != "") || (lPath != "") != (rPath != "") {
		return true
	}
	return lWH != rWH || lPath != rPath
}

func managedTableColumn(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "id", "createdat", "updatedat", "created_at", "updated_at":
		return true
	default:
		return false
	}
}

// projectTableColumns reduces remote column objects to the keys the local file sets,
// matched by column name (platform list/get DTOs add schema/flags we do not manage).
func projectTableColumns(remoteCols, localCols []any) []any {
	templates := map[string]map[string]any{}
	var fallback map[string]any
	for _, c := range localCols {
		m, ok := asObject(c)
		if !ok {
			continue
		}
		if fallback == nil {
			fallback = m
		}
		if name := stringField(m, "name"); name != "" {
			templates[name] = m
		}
	}
	out := make([]any, 0, len(remoteCols))
	for _, c := range remoteCols {
		m, ok := asObject(c)
		if !ok {
			out = append(out, c)
			continue
		}
		tmpl := templates[stringField(m, "name")]
		if tmpl == nil {
			tmpl = fallback
		}
		if tmpl == nil {
			out = append(out, m)
			continue
		}
		out = append(out, project(m, tmpl))
	}
	return out
}

func changedFields(local, remote map[string]any) []string {
	var keys []string
	for k, lv := range local {
		if readOnlyKeys[k] {
			continue
		}
		if ContentHash(lv) != ContentHash(remote[k]) {
			keys = append(keys, k)
		}
	}
	return keys
}

func sortItems(items []Item) {
	order := map[string]int{}
	for i, t := range DeployOrder() {
		order[t] = i
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Type != items[j].Type {
			return order[items[i].Type] < order[items[j].Type]
		}
		return items[i].Identity() < items[j].Identity()
	})
}
