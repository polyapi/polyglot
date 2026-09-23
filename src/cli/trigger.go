package cli

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/polyapi/polyglot/src/api"
	"github.com/polyapi/polyglot/src/glide"
	"github.com/spf13/cobra"
)

const triggerCollection = "triggers"

type triggerRef struct {
	ID               string
	Name             string
	WebhookHandleID  string
	ServerFunctionID string
	WaitForResponse  bool
	Enabled          bool
	Spec             any
}

func addTriggerCommands(root *cobra.Command) {
	tr := &cobra.Command{
		Use:   "trigger",
		Short: "Manage triggers (webhook or error-handler → server function)",
		Example: examples(
			ex{"List triggers:", "polyapi trigger list"},
			ex{"Scaffold a local trigger file:", "polyapi trigger init --name weekly --type webhook"},
			ex{"Link a webhook to a server function:", "polyapi trigger create --name weekly --webhook billing.hook --function billing.weeklyReport"},
			ex{"Link an error handler to a server function:", "polyapi trigger create --name on-error --error-handler-path billing.handle --function billing.weeklyReport"},
		),
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "List triggers",
		Args:  cobra.NoArgs,
		RunE:  runTriggerList,
		Example: examples(
			ex{"List all triggers:", "polyapi trigger list"},
		),
	}

	get := &cobra.Command{
		Use:   "get <id-or-name>",
		Short: "Get a trigger by ID or name",
		Args:  cobra.ExactArgs(1),
		RunE:  runTriggerGet,
		Example: examples(
			ex{"Get by ID:", "polyapi trigger get abc123"},
			ex{"Get by name:", "polyapi trigger get weekly"},
		),
	}

	create := &cobra.Command{
		Use:   "create",
		Short: "Create a trigger that links a webhook or error handler to a server function",
		Args:  cobra.NoArgs,
		RunE:  runTriggerCreate,
		Example: examples(
			ex{"Webhook to server function:", "polyapi trigger create --name weekly --webhook billing.hook --function billing.weeklyReport"},
			ex{"Return the function result to the HTTP caller:", "polyapi trigger create --webhook billing.hook --function billing.weeklyReport --wait-for-response"},
			ex{"Error handler to server function:", "polyapi trigger create --name on-error --error-handler-path billing.handle --function billing.weeklyReport"},
		),
	}
	create.Flags().String("name", "", "Trigger name (generated from source and destination if omitted)")
	create.Flags().String("webhook", "", "Source webhook (`id` or `context.name`)")
	create.Flags().String("function", "", "Destination server function (`id` or `context.name`)")
	create.Flags().String("error-handler-path", "", "Error-handler source path (instead of --webhook)")
	create.Flags().Bool("wait-for-response", true, "Wait for the server function and use its return value as the HTTP response")
	create.Flags().Bool("enabled", true, "Whether the trigger is enabled")
	_ = create.MarkFlagRequired("function")
	create.MarkFlagsOneRequired("webhook", "error-handler-path")

	update := &cobra.Command{
		Use:   "update <id-or-name>",
		Short: "Update a trigger (name, wait-for-response, enabled only)",
		Long:  "The platform's update DTO only patches name, waitForResponse, and enabled. Source and destination are fixed at create time — delete and recreate the trigger to change them.",
		Args:  cobra.ExactArgs(1),
		RunE:  runTriggerUpdate,
		Example: examples(
			ex{"Rename:", "polyapi trigger update weekly --name weekly-v2"},
			ex{"Stop waiting for the function:", "polyapi trigger update weekly --wait-for-response=false"},
		),
	}
	update.Flags().String("name", "", "New trigger name")
	update.Flags().Bool("wait-for-response", true, "Wait for the server function before returning")
	update.Flags().Bool("enabled", true, "Whether the trigger is enabled")

	del := &cobra.Command{
		Use:   "delete <id-or-name>",
		Short: "Delete a trigger by ID or name",
		Args:  cobra.ExactArgs(1),
		RunE:  runTriggerDelete,
		Example: examples(
			ex{"Delete by name:", "polyapi trigger delete weekly"},
		),
	}

	initCmd := newResourceInitCommand(resourceInitKind{
		Use: "trigger", Noun: "trigger", Type: glide.TypeTrigger, NeedsContext: false, TypeMode: initTypeTrigger,
	})

	tr.AddCommand(list, get, initCmd, create, update, del)
	root.AddCommand(tr)
}

func runTriggerList(cmd *cobra.Command, _ []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	items, err := client.ListAll(triggerCollection)
	if err != nil {
		return fail(err)
	}
	var rows []triggerRef
	for _, item := range items {
		rows = append(rows, triggerFromItem(item))
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Name != rows[j].Name {
			return rows[i].Name < rows[j].Name
		}
		return rows[i].ID < rows[j].ID
	})
	out := cmd.OutOrStdout()
	if len(rows) == 0 {
		if !globalsFrom(cmd).Quiet {
			fmt.Fprintln(out, "no triggers found")
		}
		return nil
	}
	printTriggerList(out, rows)
	return nil
}

func printTriggerList(w io.Writer, rows []triggerRef) {
	nameW, enW := len("NAME"), len("ENABLED")
	for _, row := range rows {
		if n := len(row.Name); n > nameW {
			nameW = n
		}
		if n := len(strconv.FormatBool(row.Enabled)); n > enW {
			enW = n
		}
	}
	fmt.Fprintln(w, infoText(fmt.Sprintf("%-*s  %-*s  %s", nameW, "NAME", enW, "ENABLED", "ID")))
	for _, row := range rows {
		name := row.Name
		if name == "" {
			name = "-"
		}
		fmt.Fprintf(w, "%-*s  %-*s  %s\n", nameW, name, enW, strconv.FormatBool(row.Enabled), row.ID)
	}
}

func runTriggerGet(cmd *cobra.Command, args []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveTrigger(client, args[0])
	if err != nil {
		return err
	}
	return writeJSON(cmd.OutOrStdout(), ref.Spec)
}

func runTriggerCreate(cmd *cobra.Command, _ []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	fn, _ := cmd.Flags().GetString("function")
	fnID, err := resolveServerFunctionID(client, fn)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"destination": map[string]any{"serverFunctionId": fnID},
		"enabled":     boolFlagOr(cmd, "enabled", true),
	}
	if name, _ := cmd.Flags().GetString("name"); name != "" {
		payload["name"] = name
	}
	if cmd.Flags().Changed("webhook") {
		wh, _ := cmd.Flags().GetString("webhook")
		whID, err := resolveWebhookID(client, wh)
		if err != nil {
			return err
		}
		payload["source"] = map[string]any{"webhookHandleId": whID}
		payload["waitForResponse"] = boolFlagOr(cmd, "wait-for-response", true)
	} else {
		path, _ := cmd.Flags().GetString("error-handler-path")
		payload["source"] = map[string]any{"errorHandler": map[string]any{"path": path}}
		if cmd.Flags().Changed("wait-for-response") {
			v, _ := cmd.Flags().GetBool("wait-for-response")
			payload["waitForResponse"] = v
		}
	}
	created, err := client.Create(triggerCollection, payload)
	if err != nil {
		return fail(err)
	}
	id := mapString(created, "id")
	name := firstNonEmpty(mapString(created, "name"), mapString(payload, "name"), id)
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("created trigger %s (%s)", name, id))
	}
	return nil
}

func runTriggerUpdate(cmd *cobra.Command, args []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveTrigger(client, args[0])
	if err != nil {
		return err
	}
	payload := map[string]any{}
	if cmd.Flags().Changed("name") {
		name, _ := cmd.Flags().GetString("name")
		payload["name"] = name
	}
	if cmd.Flags().Changed("wait-for-response") {
		v, _ := cmd.Flags().GetBool("wait-for-response")
		payload["waitForResponse"] = v
	}
	if cmd.Flags().Changed("enabled") {
		v, _ := cmd.Flags().GetBool("enabled")
		payload["enabled"] = v
	}
	if len(payload) == 0 {
		return failUsage("pass at least one of --name, --wait-for-response, or --enabled")
	}
	updated, err := client.Update(triggerCollection, ref.ID, payload, nil)
	if err != nil {
		return fail(err)
	}
	id := firstNonEmpty(mapString(updated, "id"), ref.ID)
	name := firstNonEmpty(mapString(updated, "name"), mapString(payload, "name"), ref.Name)
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("updated trigger %s (%s)", name, id))
	}
	return nil
}

func runTriggerDelete(cmd *cobra.Command, args []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveTrigger(client, args[0])
	if err != nil {
		return err
	}
	if err := client.Delete(triggerCollection, ref.ID, nil); err != nil {
		return fail(err)
	}
	name := ref.Name
	if name == "" {
		name = ref.ID
	}
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("deleted trigger %s (%s)", name, ref.ID))
	}
	return nil
}

func boolFlagOr(cmd *cobra.Command, name string, fallback bool) bool {
	if !cmd.Flags().Changed(name) {
		return fallback
	}
	v, _ := cmd.Flags().GetBool(name)
	return v
}

func resolveTrigger(client *api.HTTPClient, idOrName string) (triggerRef, error) {
	if looksLikeUUID(idOrName) {
		return lookupTriggerByID(client, idOrName)
	}
	return lookupTriggerByName(client, idOrName)
}

func lookupTriggerByID(client *api.HTTPClient, id string) (triggerRef, error) {
	spec, err := client.Get(triggerCollection, id)
	if err != nil {
		return triggerRef{}, fail(err)
	}
	ref := triggerFromItem(spec)
	if ref.ID == "" {
		ref.ID = id
	}
	ref.Spec = spec
	return ref, nil
}

func lookupTriggerByName(client *api.HTTPClient, name string) (triggerRef, error) {
	items, err := client.ListAll(triggerCollection)
	if err != nil {
		return triggerRef{}, fail(err)
	}
	var same, anyCase []triggerRef
	for _, item := range items {
		ref := triggerFromItem(item)
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
		return triggerRef{}, failUsage(fmt.Sprintf("no trigger named %q", name))
	case 1:
		ref := matches[0]
		if ref.ID == "" {
			return ref, nil
		}
		spec, err := client.Get(triggerCollection, ref.ID)
		if err != nil {
			return triggerRef{}, fail(err)
		}
		full := triggerFromItem(spec)
		full.ID = ref.ID
		full.Spec = spec
		return full, nil
	default:
		return triggerRef{}, failUsage(fmt.Sprintf("unclear trigger reference %q; matches %d triggers", name, len(matches)))
	}
}

func triggerFromItem(item any) triggerRef {
	m, _ := item.(map[string]any)
	enabled, _ := m["enabled"].(bool)
	wait, _ := m["waitForResponse"].(bool)
	src, _ := m["source"].(map[string]any)
	dest, _ := m["destination"].(map[string]any)
	wh := firstNonEmpty(mapString(m, "webhookHandleId"), mapString(src, "webhookHandleId"))
	fn := firstNonEmpty(mapString(m, "serverFunctionId"), mapString(dest, "serverFunctionId"))
	return triggerRef{
		ID:               firstMapString(m, "id"),
		Name:             mapString(m, "name"),
		WebhookHandleID:  wh,
		ServerFunctionID: fn,
		WaitForResponse:  wait,
		Enabled:          enabled,
		Spec:             item,
	}
}
