package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/polyapi/polyglot/src/api"
	"github.com/polyapi/polyglot/src/glide"
	"github.com/spf13/cobra"
)

const webhookCollection = "webhooks"

var uuidLike = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type webhookRef struct {
	ID         string
	Name       string
	Context    string
	Visibility string
	URL        string
	Spec       any
}

func addWebhookCommands(root *cobra.Command) {
	wh := &cobra.Command{
		Use:   "webhook",
		Short: "Manage and test webhooks",
		Example: examples(
			ex{"List webhooks in a context:", "polyapi webhook list --context billing"},
			ex{"Scaffold a local webhook file:", "polyapi webhook init --name hook"},
			ex{"Create a webhook:", "polyapi webhook create --name hook --context billing --event-payload '{\"n\":3}'"},
			ex{"Test a webhook:", `polyapi webhook test hook --context billing --data '{"n":5}'`},
		),
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "List webhooks",
		Args:  cobra.NoArgs,
		RunE:  runWebhookList,
		Example: examples(
			ex{"List all webhooks:", "polyapi webhook list"},
			ex{"Restrict to a context:", "polyapi webhook list --context billing"},
		),
	}
	list.Flags().String("context", "", "Restrict results to this context prefix")

	get := &cobra.Command{
		Use:   "get <id-or-name>",
		Short: "Get a webhook by ID, or by name with --context",
		Args:  cobra.ExactArgs(1),
		RunE:  runWebhookGet,
		Example: examples(
			ex{"Get by ID:", "polyapi webhook get abc123"},
			ex{"Get by context and name:", "polyapi webhook get hook --context billing"},
		),
	}
	get.Flags().String("context", "", "Context of the webhook (required when looking up by name)")

	create := &cobra.Command{
		Use:   "create",
		Short: "Create a webhook",
		Args:  cobra.NoArgs,
		RunE:  runWebhookCreate,
		Example: examples(
			ex{"Create a POST webhook:", `polyapi webhook create --name hook --context billing --event-payload '{"n":3}' --response-payload '{"ok":true}'`},
			ex{"Require a Poly API key:", "polyapi webhook create --name hook --context billing --require-api-key"},
			ex{"Attach a security function:", "polyapi webhook create --name hook --context billing --security-function billing.hasValidCode"},
		),
	}
	addWebhookWriteFlags(create, true)
	_ = create.MarkFlagRequired("name")
	_ = create.MarkFlagRequired("context")

	update := &cobra.Command{
		Use:   "update <id-or-name>",
		Short: "Update a webhook by ID, or by name with --context",
		Args:  cobra.ExactArgs(1),
		RunE:  runWebhookUpdate,
		Example: examples(
			ex{"Update the response:", `polyapi webhook update hook --context billing --response-payload '{"ok":true}'`},
			ex{"Set security functions:", `polyapi webhook update hook --context billing --security-function billing.hasValidCode`},
		),
	}
	update.Flags().String("context", "", "Context of the webhook (required when looking up by name)")
	addWebhookWriteFlags(update, false)
	update.Flags().String("new-context", "", "Move the webhook to this context")

	del := &cobra.Command{
		Use:   "delete <id-or-name>",
		Short: "Delete a webhook by ID, or by name with --context",
		Args:  cobra.ExactArgs(1),
		RunE:  runWebhookDelete,
		Example: examples(
			ex{"Delete by ID:", "polyapi webhook delete abc123"},
			ex{"Delete by context and name:", "polyapi webhook delete hook --context billing"},
		),
	}
	del.Flags().String("context", "", "Context of the webhook (required when looking up by name)")

	testCmd := &cobra.Command{
		Use:   "test <id-or-name>",
		Short: "POST an event payload to a webhook (no Canopy)",
		Args:  cobra.ExactArgs(1),
		RunE:  runWebhookTest,
		Example: examples(
			ex{"Send a JSON event:", `polyapi webhook test hook --context billing --data '{"n":5}'`},
			ex{"Read the event from a file:", "polyapi webhook test hook --context billing --data-file ./event.json"},
		),
	}
	testCmd.Flags().String("context", "", "Context of the webhook (required when looking up by name)")
	testCmd.Flags().String("data", "", "JSON event payload")
	testCmd.Flags().String("data-file", "", "Read the event payload from a file")

	urlCmd := &cobra.Command{
		Use:   "url <id-or-name>",
		Short: "Print the public webhook URL",
		Args:  cobra.ExactArgs(1),
		RunE:  runWebhookURL,
		Example: examples(
			ex{"Print the URL:", "polyapi webhook url hook --context billing"},
		),
	}
	urlCmd.Flags().String("context", "", "Context of the webhook (required when looking up by name)")

	initCmd := newResourceInitCommand(resourceInitKind{
		Use: "webhook", Noun: "webhook", Type: glide.TypeWebhook, NeedsContext: true,
	})

	wh.AddCommand(list, get, initCmd, create, update, del, testCmd, urlCmd)
	root.AddCommand(wh)
}

func addWebhookWriteFlags(cmd *cobra.Command, creating bool) {
	if creating {
		cmd.Flags().String("name", "", "Webhook name")
		cmd.Flags().String("context", "", "Webhook context")
	} else {
		cmd.Flags().String("name", "", "New webhook name")
	}
	cmd.Flags().String("description", "", "Description")
	visDef := ""
	visHelp := "Visibility: PUBLIC, TENANT, or ENVIRONMENT"
	if creating {
		visDef = "ENVIRONMENT"
		visHelp += " (default ENVIRONMENT)"
	}
	cmd.Flags().String("visibility", visDef, visHelp)
	methodDef := ""
	methodHelp := "HTTP method, or comma-separated methods"
	if creating {
		methodDef = "POST"
		methodHelp += " (default POST)"
	}
	cmd.Flags().String("method", methodDef, methodHelp)
	cmd.Flags().String("slug", "", "Optional URL slug instead of the webhook id")
	cmd.Flags().String("subpath", "", "Path/query template appended to the webhook URL")
	cmd.Flags().Bool("require-api-key", false, "Require a Poly API key on incoming requests")
	cmd.Flags().String("event-payload", "", "Sample event payload (JSON, or a raw string)")
	cmd.Flags().String("event-payload-file", "", "Read the event payload from a file")
	cmd.Flags().String("event-payload-schema", "", "JSON Schema for the event payload")
	cmd.Flags().String("response-payload", "", "Static response body (JSON, or a raw string)")
	cmd.Flags().String("response-payload-file", "", "Read the static response body from a file")
	cmd.Flags().String("response-headers", "", "JSON object of response headers")
	cmd.Flags().Uint("response-status", 0, "HTTP status for the static response (200-599)")
	cmd.Flags().StringArray("security-function", nil, "Security function (`id`, `context.name`, or JSON `{id,message}`). Repeatable")
	cmd.Flags().String("security-functions", "", "JSON array of `{id, message}` security functions")
	cmd.Flags().String("xml-parser", "", "JSON object of XML parser options")
}

func runWebhookList(cmd *cobra.Command, _ []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	context, _ := cmd.Flags().GetString("context")
	items, err := client.ListAll(webhookCollection)
	if err != nil {
		return fail(err)
	}
	var rows []webhookRef
	for _, item := range items {
		ref := webhookFromItem(item)
		if context != "" && !strings.HasPrefix(ref.Context, context) {
			continue
		}
		rows = append(rows, ref)
	}
	sort.Slice(rows, func(i, j int) bool {
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
			fmt.Fprintln(out, "no webhooks found")
		}
		return nil
	}
	printWebhookList(out, rows)
	return nil
}

func printWebhookList(w io.Writer, rows []webhookRef) {
	nameW, visW := len("NAME"), len("VISIBILITY")
	for _, row := range rows {
		name := glide.DisplayName(row.Context, row.Name)
		if n := len(name); n > nameW {
			nameW = n
		}
		if n := len(visibilityLabel(row.Visibility)); n > visW {
			visW = n
		}
	}
	fmt.Fprintln(w, infoText(fmt.Sprintf("%-*s  %-*s  %s", nameW, "NAME", visW, "VISIBILITY", "ID")))
	for _, row := range rows {
		fmt.Fprintf(w, "%-*s  %-*s  %s\n", nameW, glide.DisplayName(row.Context, row.Name), visW, visibilityLabel(row.Visibility), row.ID)
	}
}

func runWebhookGet(cmd *cobra.Command, args []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveWebhook(cmd, client, args[0])
	if err != nil {
		return err
	}
	return writeJSON(cmd.OutOrStdout(), ref.Spec)
}

func runWebhookCreate(cmd *cobra.Command, _ []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	payload, err := webhookWritePayload(cmd, client, true)
	if err != nil {
		return err
	}
	created, err := client.Create(webhookCollection, payload)
	if err != nil {
		return fail(err)
	}
	id := mapString(created, "id")
	name := mapString(payload, "name")
	ctx := mapString(payload, "context")
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("created webhook %s (%s)", glide.DisplayName(ctx, name), id))
	}
	return nil
}

func runWebhookUpdate(cmd *cobra.Command, args []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveWebhook(cmd, client, args[0])
	if err != nil {
		return err
	}
	payload, err := webhookWritePayload(cmd, client, false)
	if err != nil {
		return err
	}
	if ctx, _ := cmd.Flags().GetString("new-context"); cmd.Flags().Changed("new-context") {
		payload["context"] = ctx
	}
	if len(payload) == 0 {
		return failUsage("pass at least one field to update")
	}
	updated, err := client.Update(webhookCollection, ref.ID, payload, nil)
	if err != nil {
		return fail(err)
	}
	id := firstNonEmpty(mapString(updated, "id"), ref.ID)
	name := firstNonEmpty(mapString(updated, "name"), mapString(payload, "name"), ref.Name)
	ctx := firstNonEmpty(mapString(updated, "context"), mapString(payload, "context"), ref.Context)
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("updated webhook %s (%s)", glide.DisplayName(ctx, name), id))
	}
	return nil
}

func runWebhookDelete(cmd *cobra.Command, args []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveWebhook(cmd, client, args[0])
	if err != nil {
		return err
	}
	if err := client.Delete(webhookCollection, ref.ID, nil); err != nil {
		return fail(err)
	}
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("deleted webhook %s (%s)", glide.DisplayName(ref.Context, ref.Name), ref.ID))
	}
	return nil
}

func runWebhookTest(cmd *cobra.Command, args []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveWebhook(cmd, client, args[0])
	if err != nil {
		return err
	}
	body, err := readWebhookEvent(cmd)
	if err != nil {
		return err
	}
	result, err := client.PostAction(webhookCollection, ref.ID, "", body)
	if err != nil {
		return fail(err)
	}
	return writeJSON(cmd.OutOrStdout(), result)
}

func runWebhookURL(cmd *cobra.Command, args []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveWebhook(cmd, client, args[0])
	if err != nil {
		return err
	}
	url := firstNonEmpty(ref.URL, mapString(ref.Spec, "url"), mapString(ref.Spec, "uri"))
	if url == "" {
		base := strings.TrimRight(client.BaseURL(), "/")
		url = base + "/webhooks/" + ref.ID
	}
	fmt.Fprintln(cmd.OutOrStdout(), url)
	return nil
}

func webhookWritePayload(cmd *cobra.Command, client *api.HTTPClient, creating bool) (map[string]any, error) {
	payload := map[string]any{}
	if creating || cmd.Flags().Changed("name") {
		name, _ := cmd.Flags().GetString("name")
		if creating && name == "" {
			return nil, failUsage("name is required")
		}
		if name != "" {
			payload["name"] = name
		}
	}
	if creating {
		ctx, _ := cmd.Flags().GetString("context")
		payload["context"] = ctx
	}
	if cmd.Flags().Changed("description") {
		desc, _ := cmd.Flags().GetString("description")
		payload["description"] = desc
	}
	if creating || cmd.Flags().Changed("visibility") {
		vis, _ := cmd.Flags().GetString("visibility")
		if vis == "" && creating {
			vis = "ENVIRONMENT"
		}
		if vis != "" {
			parsed, err := parseVisibility(vis)
			if err != nil {
				return nil, err
			}
			payload["visibility"] = parsed
		}
	}
	if creating || cmd.Flags().Changed("method") {
		method, _ := cmd.Flags().GetString("method")
		if method == "" && creating {
			method = "POST"
		}
		if method != "" {
			parsed, err := parseWebhookMethod(method)
			if err != nil {
				return nil, err
			}
			payload["method"] = parsed
		}
	}
	if cmd.Flags().Changed("slug") {
		slug, _ := cmd.Flags().GetString("slug")
		if slug == "" {
			payload["slug"] = nil
		} else {
			payload["slug"] = slug
		}
	}
	if cmd.Flags().Changed("subpath") {
		sub, _ := cmd.Flags().GetString("subpath")
		payload["subpath"] = sub
	}
	if creating || cmd.Flags().Changed("require-api-key") {
		req, _ := cmd.Flags().GetBool("require-api-key")
		payload["requirePolyApiKey"] = req
	}
	if cmd.Flags().Changed("event-payload") && cmd.Flags().Changed("event-payload-file") {
		return nil, failUsage("pass only one of --event-payload or --event-payload-file")
	}
	if cmd.Flags().Changed("event-payload") || cmd.Flags().Changed("event-payload-file") {
		v, err := readJSONOrStringFlag(cmd, "event-payload", "event-payload-file")
		if err != nil {
			return nil, err
		}
		payload["eventPayload"] = v
	}
	if cmd.Flags().Changed("event-payload-schema") {
		raw, _ := cmd.Flags().GetString("event-payload-schema")
		obj, err := parseJSONObject([]byte(raw), "event-payload-schema")
		if err != nil {
			return nil, err
		}
		payload["eventPayloadTypeSchema"] = obj
	}
	if cmd.Flags().Changed("response-payload") && cmd.Flags().Changed("response-payload-file") {
		return nil, failUsage("pass only one of --response-payload or --response-payload-file")
	}
	if cmd.Flags().Changed("response-payload") || cmd.Flags().Changed("response-payload-file") {
		v, err := readJSONOrStringFlag(cmd, "response-payload", "response-payload-file")
		if err != nil {
			return nil, err
		}
		payload["responsePayload"] = v
	}
	if cmd.Flags().Changed("response-headers") {
		raw, _ := cmd.Flags().GetString("response-headers")
		obj, err := parseJSONObject([]byte(raw), "response-headers")
		if err != nil {
			return nil, err
		}
		payload["responseHeaders"] = obj
	}
	if cmd.Flags().Changed("response-status") {
		status, _ := cmd.Flags().GetUint("response-status")
		if status < 200 || status > 599 {
			return nil, failUsage("option `response-status` must be between 200 and 599")
		}
		payload["responseStatus"] = status
	}
	fns, err := readSecurityFunctions(cmd, client)
	if err != nil {
		return nil, err
	}
	if fns != nil {
		payload["securityFunctions"] = fns
	}
	if cmd.Flags().Changed("xml-parser") {
		raw, _ := cmd.Flags().GetString("xml-parser")
		obj, err := parseJSONObject([]byte(raw), "xml-parser")
		if err != nil {
			return nil, err
		}
		payload["xmlParserOptions"] = obj
	}
	return payload, nil
}

func readWebhookEvent(cmd *cobra.Command) (any, error) {
	if cmd.Flags().Changed("data") && cmd.Flags().Changed("data-file") {
		return nil, failUsage("pass only one of --data or --data-file")
	}
	if !cmd.Flags().Changed("data") && !cmd.Flags().Changed("data-file") {
		return map[string]any{}, nil
	}
	return readJSONOrStringFlag(cmd, "data", "data-file")
}

func readJSONOrStringFlag(cmd *cobra.Command, flagName, fileFlag string) (any, error) {
	var raw []byte
	if cmd.Flags().Changed(fileFlag) {
		path, _ := cmd.Flags().GetString(fileFlag)
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, failUsage(fmt.Sprintf("read %s: %v", path, err))
		}
		raw = b
	} else {
		s, _ := cmd.Flags().GetString(flagName)
		raw = []byte(s)
	}
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return "", nil
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err == nil {
		return v, nil
	}
	return string(raw), nil
}

func readSecurityFunctions(cmd *cobra.Command, client *api.HTTPClient) ([]any, error) {
	set := cmd.Flags().Changed("security-function")
	jsonSet := cmd.Flags().Changed("security-functions")
	if set && jsonSet {
		return nil, failUsage("pass only one of --security-function or --security-functions")
	}
	if !set && !jsonSet {
		return nil, nil
	}
	var entries []any
	if jsonSet {
		raw, _ := cmd.Flags().GetString("security-functions")
		var v any
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, failUsage("invalid --security-functions JSON: " + err.Error())
		}
		switch t := v.(type) {
		case []any:
			entries = t
		case map[string]any:
			entries = []any{t}
		default:
			return nil, failUsage("option `security-functions` must be a JSON array")
		}
	} else {
		raw, _ := cmd.Flags().GetStringArray("security-function")
		for _, s := range raw {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			var obj map[string]any
			if err := json.Unmarshal([]byte(s), &obj); err == nil {
				entries = append(entries, obj)
				continue
			}
			entries = append(entries, map[string]any{"id": s})
		}
	}
	out := make([]any, 0, len(entries))
	for i, raw := range entries {
		m, ok := raw.(map[string]any)
		if !ok {
			return nil, failUsage(fmt.Sprintf("securityFunctions[%d] must be an object", i))
		}
		id := strings.TrimSpace(firstNonEmpty(mapString(m, "id"), mapString(m, "functionId")))
		if id == "" {
			return nil, failUsage(fmt.Sprintf("securityFunctions[%d] needs an id", i))
		}
		resolved, err := resolveServerFunctionID(client, id)
		if err != nil {
			return nil, err
		}
		entry := map[string]any{"id": resolved}
		if msg := mapString(m, "message"); msg != "" {
			entry["message"] = msg
		}
		out = append(out, entry)
	}
	return out, nil
}

func parseWebhookMethod(s string) (string, error) {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		m := strings.ToUpper(strings.TrimSpace(p))
		switch m {
		case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
			out = append(out, m)
		case "":
			continue
		default:
			return "", failUsage("option `method` must be GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS, or a comma-separated list")
		}
	}
	if len(out) == 0 {
		return "", failUsage("option `method` must be GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS, or a comma-separated list")
	}
	return strings.Join(out, ","), nil
}

func apiClient(cmd *cobra.Command) (*api.HTTPClient, error) {
	g := globalsFrom(cmd)
	cfg, err := loadFromGlobal(g)
	if err != nil {
		return nil, fail(err)
	}
	if _, _, err := cfg.RequireCredentials(); err != nil {
		return nil, fail(err)
	}
	client, err := api.FromConfig(cfg)
	if err != nil {
		return nil, fail(err)
	}
	return client, nil
}

func resolveWebhook(cmd *cobra.Command, client *api.HTTPClient, idOrName string) (webhookRef, error) {
	context, _ := cmd.Flags().GetString("context")
	if context != "" {
		return lookupWebhookByName(client, context, idOrName)
	}
	if !looksLikeUUID(idOrName) {
		if ctx, name, ok := splitContextName(idOrName); ok {
			return lookupWebhookByName(client, ctx, name)
		}
	}
	return lookupWebhookByID(client, idOrName)
}

func lookupWebhookByID(client *api.HTTPClient, id string) (webhookRef, error) {
	spec, err := client.Get(webhookCollection, id)
	if err != nil {
		return webhookRef{}, fail(err)
	}
	ref := webhookFromItem(spec)
	if ref.ID == "" {
		ref.ID = id
	}
	ref.Spec = spec
	return ref, nil
}

func lookupWebhookByName(client *api.HTTPClient, context, name string) (webhookRef, error) {
	items, err := client.ListAll(webhookCollection)
	if err != nil {
		return webhookRef{}, fail(err)
	}
	var sameCase []webhookRef
	var anyCase []webhookRef
	for _, item := range items {
		ref := webhookFromItem(item)
		if !strings.EqualFold(ref.Name, name) || !strings.EqualFold(ref.Context, context) {
			continue
		}
		anyCase = append(anyCase, ref)
		if ref.Name == name && ref.Context == context {
			sameCase = append(sameCase, ref)
		}
	}
	matches := sameCase
	if len(matches) == 0 {
		matches = anyCase
	}
	switch len(matches) {
	case 0:
		return webhookRef{}, failUsage(fmt.Sprintf("no webhook named %q in context %q", name, context))
	case 1:
		ref := matches[0]
		if ref.ID == "" {
			return ref, nil
		}
		spec, err := client.Get(webhookCollection, ref.ID)
		if err != nil {
			return webhookRef{}, fail(err)
		}
		full := webhookFromItem(spec)
		full.ID = ref.ID
		full.Spec = spec
		return full, nil
	default:
		return webhookRef{}, failUsage(fmt.Sprintf("unclear webhook reference %s; matches %d webhooks", glide.DisplayName(context, name), len(matches)))
	}
}

func webhookFromItem(item any) webhookRef {
	m, _ := item.(map[string]any)
	return webhookRef{
		ID:         firstMapString(m, "id"),
		Name:       mapString(m, "name"),
		Context:    mapString(m, "context"),
		Visibility: mapString(m, "visibility"),
		URL:        firstMapString(m, "url", "uri"),
		Spec:       item,
	}
}

func looksLikeUUID(s string) bool {
	return uuidLike.MatchString(strings.TrimSpace(s))
}

func splitContextName(s string) (context, name string, ok bool) {
	s = strings.TrimSpace(s)
	i := strings.LastIndex(s, ".")
	if i <= 0 || i == len(s)-1 {
		return "", s, false
	}
	return s[:i], s[i+1:], true
}

func resolveServerFunctionID(client *api.HTTPClient, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", failUsage("server function reference is empty")
	}
	if looksLikeUUID(ref) {
		return ref, nil
	}
	ctx, name, ok := splitContextName(ref)
	if !ok {
		return "", failUsage(fmt.Sprintf("server function %q must be an id or context.name", ref))
	}
	items, err := client.ListAll("functions/server")
	if err != nil {
		return "", fail(err)
	}
	var same, anyCase []string
	for _, item := range items {
		m, _ := item.(map[string]any)
		n, c := mapString(m, "name"), mapString(m, "context")
		id := firstMapString(m, "id")
		if id == "" || !strings.EqualFold(n, name) || !strings.EqualFold(c, ctx) {
			continue
		}
		anyCase = append(anyCase, id)
		if n == name && c == ctx {
			same = append(same, id)
		}
	}
	matches := same
	if len(matches) == 0 {
		matches = anyCase
	}
	switch len(matches) {
	case 0:
		return "", failUsage(fmt.Sprintf("no server function named %q in context %q", name, ctx))
	case 1:
		return matches[0], nil
	default:
		return "", failUsage(fmt.Sprintf("unclear server function reference %s; matches %d functions", glide.DisplayName(ctx, name), len(matches)))
	}
}

func resolveWebhookID(client *api.HTTPClient, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", failUsage("webhook reference is empty")
	}
	if looksLikeUUID(ref) {
		return ref, nil
	}
	ctx, name, ok := splitContextName(ref)
	if !ok {
		return "", failUsage(fmt.Sprintf("webhook %q must be an id or context.name", ref))
	}
	got, err := lookupWebhookByName(client, ctx, name)
	if err != nil {
		return "", err
	}
	return got.ID, nil
}
