package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/polyapi/polyglot/src/api"
	"github.com/polyapi/polyglot/src/glide"
	"github.com/spf13/cobra"
)

const schemaCollection = "schemas"

type schemaRef struct {
	ID         string
	Name       string
	Context    string
	Visibility string
	Spec       any
}

func addSchemaCommands(root *cobra.Command) {
	schema := &cobra.Command{
		Use:   "schema",
		Short: "Manage JSON schemas",
		Example: examples(
			ex{"List schemas in a context:", "polyapi schema list --context billing"},
			ex{"Scaffold a local schema file:", "polyapi schema init --name Order"},
			ex{"Create a schema:", `polyapi schema create --name Order --context billing --definition '{"type":"object"}'`},
			ex{"Get a schema by context.name:", "polyapi schema get billing.Order"},
		),
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "List schemas",
		Args:  cobra.NoArgs,
		RunE:  runSchemaList,
		Example: examples(
			ex{"List all schemas:", "polyapi schema list"},
			ex{"Restrict to a context:", "polyapi schema list --context billing"},
		),
	}
	list.Flags().String("context", "", "Restrict results to this context prefix")

	get := &cobra.Command{
		Use:   "get <id-or-name>",
		Short: "Get a schema by ID, or by name with --context",
		Args:  cobra.ExactArgs(1),
		RunE:  runSchemaGet,
		Example: examples(
			ex{"Get by ID:", "polyapi schema get abc123"},
			ex{"Get by context and name:", "polyapi schema get Order --context billing"},
			ex{"Get by context.name:", "polyapi schema get billing.Order"},
		),
	}
	get.Flags().String("context", "", "Context of the schema (required when looking up by name)")

	create := &cobra.Command{
		Use:   "create",
		Short: "Create a schema",
		Long:  "Create a JSON Schema. The definition is a JSON Schema object; if `$schema` is omitted the platform defaults to draft-06. Other schemas are referenced with `x-poly-ref` `{path}` (and optional `publicNamespace`). OpenAPI train also upserts schemas (`polyapi model train`).",
		Args:  cobra.NoArgs,
		RunE:  runSchemaCreate,
		Example: examples(
			ex{"Inline definition:", `polyapi schema create --name Order --context billing --definition '{"type":"object","properties":{"sku":{"type":"string"}}}'`},
			ex{"From a file:", "polyapi schema create --name Order --context billing --definition-file ./order.schema.json"},
		),
	}
	addSchemaWriteFlags(create, true)
	_ = create.MarkFlagRequired("name")
	_ = create.MarkFlagRequired("context")
	create.MarkFlagsOneRequired("definition", "definition-file")

	update := &cobra.Command{
		Use:   "update <id-or-name>",
		Short: "Update a schema by ID, or by name with --context",
		Args:  cobra.ExactArgs(1),
		RunE:  runSchemaUpdate,
		Example: examples(
			ex{"Replace the definition:", `polyapi schema update Order --context billing --definition '{"type":"object"}'`},
			ex{"Rename:", "polyapi schema update Order --context billing --name OrderV2"},
		),
	}
	update.Flags().String("context", "", "Context of the schema (required when looking up by name)")
	addSchemaWriteFlags(update, false)
	update.Flags().String("new-context", "", "Move the schema to this context")

	del := &cobra.Command{
		Use:   "delete <id-or-name>",
		Short: "Delete a schema by ID, or by name with --context",
		Args:  cobra.ExactArgs(1),
		RunE:  runSchemaDelete,
		Example: examples(
			ex{"Delete by ID:", "polyapi schema delete abc123"},
			ex{"Delete by context and name:", "polyapi schema delete Order --context billing"},
		),
	}
	del.Flags().String("context", "", "Context of the schema (required when looking up by name)")

	initCmd := newResourceInitCommand(resourceInitKind{
		Use: "schema", Noun: "schema", Type: glide.TypeSchema, NeedsContext: true,
	})

	schema.AddCommand(list, get, initCmd, create, update, del)
	root.AddCommand(schema)
}

func addSchemaWriteFlags(cmd *cobra.Command, creating bool) {
	if creating {
		cmd.Flags().String("name", "", "Schema name")
		cmd.Flags().String("context", "", "Schema context")
	} else {
		cmd.Flags().String("name", "", "New schema name")
	}
	visDef := ""
	visHelp := "Visibility: PUBLIC, TENANT, or ENVIRONMENT"
	if creating {
		visDef = "ENVIRONMENT"
		visHelp += " (default ENVIRONMENT)"
	}
	cmd.Flags().String("visibility", visDef, visHelp)
	cmd.Flags().String("definition", "", "JSON Schema object (or a `{definition: …}` wrapper)")
	cmd.Flags().String("definition-file", "", "Read the JSON Schema from a file")
}

func runSchemaList(cmd *cobra.Command, _ []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	context, _ := cmd.Flags().GetString("context")
	items, err := client.ListAll(schemaCollection)
	if err != nil {
		return fail(err)
	}
	var rows []schemaRef
	for _, item := range items {
		ref := schemaFromItem(item)
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
			fmt.Fprintln(out, "no schemas found")
		}
		return nil
	}
	printSchemaList(out, rows)
	return nil
}

func printSchemaList(w io.Writer, rows []schemaRef) {
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

func runSchemaGet(cmd *cobra.Command, args []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveSchema(cmd, client, args[0])
	if err != nil {
		return err
	}
	return writeJSON(cmd.OutOrStdout(), ref.Spec)
}

func runSchemaCreate(cmd *cobra.Command, _ []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	payload, err := schemaWritePayload(cmd, true)
	if err != nil {
		return err
	}
	created, err := client.Create(schemaCollection, payload)
	if err != nil {
		return fail(err)
	}
	id := mapString(created, "id")
	name := mapString(payload, "name")
	ctx := mapString(payload, "context")
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("created schema %s (%s)", glide.DisplayName(ctx, name), id))
	}
	return nil
}

func runSchemaUpdate(cmd *cobra.Command, args []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveSchema(cmd, client, args[0])
	if err != nil {
		return err
	}
	payload, err := schemaWritePayload(cmd, false)
	if err != nil {
		return err
	}
	if ctx, _ := cmd.Flags().GetString("new-context"); cmd.Flags().Changed("new-context") {
		payload["context"] = ctx
	}
	if len(payload) == 0 {
		return failUsage("pass at least one field to update")
	}
	updated, err := client.Update(schemaCollection, ref.ID, payload, nil)
	if err != nil {
		return fail(err)
	}
	id := firstNonEmpty(mapString(updated, "id"), ref.ID)
	name := firstNonEmpty(mapString(updated, "name"), mapString(payload, "name"), ref.Name)
	ctx := firstNonEmpty(mapString(updated, "context"), mapString(payload, "context"), ref.Context)
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("updated schema %s (%s)", glide.DisplayName(ctx, name), id))
	}
	return nil
}

func runSchemaDelete(cmd *cobra.Command, args []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveSchema(cmd, client, args[0])
	if err != nil {
		return err
	}
	if err := client.Delete(schemaCollection, ref.ID, nil); err != nil {
		return fail(err)
	}
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("deleted schema %s (%s)", glide.DisplayName(ref.Context, ref.Name), ref.ID))
	}
	return nil
}

func schemaWritePayload(cmd *cobra.Command, creating bool) (map[string]any, error) {
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
	if cmd.Flags().Changed("definition") && cmd.Flags().Changed("definition-file") {
		return nil, failUsage("pass only one of --definition or --definition-file")
	}
	if cmd.Flags().Changed("definition") || cmd.Flags().Changed("definition-file") {
		def, err := readSchemaDefinition(cmd)
		if err != nil {
			return nil, err
		}
		payload["definition"] = def
	} else if creating {
		return nil, failUsage("pass --definition or --definition-file")
	}
	return payload, nil
}

func readSchemaDefinition(cmd *cobra.Command) (map[string]any, error) {
	var raw []byte
	if cmd.Flags().Changed("definition-file") {
		path, _ := cmd.Flags().GetString("definition-file")
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, failUsage(fmt.Sprintf("read %s: %v", path, err))
		}
		raw = b
	} else {
		s, _ := cmd.Flags().GetString("definition")
		raw = []byte(s)
	}
	return parseSchemaDefinition(raw)
}

func parseSchemaDefinition(raw []byte) (map[string]any, error) {
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return nil, failUsage("definition JSON is empty")
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, failUsage("invalid definition JSON: " + err.Error())
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, failUsage("definition must be a JSON object")
	}
	if inner, ok := m["definition"].(map[string]any); ok {
		return inner, nil
	}
	return m, nil
}

func resolveSchema(cmd *cobra.Command, client *api.HTTPClient, idOrName string) (schemaRef, error) {
	context, _ := cmd.Flags().GetString("context")
	if context != "" {
		return lookupSchemaByName(client, context, idOrName)
	}
	if !looksLikeUUID(idOrName) {
		if ctx, name, ok := splitContextName(idOrName); ok {
			return lookupSchemaByName(client, ctx, name)
		}
	}
	return lookupSchemaByID(client, idOrName)
}

func lookupSchemaByID(client *api.HTTPClient, id string) (schemaRef, error) {
	spec, err := client.Get(schemaCollection, id)
	if err != nil {
		return schemaRef{}, fail(err)
	}
	ref := schemaFromItem(spec)
	if ref.ID == "" {
		ref.ID = id
	}
	ref.Spec = spec
	return ref, nil
}

func lookupSchemaByName(client *api.HTTPClient, context, name string) (schemaRef, error) {
	items, err := client.ListAll(schemaCollection)
	if err != nil {
		return schemaRef{}, fail(err)
	}
	var sameCase []schemaRef
	var anyCase []schemaRef
	for _, item := range items {
		ref := schemaFromItem(item)
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
		return schemaRef{}, failUsage(fmt.Sprintf("no schema named %q in context %q", name, context))
	case 1:
		ref := matches[0]
		if ref.ID == "" {
			return ref, nil
		}
		spec, err := client.Get(schemaCollection, ref.ID)
		if err != nil {
			return schemaRef{}, fail(err)
		}
		full := schemaFromItem(spec)
		full.ID = ref.ID
		full.Spec = spec
		return full, nil
	default:
		return schemaRef{}, failUsage(fmt.Sprintf("unclear schema reference %s; matches %d schemas", glide.DisplayName(context, name), len(matches)))
	}
}

func schemaFromItem(item any) schemaRef {
	m, _ := item.(map[string]any)
	return schemaRef{
		ID:         firstMapString(m, "id"),
		Name:       mapString(m, "name"),
		Context:    mapString(m, "context"),
		Visibility: mapString(m, "visibility"),
		Spec:       item,
	}
}
