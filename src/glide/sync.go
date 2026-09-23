package glide

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/polyapi/polyglot/src/api"
	"github.com/polyapi/polyglot/src/exitcode"
)

func (e *Engine) Plan() ([]Result, error) {
	return e.sync(true)
}

func (e *Engine) Push() ([]Result, error) {
	if err := e.guardMutate(); err != nil {
		return nil, err
	}
	if err := e.confirmProduction(); err != nil {
		return nil, err
	}
	return e.sync(false)
}

func (e *Engine) guardMutate() error {
	if e.PushAllowed || e.EvalStatus == "skip" || e.EvalStatus == "" {
		return nil
	}
	msg := e.EvalMessage
	if msg == "" {
		msg = "current branch is not in deploy.targets"
	}
	if !strings.Contains(msg, "deploy.targets") {
		msg += "; add the branch to [[deploy.targets]] in .poly/config.toml"
	}
	return fail(msg)
}

func (e *Engine) confirmProduction() error {
	if !e.Production {
		return nil
	}
	if os.Getenv("POLYPROD_EXEC_KEY") != "" {
		return nil
	}
	if e.Yes {
		return nil
	}
	if e.NonInteractive {
		return fail("production deploy requires --yes or POLYPROD_EXEC_KEY")
	}
	if e.Stderr != nil {
		_, _ = e.Stderr.Write([]byte("Type PROD to continue: "))
	}
	if e.Stdin == nil {
		return fail("production deploy requires --yes or POLYPROD_EXEC_KEY")
	}
	buf := make([]byte, 32)
	n, _ := e.Stdin.Read(buf)
	got := strings.TrimSpace(string(buf[:n]))
	if got != "PROD" {
		return fail("production confirmation failed")
	}
	return nil
}

func (e *Engine) sync(dry bool) ([]Result, error) {
	if e.Client == nil {
		return nil, failCode(exitcode.Auth, "no API client; configure credentials")
	}
	items, err := e.load()
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, e.nothingFound()
	}
	sortItems(items)

	if !e.AllowUnresolved {
		for _, it := range items {
			if len(it.Unresolved) > 0 {
				return nil, fail("unresolved {{" + it.Unresolved[0].Name + "}} in " + it.File + "; run `polyapi deploy validate --only env` or pass --allow-unresolved")
			}
		}
	}

	done := map[string]bool{}
	if e.Resume {
		done = e.readResume()
	} else {
		_ = os.Remove(e.resumePath())
	}

	lists := map[string][]map[string]any{}
	ids := idIndex{}
	for _, typ := range DeployOrder() {
		rows, err := e.listCol(lists, Collection(typ))
		if err != nil {
			continue
		}
		for _, row := range rows {
			name := stringField(row, "name")
			ctx := stringField(row, "context")
			id := stringField(row, "id")
			if name == "" || id == "" {
				continue
			}
			ids.put(typ, ctx, name, id)
		}
	}
	var results []Result
	for _, it := range items {
		if it.LoadError != "" {
			results = append(results, Result{
				Type: it.Type, Name: it.Name, Context: it.Context, File: it.File,
				Action: ActionFailed, Error: it.LoadError,
			})
			continue
		}
		if done[it.Identity()] {
			results = append(results, Result{
				Type: it.Type, Name: it.Name, Context: it.Context, File: it.File,
				Action: ActionSkip, Error: "resume",
			})
			continue
		}
		col := Collection(it.Type)
		rows, err := e.listCol(lists, col)
		if err != nil {
			results = append(results, Result{
				Type: it.Type, Name: it.Name, Context: it.Context, File: it.File,
				Action: ActionFailed, Error: err.Error(),
			})
			continue
		}
		remote := findRemote(it, rows, ids)
		action, changed, id, err := e.applyOne(it, remote, dry, ids)
		if id != "" {
			ids.put(it.Type, it.Context, it.Name, id)
		}
		res := Result{
			Type: it.Type, Name: it.Name, Context: it.Context, File: it.File,
			Action: action, Changed: changed, ID: id,
		}
		if err != nil {
			if action != ActionBlocked {
				res.Action = ActionFailed
			}
			res.Error = err.Error()
		}
		results = append(results, res)
		if !dry && err == nil && (action == ActionCreate || action == ActionUpdate || action == ActionSkip) {
			e.writeResume(it.Identity())
		}
	}

	orphans, err := e.collectOrphans(items, lists)
	if err != nil {
		return results, err
	}
	for _, o := range orphans {
		res := o
		if dry {
			if e.DeleteOrphans {
				res.Action = ActionWouldDelete
			} else {
				res.Action = ActionOrphan
			}
		} else if e.DeleteOrphans || (e.Receipts && o.Error == "receipt") {
			if err := e.Client.Delete(Collection(o.Type), o.ID, nil); err != nil {
				res.Action = ActionFailed
				res.Error = err.Error()
			} else {
				res.Action = ActionDeleted
				res.Error = ""
				if e.Receipts {
					e.removeReceipt(o)
				}
			}
		} else {
			res.Action = ActionOrphan
			res.Error = ""
		}
		results = append(results, res)
	}

	if !dry {
		_ = os.Remove(e.resumePath())
	}
	return results, nil
}

type idIndex map[string]string

func (idx idIndex) put(typ, ctx, name, id string) {
	if idx == nil || id == "" || name == "" {
		return
	}
	idx[IdentityKey(typ, ctx, name)] = id
	idx[DisplayName(ctx, name)] = id
	idx[typ+":"+DisplayName(ctx, name)] = id
}

func (idx idIndex) lookup(raw string, types []string) string {
	body := strings.TrimSpace(raw)
	body = strings.TrimPrefix(body, "{{")
	body = strings.TrimSuffix(body, "}}")
	body = strings.TrimSpace(body)
	if uuidRe.MatchString(body) {
		return body
	}
	for _, t := range types {
		if id := idx[t+":"+body]; id != "" {
			return id
		}
		if id := idx[IdentityKey(t, "", body)]; id != "" {
			return id
		}
	}
	if id := idx[body]; id != "" {
		return id
	}
	return body
}

func rewriteRefs(it Item, ids idIndex) map[string]any {
	payload := dropReadOnly(it.Payload)
	switch it.Type {
	case TypeWebhook:
		if arr, ok := payload["securityFunctions"].([]any); ok {
			out := make([]any, len(arr))
			for i, raw := range arr {
				m, ok := asObject(raw)
				if !ok {
					out[i] = raw
					continue
				}
				cp := map[string]any{}
				for k, v := range m {
					cp[k] = v
				}
				if id := stringField(cp, "id"); id != "" {
					cp["id"] = ids.lookup(id, []string{TypeServerFunction})
				}
				out[i] = cp
			}
			payload["securityFunctions"] = out
		}
	case TypeJob:
		if arr, ok := payload["functions"].([]any); ok {
			out := make([]any, len(arr))
			for i, raw := range arr {
				m, ok := asObject(raw)
				if !ok {
					out[i] = raw
					continue
				}
				cp := map[string]any{}
				for k, v := range m {
					cp[k] = v
				}
				id := stringField(cp, "id")
				if id == "" {
					ctx, name := stringField(cp, "functionContext"), stringField(cp, "functionName")
					if ctx != "" && name != "" {
						id = ctx + "." + name
					}
				}
				if id != "" {
					cp["id"] = ids.lookup(id, []string{TypeServerFunction})
					delete(cp, "functionContext")
					delete(cp, "functionName")
				}
				out[i] = cp
			}
			payload["functions"] = out
		}
	case TypeTrigger:
		if dest, ok := asObject(payload["destination"]); ok {
			cp := map[string]any{}
			for k, v := range dest {
				cp[k] = v
			}
			ctx, name := stringField(cp, "functionContext"), stringField(cp, "functionName")
			if ctx != "" && name != "" {
				cp["serverFunctionId"] = ids.lookup(ctx+"."+name, []string{TypeServerFunction})
				delete(cp, "functionContext")
				delete(cp, "functionName")
			} else if id := stringField(cp, "serverFunctionId"); id != "" {
				cp["serverFunctionId"] = ids.lookup(id, []string{TypeServerFunction})
			}
			payload["destination"] = cp
		}
		if src, ok := asObject(payload["source"]); ok {
			cp := map[string]any{}
			for k, v := range src {
				cp[k] = v
			}
			ctx, name := stringField(cp, "webhookContext"), stringField(cp, "webhookName")
			if ctx != "" && name != "" {
				cp["webhookHandleId"] = ids.lookup(ctx+"."+name, []string{TypeWebhook})
				delete(cp, "webhookContext")
				delete(cp, "webhookName")
			} else if id := stringField(cp, "webhookHandleId"); id != "" {
				cp["webhookHandleId"] = ids.lookup(id, []string{TypeWebhook})
			}
			payload["source"] = cp
		}
	case TypeSubscription:
		if id := stringField(payload, "functionId"); id != "" {
			payload["functionId"] = ids.lookup(id, []string{TypeServerFunction})
		}
		if id := stringField(payload, "paramsSfxId"); id != "" {
			payload["paramsSfxId"] = ids.lookup(id, []string{TypeServerFunction})
		}
		if id := stringField(payload, "paramsVariableId"); id != "" {
			payload["paramsVariableId"] = ids.lookup(id, []string{TypeVariable})
		}
	}
	return payload
}

func (e *Engine) applyOne(it Item, remote map[string]any, dry bool, ids idIndex) (action string, changed []string, id string, err error) {
	it.Payload = rewriteRefs(it, ids)
	if dry && remote == nil {
		ids.put(it.Type, it.Context, it.Name, "<planned>")
	}
	local := dropReadOnly(it.Payload)
	it.Hash = ContentHash(local)
	if remote == nil {
		if dry {
			return ActionWouldCreate, []string{"new"}, "", nil
		}
		created, err := e.Client.Create(Collection(it.Type), outbound(it))
		if err != nil {
			return ActionFailed, nil, "", err
		}
		id = stringField(asObjectOrEmpty(created), "id")
		if e.Receipts {
			e.saveReceipt(it, id)
		}
		return ActionCreate, []string{"new"}, id, nil
	}
	id = stringField(remote, "id")
	if it.Type == TypeTable {
		detail, err := e.hydrateDetail(TypeTable, id, remote)
		if err != nil {
			return ActionFailed, nil, id, err
		}
		remote = tableOutbound(dropReadOnly(detail))
		local = tableOutbound(local)
		if lc, ok := local["columns"].([]any); ok {
			if rc, ok := remote["columns"].([]any); ok {
				remote["columns"] = projectTableColumns(rc, lc)
			}
		}
		it.Hash = ContentHash(local)
	}
	if it.Type == TypeWebhook {
		detail, err := e.hydrateDetail(TypeWebhook, id, remote)
		if err != nil {
			return ActionFailed, nil, id, err
		}
		remote = webhookOutbound(dropReadOnly(detail))
		local = webhookOutbound(local)
		it.Hash = ContentHash(local)
	}
	if it.Type == TypeTrigger {
		if triggerSourceDestChanged(local, remote) {
			return ActionFailed, nil, id, fail("a trigger's source or destination cannot be changed by an update; delete and recreate the trigger")
		}
		local = triggerComparable(local)
		remote = triggerComparable(remote)
		it.Hash = ContentHash(local)
	}
	if it.Type == TypeJob {
		local = jobOutbound(local)
		remote = jobOutbound(dropReadOnly(remote))
		if lf, ok := local["functions"].([]any); ok {
			if rf, ok := remote["functions"].([]any); ok {
				remote["functions"] = projectJobFunctions(rf, lf)
			}
		}
		it.Hash = ContentHash(local)
	}
	if it.Type == TypeSchema {
		detail, err := e.hydrateDetail(TypeSchema, id, remote)
		if err != nil {
			return ActionFailed, nil, id, err
		}
		remote = schemaOutbound(dropReadOnly(detail))
		local = schemaOutbound(local)
		it.Hash = ContentHash(local)
	}
	if it.Type == TypeSnippet {
		detail, err := e.hydrateDetail(TypeSnippet, id, remote)
		if err != nil {
			return ActionFailed, nil, id, err
		}
		remote = snippetOutbound(dropReadOnly(detail))
		local = snippetOutbound(local)
		it.Hash = ContentHash(local)
	}
	if it.Type == TypeApplication {
		detail, err := e.hydrateDetail(TypeApplication, id, remote)
		if err != nil {
			return ActionFailed, nil, id, err
		}
		remote = applicationOutbound(dropReadOnly(detail))
		local = applicationOutbound(local)
		it.Hash = ContentHash(local)
	}
	if it.Type == TypeSubscription {
		detail, err := e.hydrateDetail(TypeSubscription, id, remote)
		if err != nil {
			return ActionFailed, nil, id, err
		}
		remote = subscriptionOutbound(dropReadOnly(detail))
		local = projectSubscriptionSecrets(subscriptionOutbound(local), remote)
		it.Hash = ContentHash(local)
		it.Payload = local
	}
	proj := project(remote, local)
	remoteHash := ContentHash(proj)
	changed = changedFields(local, proj)
	equal := it.Hash == remoteHash || ContentHash(local) == remoteHash
	if it.Receipt != nil && it.Receipt.Hash != "" && it.Hash == it.Receipt.Hash && remoteHash != it.Receipt.Hash && !e.Force {
		return ActionBlocked, changed, id, fail("remote changed since last deploy; pull or pass --force")
	}
	if equal && !e.Force {
		if dry {
			return ActionSkip, nil, id, nil
		}
		return ActionSkip, nil, id, nil
	}
	if isObscured(it.Payload) && !e.Force {
		return ActionBlocked, changed, id, fail("existing obscured variable requires --force to update")
	}
	if dry {
		return ActionWouldUpdate, changed, id, nil
	}
	updateBody := outboundUpdate(it)
	if it.Type == TypeSubscription {
		updateBody = subscriptionPatch(local, remote)
		if len(updateBody) == 0 {
			return ActionSkip, nil, id, nil
		}
	}
	_, err = e.Client.Update(Collection(it.Type), id, updateBody, nil)
	if err != nil {
		return ActionFailed, changed, id, err
	}
	if e.Receipts {
		e.saveReceipt(it, id)
	}
	return ActionUpdate, changed, id, nil
}

func outbound(it Item) map[string]any {
	m := dropReadOnly(it.Payload)
	switch it.Type {
	case TypeTable:
		return tableOutbound(m)
	case TypeWebhook:
		return webhookOutbound(m)
	case TypeJob:
		return jobOutbound(m)
	case TypeSchema:
		return schemaOutbound(m)
	case TypeSnippet:
		return snippetOutbound(m)
	case TypeSubscription:
		return subscriptionOutbound(m)
	case TypeApplication:
		return applicationOutbound(m)
	default:
		return m
	}
}

func projectJobFunctions(remote, local []any) []any {
	if len(remote) != len(local) {
		return remote
	}
	out := make([]any, len(remote))
	for i := range local {
		lm, lok := asObject(local[i])
		rm, rok := asObject(remote[i])
		if !lok || !rok {
			out[i] = remote[i]
			continue
		}
		entry := map[string]any{}
		for k, tv := range lm {
			rv, ok := rm[k]
			if !ok {
				continue
			}
			entry[k] = projectValue(rv, tv)
		}
		out[i] = entry
	}
	return out
}

func outboundUpdate(it Item) map[string]any {
	if it.Type == TypeTrigger {
		m := map[string]any{}
		for _, k := range []string{"name", "waitForResponse", "enabled"} {
			if v, ok := it.Payload[k]; ok {
				m[k] = v
			}
		}
		return m
	}
	m := outbound(it)
	delete(m, "name")
	delete(m, "context")
	return m
}

func (e *Engine) hydrateDetail(typ, id string, listed map[string]any) (map[string]any, error) {
	if id == "" || e.Client == nil {
		return listed, nil
	}
	got, err := e.Client.Get(Collection(typ), id)
	if err != nil {
		return nil, err
	}
	if m, ok := asObject(got); ok {
		return m, nil
	}
	return listed, nil
}

func asObjectOrEmpty(v any) map[string]any {
	m, _ := asObject(v)
	if m == nil {
		return map[string]any{}
	}
	return m
}

func (e *Engine) listCol(cache map[string][]map[string]any, col string) ([]map[string]any, error) {
	if col == "" {
		return nil, fail("unknown collection")
	}
	if rows, ok := cache[col]; ok {
		return rows, nil
	}
	raw, err := e.Client.ListAll(col)
	if err != nil {
		if ae, ok := err.(*api.Error); ok && ae.StatusCode == 404 {
			cache[col] = []map[string]any{}
			return cache[col], nil
		}
		return nil, err
	}
	var rows []map[string]any
	for _, v := range raw {
		if m, ok := asObject(v); ok {
			rows = append(rows, m)
		}
	}
	cache[col] = rows
	return rows, nil
}

func findRemote(it Item, rows []map[string]any, ids idIndex) map[string]any {
	if it.Type == TypeTrigger {
		return findRemoteTrigger(it, rows, ids)
	}
	for _, row := range rows {
		if stringField(row, "name") != it.Name {
			continue
		}
		if it.Type == TypeJob {
			return row
		}
		if stringField(row, "context") == it.Context {
			return row
		}
	}
	return nil
}

func findRemoteTrigger(it Item, rows []map[string]any, ids idIndex) map[string]any {
	if it.Name != "" {
		for _, row := range rows {
			if stringField(row, "name") == it.Name {
				return row
			}
		}
	}
	want := triggerComparable(rewriteRefs(it, ids))
	wh := stringField(want, "webhookHandleId")
	fn := stringField(want, "serverFunctionId")
	ehPath := errorHandlerPath(want)
	if fn == "" && wh == "" && ehPath == "" {
		return nil
	}
	for _, row := range rows {
		got := triggerComparable(row)
		if fn != "" && stringField(got, "serverFunctionId") != fn {
			continue
		}
		if ehPath != "" {
			if errorHandlerPath(got) == ehPath {
				return row
			}
			continue
		}
		if wh != "" && stringField(got, "webhookHandleId") == wh {
			return row
		}
	}
	return nil
}

func (e *Engine) collectOrphans(local []Item, lists map[string][]map[string]any) ([]Result, error) {
	have := map[string]bool{}
	for _, it := range local {
		if it.LoadError == "" {
			have[it.Identity()] = true
		}
	}
	var out []Result
	seenTypes := map[string]bool{}
	for _, it := range local {
		seenTypes[it.Type] = true
	}
	for _, typ := range e.Scope.Types {
		seenTypes[typ] = true
	}
	for typ := range seenTypes {
		col := Collection(typ)
		if col == "" {
			continue
		}
		rows, err := e.listCol(lists, col)
		if err != nil {
			return out, err
		}
		for _, row := range rows {
			name := stringField(row, "name")
			ctx := stringField(row, "context")
			if !e.Scope.allowsContext(ctx) {
				continue
			}
			if excludedOrphanContext(e.Scope.ExcludeOrphanContexts, ctx) {
				continue
			}
			id := IdentityKey(typ, ctx, name)
			if have[id] {
				continue
			}
			res := Result{
				Type: typ, Name: name, Context: ctx,
				ID: stringField(row, "id"), Action: ActionOrphan,
			}
			if e.Receipts {
				if rec := e.receiptByIdentity(id); rec != nil {
					res.File = rec.File
					res.Error = "receipt"
				}
			}
			out = append(out, res)
		}
	}
	return out, nil
}

func excludedOrphanContext(prefixes []string, ctx string) bool {
	for _, p := range prefixes {
		if ctx == p || strings.HasPrefix(ctx, p+".") {
			return true
		}
	}
	return false
}

func (e *Engine) resumePath() string {
	poly := e.PolyPath
	if poly == "" {
		poly = ".poly"
	}
	return filepath.Join(e.Root, poly, "resume.json")
}

func (e *Engine) receiptsPath() string {
	poly := e.PolyPath
	if poly == "" {
		poly = ".poly"
	}
	return filepath.Join(e.Root, poly, "receipts.json")
}

type resumeFile struct {
	Keys []string `json:"keys"`
}

func (e *Engine) readResume() map[string]bool {
	raw, err := os.ReadFile(e.resumePath())
	if err != nil {
		return map[string]bool{}
	}
	var f resumeFile
	if json.Unmarshal(raw, &f) != nil {
		return map[string]bool{}
	}
	out := map[string]bool{}
	for _, k := range f.Keys {
		out[k] = true
	}
	return out
}

func (e *Engine) writeResume(key string) {
	done := e.readResume()
	done[key] = true
	var keys []string
	for k := range done {
		keys = append(keys, k)
	}
	b, _ := json.MarshalIndent(resumeFile{Keys: keys}, "", "  ")
	_ = os.MkdirAll(filepath.Dir(e.resumePath()), 0o755)
	_ = os.WriteFile(e.resumePath(), b, 0o644)
}

type receiptsFile struct {
	Version   int                           `json:"version"`
	Instances map[string]map[string]Receipt `json:"instances"`
}

func (e *Engine) loadReceiptsFile() receiptsFile {
	raw, err := os.ReadFile(e.receiptsPath())
	if err != nil {
		return receiptsFile{Version: 1, Instances: map[string]map[string]Receipt{}}
	}
	var f receiptsFile
	if json.Unmarshal(raw, &f) != nil || f.Instances == nil {
		return receiptsFile{Version: 1, Instances: map[string]map[string]Receipt{}}
	}
	return f
}

func (e *Engine) instanceKey() string {
	if e.Instance != "" {
		return e.Instance
	}
	return "default"
}

func (e *Engine) lookupReceipt(it Item) *Receipt {
	f := e.loadReceiptsFile()
	if m := f.Instances[e.instanceKey()]; m != nil {
		if r, ok := m[it.Identity()]; ok {
			cp := r
			return &cp
		}
	}
	if rec := parseReceiptComment(it.AbsFile); rec != nil {
		return rec
	}
	return nil
}

func (e *Engine) receiptByIdentity(id string) *Receipt {
	f := e.loadReceiptsFile()
	if m := f.Instances[e.instanceKey()]; m != nil {
		if r, ok := m[id]; ok {
			cp := r
			return &cp
		}
	}
	return nil
}

func (e *Engine) saveReceipt(it Item, id string) {
	f := e.loadReceiptsFile()
	if f.Instances == nil {
		f.Instances = map[string]map[string]Receipt{}
	}
	inst := e.instanceKey()
	if f.Instances[inst] == nil {
		f.Instances[inst] = map[string]Receipt{}
	}
	rec := Receipt{
		Instance:   inst,
		ID:         id,
		Hash:       it.Hash,
		DeployedAt: time.Now().UTC().Format(time.RFC3339),
		Type:       it.Type,
		File:       it.File,
	}
	f.Instances[inst][it.Identity()] = rec
	b, _ := json.MarshalIndent(f, "", "  ")
	_ = os.MkdirAll(filepath.Dir(e.receiptsPath()), 0o755)
	_ = os.WriteFile(e.receiptsPath(), b, 0o644)
	_ = writeReceiptComment(it.AbsFile, rec, DisplayName(it.Context, it.Name))
}

func (e *Engine) removeReceipt(r Result) {
	f := e.loadReceiptsFile()
	inst := e.instanceKey()
	if m := f.Instances[inst]; m != nil {
		delete(m, r.Identity())
		b, _ := json.MarshalIndent(f, "", "  ")
		_ = os.WriteFile(e.receiptsPath(), b, 0o644)
	}
}

func parseReceiptComment(path string) *Receipt {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	line := firstLine(string(raw))
	const mark = "Poly deployed @"
	i := strings.Index(line, mark)
	if i < 0 {
		return nil
	}
	rest := strings.TrimSpace(line[i+len(mark):])
	parts := strings.Split(rest, " - ")
	if len(parts) < 3 {
		return nil
	}
	return &Receipt{
		DeployedAt: strings.TrimSpace(parts[0]),
		Instance:   strings.TrimSpace(parts[len(parts)-2]),
		Hash:       strings.TrimSpace(parts[len(parts)-1]),
	}
}

func writeReceiptComment(path string, rec Receipt, label string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	prefix := commentPrefix(path)
	header := prefix + " Poly deployed @ " + rec.DeployedAt + " - " + label + " - " + rec.Instance + " - " + rec.Hash
	text := string(raw)
	if strings.Contains(firstLine(text), "Poly deployed @") {
		nl := strings.Index(text, "\n")
		if nl < 0 {
			text = header + "\n"
		} else {
			text = header + text[nl:]
		}
	} else {
		text = header + "\n" + text
	}
	return os.WriteFile(path, []byte(text), 0o644)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
