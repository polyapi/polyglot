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

const tableCollection = "tables"

// Aliases accepted by POST /tables (docs.polyapi.io/tabi_tables). Values are the stored PG types.
var tableColumnTypes = map[string]string{
	"string": "text", "text": "text", "varchar": "text",
	"char":   "char",
	"uuid":   "uuid",
	"number": "numeric", "numeric": "numeric", "decimal": "numeric",
	"float": "double precision", "double": "double precision", "float8": "double precision",
	"int": "integer", "integer": "integer", "int4": "integer",
	"bigint": "bigint", "int8": "bigint",
	"serial":    "serial",
	"bigserial": "bigserial",
	"boolean":   "boolean", "bool": "boolean",
	"date": "date",
	"time": "time", "timesec": "time",
	"timestamp": "timestamptz", "timestamptz": "timestamptz",
	"json": "jsonb", "jsonb": "jsonb", "object": "jsonb",
}

var tableColumnTypeHelp = "string, text, varchar, char, uuid, number, numeric, decimal, float, double, float8, int, integer, int4, bigint, int8, serial, bigserial, boolean, bool, date, time, timesec, timestamp, timestamptz, json, jsonb, object"

type tableRef struct {
	ID         string
	Name       string
	Context    string
	Visibility string
	Spec       any
}

func addTableCommands(root *cobra.Command) {
	table := &cobra.Command{
		Use:   "table",
		Short: "Manage tables and query rows",
		Example: examples(
			ex{"List tables in a context:", "polyapi table list --context billing"},
			ex{"Scaffold a local table file:", "polyapi table init --name orders"},
			ex{"Create a table:", `polyapi table create --name orders --context billing --columns '[{"name":"sku","type":"text","required":true}]'`},
			ex{"Select rows:", `polyapi table rows list orders --context billing --where '{"status":"open"}'`},
		),
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "List tables",
		Args:  cobra.NoArgs,
		RunE:  runTableList,
		Example: examples(
			ex{"List all tables:", "polyapi table list"},
			ex{"Restrict to a context:", "polyapi table list --context billing"},
		),
	}
	list.Flags().String("context", "", "Restrict results to this context prefix")

	get := &cobra.Command{
		Use:   "get <id-or-name>",
		Short: "Get a table by ID, or by name with --context",
		Args:  cobra.ExactArgs(1),
		RunE:  runTableGet,
		Example: examples(
			ex{"Get by ID:", "polyapi table get abc123"},
			ex{"Get by context and name:", "polyapi table get orders --context billing"},
		),
	}
	get.Flags().String("context", "", "Context of the table (required when looking up by name)")

	create := &cobra.Command{
		Use:   "create",
		Short: "Create a table",
		Long:  "Create a table. Tabi adds id, created_at, and updated_at automatically — do not include those columns.\n\nColumn types (aliases): " + tableColumnTypeHelp + ".\nVisibility is ENVIRONMENT or TENANT (not PUBLIC).",
		Args:  cobra.NoArgs,
		RunE:  runTableCreate,
		Example: examples(
			ex{"Create from a columns JSON file:", "polyapi table create --name orders --context billing --columns-file ./orders.columns.json"},
			ex{"Inline columns:", `polyapi table create --name orders --context billing --columns '[{"name":"sku","type":"text","required":true,"unique":true}]'`},
		),
	}
	addTableWriteFlags(create, true)
	_ = create.MarkFlagRequired("name")
	_ = create.MarkFlagRequired("context")
	create.MarkFlagsOneRequired("columns", "columns-file")

	update := &cobra.Command{
		Use:   "update <id-or-name>",
		Short: "Update a table by ID, or by name with --context",
		Long:  "Update table metadata or schema. Schema changes can add or drop columns only — not rename columns or change types. UNIQUE can be added but not removed.",
		Args:  cobra.ExactArgs(1),
		RunE:  runTableUpdate,
		Example: examples(
			ex{"Update the description:", "polyapi table update orders --context billing --description \"Order lines\""},
			ex{"Add a column:", `polyapi table update orders --context billing --columns '[{"name":"sku","type":"text"},{"name":"status","type":"text"}]'`},
		),
	}
	update.Flags().String("context", "", "Context of the table (required when looking up by name)")
	addTableWriteFlags(update, false)
	update.Flags().String("new-context", "", "Move the table to this context")
	update.Flags().String("otp", "", "One-time password when the instance requires MFA for updates")

	del := &cobra.Command{
		Use:   "delete <id-or-name>",
		Short: "Delete a table by ID, or by name with --context",
		Args:  cobra.ExactArgs(1),
		RunE:  runTableDelete,
		Example: examples(
			ex{"Delete by ID:", "polyapi table delete abc123"},
			ex{"Delete by context and name:", "polyapi table delete orders --context billing"},
		),
	}
	del.Flags().String("context", "", "Context of the table (required when looking up by name)")
	del.Flags().String("otp", "", "One-time password when the instance requires MFA for deletes")

	initCmd := newResourceInitCommand(resourceInitKind{
		Use: "table", Noun: "table", Type: glide.TypeTable, NeedsContext: true,
	})

	table.AddCommand(list, get, initCmd, create, update, del)
	addTableRowCommands(table)
	root.AddCommand(table)
}

func addTableWriteFlags(cmd *cobra.Command, creating bool) {
	if creating {
		cmd.Flags().String("name", "", "Table name")
		cmd.Flags().String("context", "", "Table context")
	} else {
		cmd.Flags().String("name", "", "New table name")
	}
	cmd.Flags().String("description", "", "Description")
	visDef := ""
	visHelp := "Visibility: ENVIRONMENT or TENANT"
	if creating {
		visDef = "ENVIRONMENT"
		visHelp += " (default ENVIRONMENT)"
	}
	cmd.Flags().String("visibility", visDef, visHelp)
	cmd.Flags().String("columns", "", "JSON array of columns (`name`, `type`, optional `required`/`unique`/`primary`/`default`/`schema`)")
	cmd.Flags().String("columns-file", "", "Read columns JSON from a file")
}

func runTableList(cmd *cobra.Command, _ []string) error {
	client, err := tableClient(cmd)
	if err != nil {
		return err
	}
	context, _ := cmd.Flags().GetString("context")
	items, err := client.ListAll(tableCollection)
	if err != nil {
		return fail(err)
	}
	var rows []tableRef
	for _, item := range items {
		ref := tableFromItem(item)
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
			fmt.Fprintln(out, "no tables found")
		}
		return nil
	}
	printTableList(out, rows)
	return nil
}

func printTableList(w io.Writer, rows []tableRef) {
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

func runTableGet(cmd *cobra.Command, args []string) error {
	client, err := tableClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveTable(cmd, client, args[0])
	if err != nil {
		return err
	}
	return writeJSON(cmd.OutOrStdout(), ref.Spec)
}

func runTableCreate(cmd *cobra.Command, _ []string) error {
	client, err := tableClient(cmd)
	if err != nil {
		return err
	}
	payload, err := tableWritePayload(cmd, true)
	if err != nil {
		return err
	}
	created, err := client.Create(tableCollection, payload)
	if err != nil {
		return fail(err)
	}
	id := mapString(created, "id")
	name := mapString(payload, "name")
	ctx := mapString(payload, "context")
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("created table %s (%s)", glide.DisplayName(ctx, name), id))
	}
	return nil
}

func runTableUpdate(cmd *cobra.Command, args []string) error {
	client, err := tableClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveTable(cmd, client, args[0])
	if err != nil {
		return err
	}
	payload, err := tableWritePayload(cmd, false)
	if err != nil {
		return err
	}
	if ctx, _ := cmd.Flags().GetString("new-context"); cmd.Flags().Changed("new-context") {
		payload["context"] = ctx
	}
	if len(payload) == 0 {
		return failUsage("pass at least one field to update")
	}
	updated, err := client.Update(tableCollection, ref.ID, payload, otpHeaders(cmd))
	if err != nil {
		return fail(err)
	}
	id := mapString(updated, "id")
	if id == "" {
		id = ref.ID
	}
	name := firstNonEmpty(mapString(updated, "name"), mapString(payload, "name"), ref.Name)
	ctx := firstNonEmpty(mapString(updated, "context"), mapString(payload, "context"), ref.Context)
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("updated table %s (%s)", glide.DisplayName(ctx, name), id))
	}
	return nil
}

func runTableDelete(cmd *cobra.Command, args []string) error {
	client, err := tableClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveTable(cmd, client, args[0])
	if err != nil {
		return err
	}
	if err := client.Delete(tableCollection, ref.ID, otpHeaders(cmd)); err != nil {
		return fail(err)
	}
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("deleted table %s (%s)", glide.DisplayName(ref.Context, ref.Name), ref.ID))
	}
	return nil
}

func tableWritePayload(cmd *cobra.Command, creating bool) (map[string]any, error) {
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
			parsed, err := parseTableVisibility(vis)
			if err != nil {
				return nil, err
			}
			payload["visibility"] = parsed
		}
	}
	if cmd.Flags().Changed("columns") && cmd.Flags().Changed("columns-file") {
		return nil, failUsage("pass only one of --columns or --columns-file")
	}
	if cmd.Flags().Changed("columns") || cmd.Flags().Changed("columns-file") {
		cols, err := readTableColumns(cmd)
		if err != nil {
			return nil, err
		}
		payload["columns"] = cols
	} else if creating {
		return nil, failUsage("pass --columns or --columns-file")
	}
	return payload, nil
}

func readTableColumns(cmd *cobra.Command) ([]any, error) {
	var raw []byte
	if cmd.Flags().Changed("columns-file") {
		path, _ := cmd.Flags().GetString("columns-file")
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, failUsage(fmt.Sprintf("read %s: %v", path, err))
		}
		raw = b
	} else {
		s, _ := cmd.Flags().GetString("columns")
		raw = []byte(s)
	}
	cols, err := parseTableColumns(raw)
	if err != nil {
		return nil, err
	}
	if err := validateTableColumns(cols); err != nil {
		return nil, err
	}
	return cols, nil
}

func parseTableColumns(raw []byte) ([]any, error) {
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return nil, failUsage("columns JSON is empty")
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, failUsage("invalid columns JSON: " + err.Error())
	}
	switch t := v.(type) {
	case []any:
		return t, nil
	case map[string]any:
		if inner, ok := t["columns"].([]any); ok {
			return inner, nil
		}
		return []any{t}, nil
	default:
		return nil, failUsage("columns must be a JSON array of column objects")
	}
}

func validateTableColumns(cols []any) error {
	if cols == nil {
		return failUsage("columns is required")
	}
	for i, raw := range cols {
		m, ok := raw.(map[string]any)
		if !ok {
			return failUsage(fmt.Sprintf("column %d must be an object", i))
		}
		name := strings.TrimSpace(mapString(m, "name"))
		if name == "" {
			return failUsage(fmt.Sprintf("column %d is missing name", i))
		}
		typ := strings.TrimSpace(mapString(m, "type"))
		if typ == "" {
			return failUsage(fmt.Sprintf("column %q is missing type", name))
		}
		if _, ok := tableColumnTypes[strings.ToLower(typ)]; !ok {
			return failUsage(fmt.Sprintf("column %q has unsupported type %q; use one of: %s", name, typ, tableColumnTypeHelp))
		}
		m["type"] = strings.ToLower(typ)
	}
	return nil
}

func parseTableVisibility(s string) (string, error) {
	v := strings.ToUpper(strings.TrimSpace(s))
	switch v {
	case "ENVIRONMENT", "TENANT":
		return v, nil
	case "PUBLIC":
		return "", failUsage("option `visibility` must be ENVIRONMENT or TENANT (tables are not PUBLIC)")
	default:
		return "", failUsage("option `visibility` must be ENVIRONMENT or TENANT")
	}
}

func tableClient(cmd *cobra.Command) (*api.HTTPClient, error) {
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

func resolveTable(cmd *cobra.Command, client *api.HTTPClient, idOrName string) (tableRef, error) {
	context, _ := cmd.Flags().GetString("context")
	if context != "" {
		return lookupTableByName(client, context, idOrName)
	}
	return lookupTableByID(client, idOrName)
}

func lookupTableByID(client *api.HTTPClient, id string) (tableRef, error) {
	spec, err := client.Get(tableCollection, id)
	if err != nil {
		return tableRef{}, fail(err)
	}
	ref := tableFromItem(spec)
	if ref.ID == "" {
		ref.ID = id
	}
	ref.Spec = spec
	return ref, nil
}

func lookupTableByName(client *api.HTTPClient, context, name string) (tableRef, error) {
	items, err := client.ListAll(tableCollection)
	if err != nil {
		return tableRef{}, fail(err)
	}
	var sameCase []tableRef
	var anyCase []tableRef
	for _, item := range items {
		ref := tableFromItem(item)
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
		return tableRef{}, failUsage(fmt.Sprintf("no table named %q in context %q", name, context))
	case 1:
		ref := matches[0]
		if ref.ID == "" {
			return ref, nil
		}
		spec, err := client.Get(tableCollection, ref.ID)
		if err != nil {
			return tableRef{}, fail(err)
		}
		full := tableFromItem(spec)
		full.ID = ref.ID
		full.Spec = spec
		return full, nil
	default:
		return tableRef{}, failUsage(fmt.Sprintf("unclear table reference %s; matches %d tables", glide.DisplayName(context, name), len(matches)))
	}
}

func tableFromItem(item any) tableRef {
	m, _ := item.(map[string]any)
	return tableRef{
		ID:         firstMapString(m, "id"),
		Name:       mapString(m, "name"),
		Context:    mapString(m, "context"),
		Visibility: mapString(m, "visibility"),
		Spec:       item,
	}
}
