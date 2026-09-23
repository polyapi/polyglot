package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/polyapi/polyglot/src/api"
	"github.com/polyapi/polyglot/src/glide"
	"github.com/spf13/cobra"
)

const subscriptionCollection = "subscriptions/graphql"

type subscriptionRef struct {
	ID      string
	Name    string
	Context string
	Enabled bool
	Spec    any
}

func addSubscriptionCommands(root *cobra.Command) {
	sub := &cobra.Command{
		Use:     "subscription",
		Aliases: []string{"subscriptions", "graphql-subscription", "gql-subscription"},
		Short:   "Manage GraphQL subscriptions (CUSTOM or OHIP → server function)",
		Example: examples(
			ex{"List subscriptions:", "polyapi subscription list"},
			ex{"Scaffold a local CUSTOM subscription:", "polyapi subscription init --name ordersStream --type CUSTOM --context shopify"},
			ex{"Create a CUSTOM subscription:", `polyapi subscription create --name ordersStream --context shopify --type CUSTOM --websocket-url wss://example.com/graphql --query 'subscription { orderUpdated { id } }' --function shopify.handleOrder`},
			ex{"Recover a stuck OHIP stream:", "polyapi subscription recover opera.events --ohip-offset-selection PERSISTED"},
		),
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "List GraphQL subscriptions",
		Args:  cobra.NoArgs,
		RunE:  runSubscriptionList,
		Example: examples(
			ex{"List all subscriptions:", "polyapi subscription list"},
			ex{"Restrict to a context:", "polyapi subscription list --context shopify"},
		),
	}
	list.Flags().String("context", "", "Restrict results to this context prefix")

	get := &cobra.Command{
		Use:   "get <id-or-name>",
		Short: "Get a subscription by ID, or by name with --context",
		Args:  cobra.ExactArgs(1),
		RunE:  runSubscriptionGet,
		Example: examples(
			ex{"Get by ID:", "polyapi subscription get abc123"},
			ex{"Get by context and name:", "polyapi subscription get ordersStream --context shopify"},
			ex{"Get by context.name:", "polyapi subscription get shopify.ordersStream"},
		),
	}
	get.Flags().String("context", "", "Context of the subscription (required when looking up by name)")

	create := &cobra.Command{
		Use:   "create",
		Short: "Create a GraphQL subscription",
		Long:  "Create a CUSTOM or OHIP GraphQL subscription that forwards provider events into a server function. The platform starts the stream immediately when enabled. Changing query, URL, type, params, or OHIP offset settings later restarts the live connection.",
		Args:  cobra.NoArgs,
		RunE:  runSubscriptionCreate,
		Example: examples(
			ex{"CUSTOM stream:", `polyapi subscription create --name ordersStream --context shopify --type CUSTOM --websocket-url wss://example.com/graphql --query 'subscription { orderUpdated { id } }' --function shopify.handleOrder`},
			ex{"OHIP from provider highest:", "polyapi subscription create --name operaEvents --context opera --type OHIP --websocket-url wss://ohip.example/graphql --query-file ./ohip.graphql --function opera.handleEvent --params-object-file ./ohip-params.json --ohip-maintain-offset"},
		),
	}
	addSubscriptionWriteFlags(create, true)
	_ = create.MarkFlagRequired("name")
	_ = create.MarkFlagRequired("context")
	_ = create.MarkFlagRequired("type")
	_ = create.MarkFlagRequired("websocket-url")
	_ = create.MarkFlagRequired("function")
	create.MarkFlagsOneRequired("query", "query-file")
	create.MarkFlagsMutuallyExclusive("params-variable", "params-sfx", "params-object", "params-object-file")
	create.MarkFlagsMutuallyExclusive("query", "query-file")
	create.MarkFlagsMutuallyExclusive("function-params", "function-params-file")
	create.MarkFlagsMutuallyExclusive("params-object", "params-object-file")

	update := &cobra.Command{
		Use:   "update <id-or-name>",
		Short: "Update a subscription by ID, or by name with --context",
		Long:  "PATCH only the flags you pass. Including query, websocket-url, type, params, or OHIP offset settings restarts the live stream. Visibility is accepted by the API but currently ignored on update.",
		Args:  cobra.ExactArgs(1),
		RunE:  runSubscriptionUpdate,
		Example: examples(
			ex{"Rename:", "polyapi subscription update ordersStream --context shopify --name ordersStreamV2"},
			ex{"Disable without deleting:", "polyapi subscription update shopify.ordersStream --enabled=false"},
		),
	}
	update.Flags().String("context", "", "Context of the subscription (required when looking up by name)")
	addSubscriptionWriteFlags(update, false)
	update.Flags().String("new-context", "", "Move the subscription to this context")
	update.MarkFlagsMutuallyExclusive("params-variable", "params-sfx", "params-object", "params-object-file")
	update.MarkFlagsMutuallyExclusive("query", "query-file")
	update.MarkFlagsMutuallyExclusive("function-params", "function-params-file")
	update.MarkFlagsMutuallyExclusive("params-object", "params-object-file")

	del := &cobra.Command{
		Use:   "delete <id-or-name>",
		Short: "Delete a subscription by ID, or by name with --context",
		Args:  cobra.ExactArgs(1),
		RunE:  runSubscriptionDelete,
		Example: examples(
			ex{"Delete by context.name:", "polyapi subscription delete shopify.ordersStream"},
		),
	}
	del.Flags().String("context", "", "Context of the subscription (required when looking up by name)")
	del.Flags().String("otp", "", "One-time password when the instance requires MFA for deletes")

	recoverCmd := &cobra.Command{
		Use:   "recover <id-or-name>",
		Short: "Force-stop a subscription and optionally restart it",
		Long:  "Operator recovery for a live GraphQL subscription. This is not part of deploy. PROVIDER_HIGHEST is allowed here because it is an explicit request.",
		Args:  cobra.ExactArgs(1),
		RunE:  runSubscriptionRecover,
		Example: examples(
			ex{"Restart from the persisted OHIP checkpoint:", "polyapi subscription recover opera.events --ohip-offset-selection PERSISTED"},
			ex{"Stop without restarting:", "polyapi subscription recover opera.events --restart=false"},
		),
	}
	recoverCmd.Flags().String("context", "", "Context of the subscription (required when looking up by name)")
	recoverCmd.Flags().String("reason", "", "Why recovery was requested")
	recoverCmd.Flags().Bool("restart", true, "Restart after the owner closes (default true)")
	recoverCmd.Flags().String("ohip-offset-selection", "", "OHIP start after recover: PROVIDER_HIGHEST or PERSISTED")
	recoverCmd.Flags().Int("wait-for-close-ms", 0, "How long to wait for the owner to close (default 30000, max 60000)")
	recoverCmd.Flags().String("otp", "", "One-time password when the instance requires MFA")

	initCmd := newResourceInitCommand(resourceInitKind{
		Use: "subscription", Noun: "GraphQL subscription", Type: glide.TypeSubscription, NeedsContext: true, TypeMode: initTypeSubscription,
	})

	sub.AddCommand(list, get, initCmd, create, update, del, recoverCmd)
	root.AddCommand(sub)
}

func addSubscriptionWriteFlags(cmd *cobra.Command, creating bool) {
	if creating {
		cmd.Flags().String("name", "", "Subscription name")
		cmd.Flags().String("context", "", "Subscription context")
		cmd.Flags().Var(&subscriptionTypeValue{}, "type", "Subscription type (CUSTOM|OHIP)")
		cmd.Flags().Bool("enabled", true, "Whether the subscription is enabled (default true)")
		cmd.Flags().String("visibility", "ENVIRONMENT", "Visibility: TENANT or ENVIRONMENT (default ENVIRONMENT)")
	} else {
		cmd.Flags().String("name", "", "New subscription name")
		cmd.Flags().Var(&subscriptionTypeValue{}, "type", "Subscription type (CUSTOM|OHIP)")
		cmd.Flags().Bool("enabled", true, "Whether the subscription is enabled")
		cmd.Flags().String("visibility", "", "Visibility: TENANT or ENVIRONMENT (the platform currently ignores visibility on update)")
	}
	cmd.Flags().String("description", "", "Description")
	cmd.Flags().String("websocket-url", "", "Upstream GraphQL websocket URL (`ws://` or `wss://`)")
	cmd.Flags().String("query", "", "GraphQL subscription query")
	cmd.Flags().String("query-file", "", "Read the GraphQL subscription query from a file")
	cmd.Flags().String("function", "", "Destination server function (`id` or `context.name`)")
	cmd.Flags().String("function-params", "", "JSON object passed to the destination function as params")
	cmd.Flags().String("function-params-file", "", "Read function params JSON from a file")
	cmd.Flags().String("params-variable", "", "Variable holding connection params (`id` or `context.name`)")
	cmd.Flags().String("params-sfx", "", "Server function that produces connection params (`id` or `context.name`)")
	cmd.Flags().String("params-object", "", "Inline connection params JSON object")
	cmd.Flags().String("params-object-file", "", "Read connection params JSON from a file")
	cmd.Flags().String("ohip-offset-selection", "", "OHIP start: PROVIDER_HIGHEST or SPECIFIC on create; PROVIDER_HIGHEST or PERSISTED on update")
	cmd.Flags().String("ohip-offset", "", "OHIP offset when create --ohip-offset-selection is SPECIFIC")
	cmd.Flags().Bool("ohip-maintain-offset", false, "Persist OHIP checkpoints after successful handling")
	cmd.Flags().Int("event-inactivity-threshold-ms", 0, "Inactivity monitor threshold in ms (60000-86400000); 0 omits")
	cmd.Flags().String("queue-id", "", "Optional queue UUID; events go to the queue instead of the function")
	cmd.Flags().String("otp", "", "One-time password when the instance requires MFA")
}

func runSubscriptionList(cmd *cobra.Command, _ []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	context, _ := cmd.Flags().GetString("context")
	items, err := client.ListAll(subscriptionCollection)
	if err != nil {
		return fail(err)
	}
	var rows []subscriptionRef
	for _, item := range items {
		ref := subscriptionFromItem(item)
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
			fmt.Fprintln(out, "no subscriptions found")
		}
		return nil
	}
	printSubscriptionList(out, rows)
	return nil
}

func printSubscriptionList(w io.Writer, rows []subscriptionRef) {
	nameW, enW := len("NAME"), len("ENABLED")
	for _, row := range rows {
		name := glide.DisplayName(row.Context, row.Name)
		if n := len(name); n > nameW {
			nameW = n
		}
		if n := len(strconv.FormatBool(row.Enabled)); n > enW {
			enW = n
		}
	}
	fmt.Fprintln(w, infoText(fmt.Sprintf("%-*s  %-*s  %s", nameW, "NAME", enW, "ENABLED", "ID")))
	for _, row := range rows {
		fmt.Fprintf(w, "%-*s  %-*s  %s\n", nameW, glide.DisplayName(row.Context, row.Name), enW, strconv.FormatBool(row.Enabled), row.ID)
	}
}

func runSubscriptionGet(cmd *cobra.Command, args []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveSubscription(cmd, client, args[0])
	if err != nil {
		return err
	}
	return writeJSON(cmd.OutOrStdout(), redactSubscriptionSpec(ref.Spec))
}

func runSubscriptionCreate(cmd *cobra.Command, _ []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	payload, err := subscriptionWritePayload(cmd, client, true)
	if err != nil {
		return err
	}
	created, err := client.CreateHeaders(subscriptionCollection, payload, otpHeaders(cmd))
	if err != nil {
		return fail(err)
	}
	id := mapString(created, "id")
	name := mapString(payload, "name")
	ctx := mapString(payload, "context")
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("created subscription %s (%s)", glide.DisplayName(ctx, name), id))
	}
	return nil
}

func runSubscriptionUpdate(cmd *cobra.Command, args []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveSubscription(cmd, client, args[0])
	if err != nil {
		return err
	}
	payload, err := subscriptionWritePayload(cmd, client, false)
	if err != nil {
		return err
	}
	if cmd.Flags().Changed("new-context") {
		ctx, _ := cmd.Flags().GetString("new-context")
		payload["context"] = ctx
	}
	if len(payload) == 0 {
		return failUsage("pass at least one field to update")
	}
	updated, err := client.Update(subscriptionCollection, ref.ID, payload, otpHeaders(cmd))
	if err != nil {
		return fail(err)
	}
	id := firstNonEmpty(mapString(updated, "id"), ref.ID)
	name := firstNonEmpty(mapString(updated, "name"), mapString(payload, "name"), ref.Name)
	ctx := firstNonEmpty(mapString(updated, "context"), mapString(payload, "context"), ref.Context)
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("updated subscription %s (%s)", glide.DisplayName(ctx, name), id))
	}
	return nil
}

func runSubscriptionDelete(cmd *cobra.Command, args []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveSubscription(cmd, client, args[0])
	if err != nil {
		return err
	}
	if err := client.Delete(subscriptionCollection, ref.ID, otpHeaders(cmd)); err != nil {
		return fail(err)
	}
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("deleted subscription %s (%s)", glide.DisplayName(ref.Context, ref.Name), ref.ID))
	}
	return nil
}

func runSubscriptionRecover(cmd *cobra.Command, args []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveSubscription(cmd, client, args[0])
	if err != nil {
		return err
	}
	payload := map[string]any{}
	if cmd.Flags().Changed("reason") {
		reason, _ := cmd.Flags().GetString("reason")
		payload["reason"] = reason
	}
	if cmd.Flags().Changed("restart") {
		v, _ := cmd.Flags().GetBool("restart")
		payload["restart"] = v
	}
	if cmd.Flags().Changed("ohip-offset-selection") {
		sel, err := parseOHIPUpdateOffsetSelection(flagString(cmd, "ohip-offset-selection"))
		if err != nil {
			return err
		}
		payload["ohipOffsetSelection"] = sel
	}
	if cmd.Flags().Changed("wait-for-close-ms") {
		ms, _ := cmd.Flags().GetInt("wait-for-close-ms")
		if ms < 0 || ms > 60000 {
			return failUsage("--wait-for-close-ms must be between 0 and 60000")
		}
		payload["waitForCloseMs"] = ms
	}
	got, err := client.PostActionHeaders(subscriptionCollection, ref.ID, "recover", payload, otpHeaders(cmd))
	if err != nil {
		return fail(err)
	}
	if globalsFrom(cmd).Quiet {
		return writeJSON(cmd.OutOrStdout(), got)
	}
	m, _ := got.(map[string]any)
	msg := mapString(m, "message")
	if msg == "" {
		msg = "recover requested"
	}
	printOk(cmd.OutOrStdout(), msg)
	return writeJSON(cmd.OutOrStdout(), got)
}

func subscriptionWritePayload(cmd *cobra.Command, client *api.HTTPClient, creating bool) (map[string]any, error) {
	payload := map[string]any{}
	kind := ""
	if creating || cmd.Flags().Changed("type") {
		raw := ""
		if f := cmd.Flags().Lookup("type"); f != nil {
			raw = f.Value.String()
		}
		parsed, ok := glide.NormalizeSubscriptionType(raw)
		if !ok {
			return nil, failUsage(fmt.Sprintf("invalid subscription type %q (CUSTOM|OHIP)", raw))
		}
		kind = parsed
		payload["type"] = parsed
	}

	if creating || cmd.Flags().Changed("name") {
		name := flagString(cmd, "name")
		if creating && name == "" {
			return nil, failUsage("name is required")
		}
		if name != "" {
			if err := glide.ValidSubscriptionName(name); err != nil {
				return nil, failUsage(err.Error())
			}
			payload["name"] = name
		}
	}
	if creating {
		payload["context"] = flagString(cmd, "context")
		payload["transportProtocol"] = "WS"
		payload["enabled"] = boolFlagOr(cmd, "enabled", true)
	} else if cmd.Flags().Changed("enabled") {
		v, _ := cmd.Flags().GetBool("enabled")
		payload["enabled"] = v
	}

	if creating || cmd.Flags().Changed("visibility") {
		vis := flagString(cmd, "visibility")
		if vis == "" && creating {
			vis = "ENVIRONMENT"
		}
		if vis != "" {
			parsed, err := parseNonPublicVisibility(vis)
			if err != nil {
				return nil, err
			}
			payload["visibility"] = parsed
		}
	}
	if creating || cmd.Flags().Changed("description") {
		if creating || cmd.Flags().Changed("description") {
			payload["description"] = flagString(cmd, "description")
		}
	}
	if creating || cmd.Flags().Changed("websocket-url") {
		url := flagString(cmd, "websocket-url")
		if creating && url == "" {
			return nil, failUsage("websocket-url is required")
		}
		if url != "" {
			if !strings.HasPrefix(url, "ws://") && !strings.HasPrefix(url, "wss://") {
				return nil, failUsage("websocket-url must start with ws:// or wss://")
			}
			payload["websocketUrl"] = url
		}
	}
	if cmd.Flags().Changed("query") || cmd.Flags().Changed("query-file") || creating {
		q, err := readSubscriptionQuery(cmd, creating)
		if err != nil {
			return nil, err
		}
		if q != "" {
			payload["query"] = q
		}
	}
	if creating || cmd.Flags().Changed("function") {
		ref := flagString(cmd, "function")
		if creating && ref == "" {
			return nil, failUsage("function is required")
		}
		if ref != "" {
			id, err := resolveServerFunctionID(client, ref)
			if err != nil {
				return nil, err
			}
			payload["functionId"] = id
		}
	}
	if err := addJSONObjectFlag(cmd, payload, "functionParams", "function-params", "function-params-file"); err != nil {
		return nil, err
	}
	if err := addSubscriptionParams(cmd, client, payload, creating, kind); err != nil {
		return nil, err
	}
	if err := addSubscriptionOHIPFlags(cmd, payload, creating, kind); err != nil {
		return nil, err
	}
	if cmd.Flags().Changed("event-inactivity-threshold-ms") {
		ms, _ := cmd.Flags().GetInt("event-inactivity-threshold-ms")
		if ms == 0 {
			payload["eventInactivityThresholdMs"] = nil
		} else if ms < 60_000 || ms > 86_400_000 {
			return nil, failUsage("--event-inactivity-threshold-ms must be between 60000 and 86400000, or 0 to clear")
		} else {
			payload["eventInactivityThresholdMs"] = ms
		}
	}
	if creating || cmd.Flags().Changed("queue-id") {
		qid := flagString(cmd, "queue-id")
		if cmd.Flags().Changed("queue-id") || qid != "" {
			if qid == "" {
				payload["queueId"] = nil
			} else {
				payload["queueId"] = qid
			}
		}
	}
	return payload, nil
}

func addSubscriptionParams(cmd *cobra.Command, client *api.HTTPClient, payload map[string]any, creating bool, kind string) error {
	hasVar := cmd.Flags().Changed("params-variable")
	hasSFX := cmd.Flags().Changed("params-sfx")
	hasObj := cmd.Flags().Changed("params-object") || cmd.Flags().Changed("params-object-file")
	n := 0
	for _, b := range []bool{hasVar, hasSFX, hasObj} {
		if b {
			n++
		}
	}
	if n > 1 {
		return failUsage("pass only one of --params-variable, --params-sfx, or --params-object")
	}
	if hasVar {
		ref := flagString(cmd, "params-variable")
		if ref == "" {
			payload["paramsVariableId"] = nil
			return nil
		}
		id, err := resolveVariableID(client, ref)
		if err != nil {
			return err
		}
		payload["paramsVariableId"] = id
		return nil
	}
	if hasSFX {
		ref := flagString(cmd, "params-sfx")
		if ref == "" {
			payload["paramsSfxId"] = nil
			return nil
		}
		id, err := resolveServerFunctionID(client, ref)
		if err != nil {
			return err
		}
		payload["paramsSfxId"] = id
		return nil
	}
	if hasObj {
		obj, err := readJSONObjectFlag(cmd, "params-object", "params-object-file")
		if err != nil {
			return err
		}
		payload["paramsObject"] = obj
		if kind == "" && creating {
			kind = mapString(payload, "type")
		}
		if kind == glide.SubscriptionTypeOHIP {
			if err := validateOHIPParamsObject(obj); err != nil {
				return err
			}
		}
	}
	if creating && kind == glide.SubscriptionTypeOHIP && !hasObj {
		return failUsage("OHIP subscriptions require --params-object or --params-object-file")
	}
	return nil
}

func addSubscriptionOHIPFlags(cmd *cobra.Command, payload map[string]any, creating bool, kind string) error {
	hasSel := cmd.Flags().Changed("ohip-offset-selection")
	hasOff := cmd.Flags().Changed("ohip-offset")
	hasMaint := cmd.Flags().Changed("ohip-maintain-offset")
	if !hasSel && !hasOff && !hasMaint {
		return nil
	}
	if kind == "" {
		kind = mapString(payload, "type")
	}
	if kind == glide.SubscriptionTypeCustom {
		return failUsage("OHIP offset flags are only valid with --type OHIP")
	}
	if creating && kind != glide.SubscriptionTypeOHIP && kind != "" {
		return failUsage("OHIP offset flags are only valid with --type OHIP")
	}
	if hasMaint {
		v, _ := cmd.Flags().GetBool("ohip-maintain-offset")
		payload["ohipMaintainOffset"] = v
		if v {
			q := mapString(payload, "query")
			if q != "" && !strings.Contains(q, "offset") {
				return failUsage("ohipMaintainOffset requires the query to select metadata.offset")
			}
		}
	}
	if hasSel {
		sel := strings.ToUpper(flagString(cmd, "ohip-offset-selection"))
		if creating {
			switch sel {
			case "PROVIDER_HIGHEST", "SPECIFIC":
				payload["ohipOffsetSelection"] = sel
			default:
				return failUsage("--ohip-offset-selection on create must be PROVIDER_HIGHEST or SPECIFIC")
			}
			if sel == "SPECIFIC" {
				off := flagString(cmd, "ohip-offset")
				if off == "" {
					return failUsage("--ohip-offset is required when --ohip-offset-selection is SPECIFIC")
				}
				payload["ohipOffset"] = off
			} else if hasOff {
				return failUsage("--ohip-offset cannot be set when --ohip-offset-selection is PROVIDER_HIGHEST")
			}
		} else {
			parsed, err := parseOHIPUpdateOffsetSelection(sel)
			if err != nil {
				return err
			}
			payload["ohipOffsetSelection"] = parsed
			if hasOff {
				return failUsage("--ohip-offset cannot be set on update; use --ohip-offset-selection PERSISTED or PROVIDER_HIGHEST")
			}
		}
	} else if hasOff {
		if !creating {
			return failUsage("--ohip-offset cannot be set on update")
		}
		return failUsage("--ohip-offset requires --ohip-offset-selection SPECIFIC")
	}
	return nil
}

func readSubscriptionQuery(cmd *cobra.Command, creating bool) (string, error) {
	if cmd.Flags().Changed("query") && cmd.Flags().Changed("query-file") {
		return "", failUsage("pass only one of --query or --query-file")
	}
	if cmd.Flags().Changed("query-file") {
		path := flagString(cmd, "query-file")
		b, err := os.ReadFile(path)
		if err != nil {
			return "", failUsage(fmt.Sprintf("read %s: %v", path, err))
		}
		q := strings.TrimSpace(string(b))
		if q == "" {
			return "", failUsage("query file is empty")
		}
		return q, nil
	}
	q := strings.TrimSpace(flagString(cmd, "query"))
	if creating && q == "" && !cmd.Flags().Changed("query") {
		return "", failUsage("pass --query or --query-file")
	}
	return q, nil
}

func addJSONObjectFlag(cmd *cobra.Command, payload map[string]any, key, flag, fileFlag string) error {
	if !cmd.Flags().Changed(flag) && !cmd.Flags().Changed(fileFlag) {
		return nil
	}
	obj, err := readJSONObjectFlag(cmd, flag, fileFlag)
	if err != nil {
		return err
	}
	payload[key] = obj
	return nil
}

func readJSONObjectFlag(cmd *cobra.Command, flag, fileFlag string) (any, error) {
	if cmd.Flags().Changed(flag) && cmd.Flags().Changed(fileFlag) {
		return nil, failUsage(fmt.Sprintf("pass only one of --%s or --%s", flag, fileFlag))
	}
	var raw []byte
	if cmd.Flags().Changed(fileFlag) {
		path := flagString(cmd, fileFlag)
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, failUsage(fmt.Sprintf("read %s: %v", path, err))
		}
		raw = b
	} else {
		raw = []byte(flagString(cmd, flag))
	}
	if strings.TrimSpace(string(raw)) == "" || strings.TrimSpace(string(raw)) == "null" {
		return nil, nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, failUsage(fmt.Sprintf("invalid JSON for --%s: %v", flag, err))
	}
	if _, ok := v.(map[string]any); !ok && v != nil {
		return nil, failUsage(fmt.Sprintf("--%s must be a JSON object", flag))
	}
	return v, nil
}

func validateOHIPParamsObject(v any) error {
	m, ok := v.(map[string]any)
	if !ok || m == nil {
		return failUsage("OHIP paramsObject must be an object with an ohip key")
	}
	ohip, ok := m["ohip"].(map[string]any)
	if !ok || ohip == nil {
		return failUsage("paramsObject.ohip must be an object with hostName, appKey, enterpriseId, clientId, clientSecret")
	}
	for _, k := range []string{"hostName", "appKey", "enterpriseId", "clientId", "clientSecret"} {
		if _, ok := ohip[k]; !ok {
			return failUsage("paramsObject.ohip must include " + k)
		}
	}
	return nil
}

func parseNonPublicVisibility(s string) (string, error) {
	v := strings.ToUpper(strings.TrimSpace(s))
	switch v {
	case "TENANT", "ENVIRONMENT":
		return v, nil
	case "PUBLIC":
		return "", failUsage("option `visibility` must be TENANT or ENVIRONMENT (GraphQL subscriptions are not PUBLIC)")
	default:
		return "", failUsage("option `visibility` must be TENANT or ENVIRONMENT")
	}
}

func parseOHIPUpdateOffsetSelection(s string) (string, error) {
	v := strings.ToUpper(strings.TrimSpace(s))
	switch v {
	case "PROVIDER_HIGHEST", "PERSISTED":
		return v, nil
	default:
		return "", failUsage("--ohip-offset-selection on update/recover must be PROVIDER_HIGHEST or PERSISTED")
	}
}

func resolveSubscription(cmd *cobra.Command, client *api.HTTPClient, idOrName string) (subscriptionRef, error) {
	context, _ := cmd.Flags().GetString("context")
	if context != "" {
		return lookupSubscriptionByName(client, context, idOrName)
	}
	if !looksLikeUUID(idOrName) {
		if ctx, name, ok := splitContextName(idOrName); ok {
			return lookupSubscriptionByName(client, ctx, name)
		}
	}
	if looksLikeUUID(idOrName) {
		return lookupSubscriptionByID(client, idOrName)
	}
	return lookupSubscriptionUniqueName(client, idOrName)
}

func lookupSubscriptionByID(client *api.HTTPClient, id string) (subscriptionRef, error) {
	spec, err := client.Get(subscriptionCollection, id)
	if err != nil {
		return subscriptionRef{}, fail(err)
	}
	ref := subscriptionFromItem(spec)
	if ref.ID == "" {
		ref.ID = id
	}
	ref.Spec = spec
	return ref, nil
}

func lookupSubscriptionByName(client *api.HTTPClient, context, name string) (subscriptionRef, error) {
	items, err := client.ListAll(subscriptionCollection)
	if err != nil {
		return subscriptionRef{}, fail(err)
	}
	var sameCase []subscriptionRef
	var anyCase []subscriptionRef
	for _, item := range items {
		ref := subscriptionFromItem(item)
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
		return subscriptionRef{}, failUsage(fmt.Sprintf("no subscription named %q in context %q", name, context))
	case 1:
		ref := matches[0]
		if ref.ID == "" {
			return ref, nil
		}
		return lookupSubscriptionByID(client, ref.ID)
	default:
		return subscriptionRef{}, failUsage(fmt.Sprintf("unclear subscription reference %s; matches %d subscriptions", glide.DisplayName(context, name), len(matches)))
	}
}

func lookupSubscriptionUniqueName(client *api.HTTPClient, name string) (subscriptionRef, error) {
	items, err := client.ListAll(subscriptionCollection)
	if err != nil {
		return subscriptionRef{}, fail(err)
	}
	var same, anyCase []subscriptionRef
	for _, item := range items {
		ref := subscriptionFromItem(item)
		if !strings.EqualFold(ref.Name, name) {
			continue
		}
		anyCase = append(anyCase, ref)
		if ref.Name == name {
			same = append(same, ref)
		}
	}
	matches := same
	if len(matches) == 0 {
		matches = anyCase
	}
	switch len(matches) {
	case 0:
		return subscriptionRef{}, failUsage(fmt.Sprintf("no subscription named %q", name))
	case 1:
		ref := matches[0]
		if ref.ID == "" {
			return ref, nil
		}
		return lookupSubscriptionByID(client, ref.ID)
	default:
		return subscriptionRef{}, failUsage(fmt.Sprintf("unclear subscription reference %q; matches %d subscriptions; pass context.name or an id", name, len(matches)))
	}
}

func subscriptionFromItem(item any) subscriptionRef {
	m, _ := item.(map[string]any)
	enabled := true
	switch v := m["enabled"].(type) {
	case bool:
		enabled = v
	}
	return subscriptionRef{
		ID:      firstMapString(m, "id"),
		Name:    mapString(m, "name"),
		Context: mapString(m, "context"),
		Enabled: enabled,
		Spec:    item,
	}
}

func resolveVariableID(client *api.HTTPClient, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", failUsage("variable reference is empty")
	}
	if looksLikeUUID(ref) {
		return ref, nil
	}
	ctx, name, ok := splitContextName(ref)
	if !ok {
		return "", failUsage(fmt.Sprintf("variable %q must be an id or context.name", ref))
	}
	items, err := client.ListAll(variCollection)
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
		return "", failUsage(fmt.Sprintf("no variable named %q in context %q", name, ctx))
	case 1:
		return matches[0], nil
	default:
		return "", failUsage(fmt.Sprintf("unclear variable reference %s; matches %d variables", glide.DisplayName(ctx, name), len(matches)))
	}
}

func redactSubscriptionSpec(v any) any {
	m, ok := v.(map[string]any)
	if !ok || m == nil {
		return v
	}
	cp := map[string]any{}
	for k, val := range m {
		cp[k] = val
	}
	params, ok := cp["paramsObject"].(map[string]any)
	if !ok {
		return cp
	}
	pcp := map[string]any{}
	for k, val := range params {
		pcp[k] = val
	}
	if ohip, ok := pcp["ohip"].(map[string]any); ok {
		ocp := map[string]any{}
		for k, val := range ohip {
			ocp[k] = val
		}
		if s, ok := ocp["clientSecret"].(string); ok && s != "" && !strings.HasPrefix(s, "{{") {
			ocp["clientSecret"] = redactSecret(s)
		}
		pcp["ohip"] = ocp
	}
	cp["paramsObject"] = pcp
	return cp
}

func redactSecret(s string) string {
	if len(s) <= 4 {
		return "********"
	}
	return "********" + s[len(s)-4:]
}

func flagString(cmd *cobra.Command, name string) string {
	s, _ := cmd.Flags().GetString(name)
	return strings.TrimSpace(s)
}
