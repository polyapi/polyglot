package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/polyapi/polyglot/src/api"
	"github.com/polyapi/polyglot/src/config"
	"github.com/polyapi/polyglot/src/delegate"
	"github.com/polyapi/polyglot/src/glide"
	"github.com/spf13/cobra"
)

const executeHTTPTimeout = 10 * time.Minute

var functionKindOrder = []string{"ai", "api", "client", "server"}

type functionRef struct {
	Kind       string
	Collection string
	ID         string
	Name       string
	Context    string
	Visibility string
	Spec       any
}

func addFunctionCommands(parent *cobra.Command) {
	fn := &cobra.Command{
		Use:   "function",
		Short: "Manage and execute functions",
		Example: examples(
			ex{"List functions in a context:", "polyapi function list --context billing"},
			ex{"Scaffold a local function file:", "polyapi function init --name helloWorld --type server"},
			ex{"Add a server function from a source file:", "polyapi function add echo echo.ts --type server --context billing"},
			ex{"Execute a function:", "polyapi function execute echo --context billing"},
		),
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "List functions",
		Args:  cobra.NoArgs,
		RunE:  runFunctionList,
		Example: examples(
			ex{"List all functions:", "polyapi function list"},
			ex{"Restrict to a context:", "polyapi function list --context billing"},
			ex{"Server functions only:", "polyapi function list --type server --context billing"},
		),
	}
	list.Flags().String("context", "", "Restrict results to this context prefix")
	list.Flags().Var(&functionTypeValue{}, "type", "Function type (server|client|api|ai)")

	get := &cobra.Command{
		Use:   "get <id-or-name>",
		Short: "Get a function by ID, or by name with --context",
		Args:  cobra.ExactArgs(1),
		RunE:  runFunctionGet,
		Example: examples(
			ex{"Get by ID:", "polyapi function get abc123"},
			ex{"Get by context and name:", "polyapi function get echo --context billing --type server"},
		),
	}
	get.Flags().String("context", "", "Context of the function (required when looking up by name)")
	get.Flags().Var(&functionTypeValue{}, "type", "Function type (server|client|api|ai)")

	add := newFunctionAddCommand(
		"add <name> <file>",
		"Add or update a function from a source file (language adapter)",
	)
	update := newFunctionAddCommand(
		"update <name> <file>",
		"Add or update a function from a source file (same as add)",
	)

	del := &cobra.Command{
		Use:   "delete <id-or-name>",
		Short: "Delete a function by ID, or by name with --context",
		Args:  cobra.ExactArgs(1),
		RunE:  runFunctionDelete,
		Example: examples(
			ex{"Delete by ID:", "polyapi function delete abc123"},
			ex{"Delete by context and name:", "polyapi function delete echo --context billing --type server"},
		),
	}
	del.Flags().String("context", "", "Context of the function (required when deleting by name)")
	del.Flags().Var(&functionTypeValue{}, "type", "Function type (server|client|api|ai)")

	exec := &cobra.Command{
		Use:   "execute <id-or-name> [args]...",
		Short: "Execute a function with the provided arguments",
		Args:  cobra.MinimumNArgs(1),
		RunE:  runFunctionExecute,
		Example: examples(
			ex{"Execute by name:", "polyapi function execute echo --context billing"},
			ex{"Pass a JSON object:", `polyapi function execute echo --context billing --data '{"text":"hi"}'`},
			ex{"Positional args map onto the function signature:", "polyapi function execute echo --context billing hello"},
		),
	}
	exec.Flags().String("context", "", "Context of the function (required when executing by name)")
	exec.Flags().Var(&functionTypeValue{}, "type", "Function type (server|client|api|ai)")
	exec.Flags().String("data", "", "JSON object of argument names to values")

	logs := &cobra.Command{
		Use:   "logs <id-or-name>",
		Short: "Get or delete execution logs for a server or AI function",
		Args:  cobra.ExactArgs(1),
		RunE:  runFunctionLogs,
		Example: examples(
			ex{"Show recent logs:", "polyapi function logs echo --context billing --type server"},
			ex{"Filter and limit:", "polyapi function logs abc123 --keyword error --limit 20 --last-hours 24"},
			ex{"Clear stored logs:", "polyapi function logs abc123 --delete"},
		),
	}
	logs.Flags().String("context", "", "Context of the function (required when looking up by name)")
	logs.Flags().Var(&functionTypeValue{}, "type", "Function type (server|client|api|ai)")
	logs.Flags().Bool("delete", false, "Delete stored logs instead of fetching them")
	logs.Flags().Bool("system", false, "Use system logs (container + execution) instead of execution logs")
	logs.Flags().String("keyword", "", "Return only entries containing this text")
	logs.Flags().Uint("last-hours", 0, "Look back this many hours")
	logs.Flags().Uint("last-days", 0, "Look back this many days")
	logs.Flags().Uint("limit", 0, "Maximum number of log entries")
	logs.Flags().String("execution-id", "", "Filter logs to a specific execution")

	initCmd := newResourceInitCommand(resourceInitKind{
		Use: "function", Noun: "function", NeedsContext: true, TypeMode: initTypeFunction,
	})

	fn.AddCommand(list, get, initCmd, add, update, del, exec, logs)
	parent.AddCommand(fn)
}

func newFunctionAddCommand(use, short string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(2),
		RunE:  runFunctionAdd,
		Example: examples(
			ex{"Add a server function:", "polyapi function add echo echo.ts --type server --context billing"},
			ex{"Add a client function:", "polyapi function add greet greet.ts --type client --context billing --skip-generate"},
			ex{"Set visibility and description:", "polyapi function add echo echo.ts --type server --context billing --visibility TENANT --description \"Echo input\""},
		),
	}
	cmd.Flags().String("context", "", "Context of the function")
	cmd.Flags().String("description", "", "Description of the function")
	cmd.Flags().Var(&functionTypeValue{}, "type", "Function type (server|client)")
	cmd.Flags().String("logs", "", "Server functions only: `enabled` or `disabled`")
	cmd.Flags().Bool("skip-generate", false, "Skip `generate` after a successful add")
	cmd.Flags().String("execution-api-key", "", "Optional API key for server functions")
	cmd.Flags().String("image", "", "Server functions only: Docker image to run the function in")
	cmd.Flags().String("generate-contexts", "", "Server functions only: restrict generated contexts")
	cmd.Flags().String("visibility", "ENVIRONMENT", "Visibility: PUBLIC, TENANT, or ENVIRONMENT (case insensitive)")
	_ = cmd.MarkFlagRequired("type")
	return cmd
}

func runFunctionList(cmd *cobra.Command, _ []string) error {
	client, _, err := functionClient(cmd)
	if err != nil {
		return err
	}
	kind, err := optionalFunctionKind(cmd)
	if err != nil {
		return err
	}
	context, _ := cmd.Flags().GetString("context")
	kinds := functionKindOrder
	if kind != "" {
		kinds = []string{kind}
	}
	var rows []functionRef
	for _, k := range kinds {
		items, err := client.ListAll(functionCollection(k))
		if err != nil {
			if isNotFound(err) {
				continue
			}
			return fail(err)
		}
		for _, item := range items {
			ref := refFromItem(item, k)
			if context != "" && !strings.HasPrefix(ref.Context, context) {
				continue
			}
			rows = append(rows, ref)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Kind != rows[j].Kind {
			return functionKindIndex(rows[i].Kind) < functionKindIndex(rows[j].Kind)
		}
		li := glide.DisplayName(rows[i].Context, rows[i].Name)
		lj := glide.DisplayName(rows[j].Context, rows[j].Name)
		if li != lj {
			return li < lj
		}
		return rows[i].ID < rows[j].ID
	})
	out := cmd.OutOrStdout()
	if len(rows) == 0 {
		if !globalsFrom(cmd).Quiet {
			fmt.Fprintln(out, "no functions found")
		}
		return nil
	}
	printFunctionList(out, rows)
	return nil
}

func printFunctionList(w io.Writer, rows []functionRef) {
	typeW, nameW, visW := len("TYPE"), len("NAME"), len("VISIBILITY")
	for _, row := range rows {
		if n := len(row.Kind); n > typeW {
			typeW = n
		}
		name := glide.DisplayName(row.Context, row.Name)
		if n := len(name); n > nameW {
			nameW = n
		}
		if n := len(visibilityLabel(row.Visibility)); n > visW {
			visW = n
		}
	}
	fmt.Fprintln(w, infoText(fmt.Sprintf("%-*s  %-*s  %-*s  %s", typeW, "TYPE", nameW, "NAME", visW, "VISIBILITY", "ID")))
	for _, row := range rows {
		fmt.Fprintf(w, "%-*s  %-*s  %-*s  %s\n", typeW, row.Kind, nameW, glide.DisplayName(row.Context, row.Name), visW, visibilityLabel(row.Visibility), row.ID)
	}
}

func visibilityLabel(v string) string {
	if v == "" {
		return "-"
	}
	return v
}

func runFunctionGet(cmd *cobra.Command, args []string) error {
	client, _, err := functionClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveFunction(cmd, client, args[0])
	if err != nil {
		return err
	}
	return writeJSON(cmd.OutOrStdout(), ref.Spec)
}

func runFunctionDelete(cmd *cobra.Command, args []string) error {
	client, _, err := functionClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveFunction(cmd, client, args[0])
	if err != nil {
		return err
	}
	if err := client.Delete(ref.Collection, ref.ID, nil); err != nil {
		return fail(err)
	}
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("deleted %s function %s (%s)", ref.Kind, glide.DisplayName(ref.Context, ref.Name), ref.ID))
	}
	return nil
}

func runFunctionExecute(cmd *cobra.Command, args []string) error {
	client, _, err := functionClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveFunction(cmd, client, args[0])
	if err != nil {
		return err
	}
	if ref.Kind == "client" {
		return failUsage("client functions have no HTTP execute route; call the generated SDK (poly." + glide.DisplayName(ref.Context, ref.Name) + ") or invoke them from a server function")
	}
	dataFlag, _ := cmd.Flags().GetString("data")
	payload, err := executePayload(ref.Spec, dataFlag, args[1:])
	if err != nil {
		return err
	}
	result, err := client.WithTimeout(executeHTTPTimeout).PostAction(ref.Collection, ref.ID, "execute", payload)
	if err != nil {
		return fail(err)
	}
	if result == nil {
		return nil
	}
	return writeJSON(cmd.OutOrStdout(), result)
}

func runFunctionLogs(cmd *cobra.Command, args []string) error {
	client, _, err := functionClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveFunction(cmd, client, args[0])
	if err != nil {
		return err
	}
	if ref.Kind != "server" && ref.Kind != "ai" {
		return failUsage("logs are only available for server and ai functions")
	}
	action := "logs"
	if sys, _ := cmd.Flags().GetBool("system"); sys {
		action = "system-logs"
	}
	if del, _ := cmd.Flags().GetBool("delete"); del {
		if err := client.DeleteAction(ref.Collection, ref.ID, action); err != nil {
			return fail(err)
		}
		if !globalsFrom(cmd).Quiet {
			printOk(cmd.OutOrStdout(), fmt.Sprintf("deleted %s for %s function %s", action, ref.Kind, ref.ID))
		}
		return nil
	}
	query := logsQuery(cmd)
	value, err := client.GetQuery(ref.Collection, joinAction(ref.ID, action), query)
	if err != nil {
		return fail(err)
	}
	return printFunctionLogs(cmd.OutOrStdout(), value, globalsFrom(cmd).Quiet)
}

func runFunctionAdd(cmd *cobra.Command, args []string) error {
	kind := strings.ToLower(cmd.Flags().Lookup("type").Value.String())
	if kind != "server" && kind != "client" {
		return failUsage("`function add` only supports --type server or --type client; use Glide or OpenAPI train for api/ai functions")
	}
	if err := validateServerOnlyFlags(cmd, kind); err != nil {
		return err
	}
	name := args[0]
	file := args[1]
	abs, err := filepath.Abs(file)
	if err != nil {
		return fail(err)
	}
	code, err := os.ReadFile(abs)
	if err != nil {
		return fail(&ExitError{Code: exitUsage, Msg: fmt.Sprintf("read %s: %v", file, err)})
	}

	g := globalsFrom(cmd)
	cfg, err := loadDelegateConfig(g)
	if err != nil {
		return fail(err)
	}
	if _, _, err := cfg.RequireCredentials(); err != nil {
		return fail(err)
	}
	adapter, err := resolveAdapter(g, cfg)
	if err != nil {
		return failDelegate(cmd, err)
	}
	del := delegate.New(adapter.Root, adapter.Lang, adapter.Argv, g.PolyPath)
	parsedRaw, err := del.ParseFunction(delegate.ParseFunctionParams{
		File: abs,
		Name: name,
		Kind: delegate.FunctionKind(kind),
	})
	if err != nil {
		return failDelegate(cmd, err)
	}
	payload, err := overlayFunctionDTO(parsedRaw, cmd, name, adapter.Lang.String(), string(code))
	if err != nil {
		return err
	}

	client, err := api.FromConfig(cfg)
	if err != nil {
		return fail(err)
	}
	created, err := client.Create(functionCollection(kind), payload)
	if err != nil {
		return fail(err)
	}
	id := mapString(created, "id")
	if !g.Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("deployed %s function %s (%s)", kind, glide.DisplayName(mapString(payload, "context"), name), id))
	}
	skip, _ := cmd.Flags().GetBool("skip-generate")
	if skip {
		if !g.Quiet {
			printWarning(cmd.OutOrStdout(), "flag --skip-generate received; skipping generate")
		}
		return nil
	}
	return generateAfterFunction(cmd, g, adapter, cfg, id)
}

func generateAfterFunction(cmd *cobra.Command, g Globals, adapter delegate.ResolvedAdapter, cfg config.Resolved, id string) error {
	client, err := api.FromConfig(cfg)
	if err != nil {
		return fail(err)
	}
	var ids []string
	if id != "" {
		ids = []string{id}
	}
	return generateWithClient(cmd, g, adapter, client, nil, nil, ids, false)
}

func overlayFunctionDTO(raw json.RawMessage, cmd *cobra.Command, name, lang, code string) (map[string]any, error) {
	payload := map[string]any{}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, fail(&ExitError{Code: exitFail, Msg: "adapter parse_function result is not an object: " + err.Error()})
		}
	}
	delete(payload, "kind")
	delete(payload, "type")
	payload["name"] = name
	if mapString(payload, "language") == "" && lang != "" {
		payload["language"] = lang
	}
	if mapString(payload, "code") == "" {
		payload["code"] = code
	}
	if cmd.Flags().Changed("context") || mapString(payload, "context") == "" {
		ctx, _ := cmd.Flags().GetString("context")
		payload["context"] = ctx
	}
	if cmd.Flags().Changed("description") {
		desc, _ := cmd.Flags().GetString("description")
		payload["description"] = desc
	}
	vis, _ := cmd.Flags().GetString("visibility")
	if vis == "" {
		vis = "ENVIRONMENT"
	}
	vis = strings.ToUpper(vis)
	switch vis {
	case "PUBLIC", "TENANT", "ENVIRONMENT":
		payload["visibility"] = vis
	default:
		return nil, failUsage("option `visibility` must be PUBLIC, TENANT, or ENVIRONMENT")
	}
	if cmd.Flags().Changed("logs") {
		logs, _ := cmd.Flags().GetString("logs")
		switch logs {
		case "enabled":
			payload["logsEnabled"] = true
		case "disabled":
			payload["logsEnabled"] = false
		default:
			return nil, failUsage("invalid value for `logs`; use enabled or disabled")
		}
	}
	if cmd.Flags().Changed("image") {
		image, _ := cmd.Flags().GetString("image")
		payload["image"] = image
	}
	if cmd.Flags().Changed("generate-contexts") {
		rawCtx, _ := cmd.Flags().GetString("generate-contexts")
		var ctxs []string
		for _, part := range strings.Split(rawCtx, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				ctxs = append(ctxs, part)
			}
		}
		payload["generateContexts"] = ctxs
	}
	if cmd.Flags().Changed("execution-api-key") {
		key, _ := cmd.Flags().GetString("execution-api-key")
		payload["executionApiKey"] = key
	}
	return payload, nil
}

func validateServerOnlyFlags(cmd *cobra.Command, kind string) error {
	if kind == "server" {
		if logs, _ := cmd.Flags().GetString("logs"); logs != "" && logs != "enabled" && logs != "disabled" {
			return failUsage("invalid value for `logs`; use enabled or disabled")
		}
		return nil
	}
	if cmd.Flags().Changed("logs") {
		return failUsage("option `logs` is only for server functions (--type server)")
	}
	if cmd.Flags().Changed("image") {
		return failUsage("option `image` is only for server functions (--type server)")
	}
	if cmd.Flags().Changed("generate-contexts") {
		return failUsage("option `generate-contexts` is only for server functions (--type server)")
	}
	if cmd.Flags().Changed("execution-api-key") {
		return failUsage("option `execution-api-key` is only for server functions (--type server)")
	}
	return nil
}

func functionClient(cmd *cobra.Command) (*api.HTTPClient, config.Resolved, error) {
	g := globalsFrom(cmd)
	cfg, err := loadDelegateConfig(g)
	if err != nil {
		return nil, cfg, fail(err)
	}
	if _, _, err := cfg.RequireCredentials(); err != nil {
		return nil, cfg, fail(err)
	}
	client, err := api.FromConfig(cfg)
	if err != nil {
		return nil, cfg, fail(err)
	}
	return client, cfg, nil
}

func optionalFunctionKind(cmd *cobra.Command) (string, error) {
	f := cmd.Flags().Lookup("type")
	if f == nil || !cmd.Flags().Changed("type") {
		return "", nil
	}
	return parseFunctionKind(f.Value.String())
}

func parseFunctionKind(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	typ, ok := glide.NormalizeType(s)
	if !ok || !glide.IsFunction(typ) {
		return "", failUsage(fmt.Sprintf("invalid function type %q (server|client|api|ai)", s))
	}
	return functionKindName(typ), nil
}

func functionKindName(typ string) string {
	switch typ {
	case glide.TypeAIFunction:
		return "ai"
	case glide.TypeAPIFunction:
		return "api"
	case glide.TypeClientFunction:
		return "client"
	case glide.TypeServerFunction:
		return "server"
	default:
		return ""
	}
}

func functionCollection(kind string) string {
	typ, ok := glide.NormalizeType(kind)
	if !ok {
		return ""
	}
	return glide.Collection(typ)
}

func functionKindIndex(kind string) int {
	for i, k := range functionKindOrder {
		if k == kind {
			return i
		}
	}
	return len(functionKindOrder)
}

func resolveFunction(cmd *cobra.Command, client *api.HTTPClient, idOrName string) (functionRef, error) {
	kind, err := optionalFunctionKind(cmd)
	if err != nil {
		return functionRef{}, err
	}
	context, _ := cmd.Flags().GetString("context")
	if context != "" {
		return lookupFunctionByName(client, kind, context, idOrName)
	}
	return lookupFunctionByID(client, kind, idOrName)
}

func lookupFunctionByID(client *api.HTTPClient, kind, id string) (functionRef, error) {
	kinds := functionKindOrder
	if kind != "" {
		kinds = []string{kind}
	}
	var notFound error
	for _, k := range kinds {
		spec, err := client.Get(functionCollection(k), id)
		if err != nil {
			if isNotFound(err) {
				notFound = err
				continue
			}
			return functionRef{}, fail(err)
		}
		ref := refFromItem(spec, k)
		ref.ID = id
		ref.Collection = functionCollection(k)
		ref.Kind = k
		ref.Spec = spec
		return ref, nil
	}
	if notFound != nil {
		return functionRef{}, fail(notFound)
	}
	return functionRef{}, failUsage(fmt.Sprintf("function %s not found", id))
}

func lookupFunctionByName(client *api.HTTPClient, kind, context, name string) (functionRef, error) {
	kinds := functionKindOrder
	if kind != "" {
		kinds = []string{kind}
	}
	var matches []functionRef
	for _, k := range kinds {
		items, err := client.ListAll(functionCollection(k))
		if err != nil {
			if isNotFound(err) {
				continue
			}
			return functionRef{}, fail(err)
		}
		var sameCase []functionRef
		var anyCase []functionRef
		for _, item := range items {
			ref := refFromItem(item, k)
			if !strings.EqualFold(ref.Name, name) || !strings.EqualFold(ref.Context, context) {
				continue
			}
			anyCase = append(anyCase, ref)
			if ref.Name == name && ref.Context == context {
				sameCase = append(sameCase, ref)
			}
		}
		switch {
		case len(sameCase) == 1:
			matches = append(matches, sameCase[0])
		case len(sameCase) > 1:
			return functionRef{}, failUsage(unclearFunction(context, name, sameCase))
		case len(anyCase) == 1:
			matches = append(matches, anyCase[0])
		case len(anyCase) > 1:
			return functionRef{}, failUsage(unclearFunction(context, name, anyCase))
		}
	}
	switch len(matches) {
	case 0:
		return functionRef{}, failUsage(fmt.Sprintf("no function named %q in context %q", name, context))
	case 1:
		ref := matches[0]
		if ref.ID == "" {
			return ref, nil
		}
		spec, err := client.Get(ref.Collection, ref.ID)
		if err != nil {
			return functionRef{}, fail(err)
		}
		full := refFromItem(spec, ref.Kind)
		full.ID = ref.ID
		full.Collection = ref.Collection
		full.Kind = ref.Kind
		full.Spec = spec
		return full, nil
	default:
		return functionRef{}, failUsage(unclearFunction(context, name, matches) + "; pass --type")
	}
}

func unclearFunction(context, name string, matches []functionRef) string {
	parts := make([]string, 0, len(matches))
	for _, m := range matches {
		parts = append(parts, fmt.Sprintf("%s %s (%s)", m.Kind, glide.DisplayName(m.Context, m.Name), m.ID))
	}
	return fmt.Sprintf("unclear function reference %s; matches: %s", glide.DisplayName(context, name), strings.Join(parts, ", "))
}

func refFromItem(item any, fallbackKind string) functionRef {
	m, _ := item.(map[string]any)
	kind := kindFromAPIType(mapString(m, "type"), fallbackKind)
	if kind == "" {
		kind = fallbackKind
	}
	return functionRef{
		Kind:       kind,
		Collection: functionCollection(kind),
		ID:         firstMapString(m, "id", "functionId"),
		Name:       mapString(m, "name"),
		Context:    mapString(m, "context"),
		Visibility: mapString(m, "visibility"),
		Spec:       item,
	}
}

func kindFromAPIType(t, fallback string) string {
	if t == "" {
		return fallback
	}
	switch strings.ToLower(strings.ReplaceAll(t, "_", "")) {
	case "server", "serverfunction", "server-function", "functions/server":
		return "server"
	case "client", "clientfunction", "customfunction", "client-function", "functions/client":
		return "client"
	case "api", "apifunction", "api-function", "functions/api":
		return "api"
	case "ai", "aifunction", "ai-function", "functions/ai":
		return "ai"
	}
	if typ, ok := glide.NormalizeType(t); ok && glide.IsFunction(typ) {
		return functionKindName(typ)
	}
	return fallback
}

func executePayload(spec any, dataFlag string, extra []string) (any, error) {
	if dataFlag != "" {
		if len(extra) > 0 {
			return nil, failUsage("do not pass both --data and positional arguments")
		}
		var v any
		if err := json.Unmarshal([]byte(dataFlag), &v); err != nil {
			return nil, failUsage("invalid --data JSON: " + err.Error())
		}
		if _, ok := v.(map[string]any); !ok {
			return nil, failUsage("--data must be a JSON object of argument names to values")
		}
		return v, nil
	}
	if len(extra) == 0 {
		return map[string]any{}, nil
	}
	if len(extra) == 1 {
		var v any
		if err := json.Unmarshal([]byte(extra[0]), &v); err == nil {
			if _, ok := v.(map[string]any); ok {
				return v, nil
			}
		}
	}
	keys := argumentKeys(spec)
	if len(keys) == 0 {
		return nil, failUsage("pass --data as a JSON object; this function has no named arguments to bind")
	}
	if len(extra) > len(keys) {
		return nil, failUsage(fmt.Sprintf("got %d arguments, function has %d", len(extra), len(keys)))
	}
	out := map[string]any{}
	for i, raw := range extra {
		out[keys[i]] = parseJSONOrString(raw)
	}
	return out, nil
}

func argumentKeys(spec any) []string {
	m, _ := spec.(map[string]any)
	if m == nil {
		return nil
	}
	arr, _ := m["arguments"].([]any)
	var keys []string
	for _, item := range arr {
		im, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if k := firstMapString(im, "key", "name"); k != "" {
			keys = append(keys, k)
		}
	}
	return keys
}

func parseJSONOrString(s string) any {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err == nil {
		return v
	}
	return s
}

func logsQuery(cmd *cobra.Command) [][2]string {
	var q [][2]string
	if v, _ := cmd.Flags().GetString("keyword"); v != "" {
		q = append(q, [2]string{"keyword", v})
	}
	if v, _ := cmd.Flags().GetUint("last-hours"); v > 0 {
		q = append(q, [2]string{"lastHours", fmt.Sprintf("%d", v)})
	}
	if v, _ := cmd.Flags().GetUint("last-days"); v > 0 {
		q = append(q, [2]string{"lastDays", fmt.Sprintf("%d", v)})
	}
	if v, _ := cmd.Flags().GetUint("limit"); v > 0 {
		q = append(q, [2]string{"limit", fmt.Sprintf("%d", v)})
	}
	if v, _ := cmd.Flags().GetString("execution-id"); v != "" {
		q = append(q, [2]string{"executionId", v})
	}
	return q
}

func printFunctionLogs(w io.Writer, value any, quiet bool) error {
	entries, ok := extractLogEntries(value)
	if !ok {
		return writeJSON(w, value)
	}
	if len(entries) == 0 {
		if !quiet {
			fmt.Fprintln(w, "no logs found")
		}
		return nil
	}
	for _, entry := range entries {
		fmt.Fprintln(w, formatLogMeta(entry))
		fmt.Fprintln(w, strings.TrimRight(formatLogValue(entry["value"]), "\n"))
		fmt.Fprintln(w)
	}
	return nil
}

func extractLogEntries(value any) ([]map[string]any, bool) {
	if value == nil {
		return nil, false
	}
	if m, ok := value.(map[string]any); ok {
		raw, has := m["logs"]
		if !has {
			return nil, false
		}
		if raw == nil {
			return []map[string]any{}, true
		}
		return asLogEntries(raw)
	}
	return asLogEntries(value)
}

func asLogEntries(raw any) ([]map[string]any, bool) {
	switch logs := raw.(type) {
	case []any:
		out := make([]map[string]any, 0, len(logs))
		for _, item := range logs {
			entry, ok := item.(map[string]any)
			if !ok {
				return nil, false
			}
			out = append(out, entry)
		}
		return out, true
	case []map[string]any:
		return logs, true
	default:
		return nil, false
	}
}

func formatLogMeta(entry map[string]any) string {
	if meta := firstMapString(entry, "meta"); meta != "" && stringifyLogField(entry["executionId"]) == "" && stringifyLogField(entry["level"]) == "" {
		return infoText(meta)
	}
	var parts []string
	if id := stringifyLogField(entry["executionId"]); id != "" {
		parts = append(parts, paint(Stone500, false, id))
	}
	if level := stringifyLogField(entry["level"]); level != "" {
		parts = append(parts, paintLogLevel(level))
	}
	if ts := stringifyLogField(entry["timestamp"]); ts != "" {
		parts = append(parts, paint(Stone500, false, ts))
	}
	if rev := stringifyLogField(entry["revision"]); rev != "" {
		parts = append(parts, paint(Stone500, false, rev))
	}
	return strings.Join(parts, paint(Stone600, false, " • "))
}

func paintLogLevel(level string) string {
	switch strings.ToUpper(level) {
	case "ERROR", "ERR", "FATAL", "PANIC", "CRITICAL":
		return paint(Macaw600, true, level)
	case "WARN", "WARNING":
		return paint(Sunray600, true, level)
	case "INFO", "INFORMATION", "DEBUG", "TRACE", "VERBOSE":
		return paint(Stone500, true, level)
	default:
		return paint(Stone500, true, level)
	}
}

func stringifyLogField(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func formatLogValue(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return formatLogString(t)
	default:
		if pretty, ok := prettyJSONValue(t); ok {
			return pretty
		}
		return fmt.Sprint(t)
	}
}

func formatLogString(s string) string {
	if pretty, ok := tryPrettyJSON(s); ok {
		return pretty
	}
	trimmed := strings.TrimSpace(s)
	if !strings.HasPrefix(trimmed, `"`) {
		return s
	}
	dec := json.NewDecoder(strings.NewReader(trimmed))
	dec.UseNumber()
	var inner string
	if err := dec.Decode(&inner); err != nil || dec.More() {
		return s
	}
	if pretty, ok := tryPrettyJSON(inner); ok {
		return pretty
	}
	return s
}

func tryPrettyJSON(s string) (string, bool) {
	trimmed := strings.TrimSpace(s)
	if len(trimmed) < 2 {
		return "", false
	}
	switch trimmed[0] {
	case '{', '[':
	default:
		return "", false
	}
	dec := json.NewDecoder(strings.NewReader(trimmed))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return "", false
	}
	if dec.More() {
		return "", false
	}
	return prettyJSONValue(v)
}

func prettyJSONValue(v any) (string, bool) {
	if !isJSONObjectOrArray(v) {
		return "", false
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", false
	}
	return string(b), true
}

func isJSONObjectOrArray(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return true
	default:
		return false
	}
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fail(err)
	}
	return nil
}

func isNotFound(err error) bool {
	var ae *api.Error
	return errors.As(err, &ae) && ae.Status() == 404
}

func mapString(v any, key string) string {
	m, _ := v.(map[string]any)
	return firstMapString(m, key)
}

func firstMapString(m map[string]any, keys ...string) string {
	if m == nil {
		return ""
	}
	for _, key := range keys {
		if s, ok := m[key].(string); ok {
			return s
		}
	}
	return ""
}

func joinAction(id, action string) string {
	return strings.Trim(id, "/") + "/" + strings.Trim(action, "/")
}
