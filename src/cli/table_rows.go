package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func addTableRowCommands(table *cobra.Command) {
	rows := &cobra.Command{
		Use:   "rows",
		Short: "Table row operations",
		Example: examples(
			ex{"List rows:", "polyapi table rows list orders --context billing"},
			ex{"Insert rows:", `polyapi table rows insert orders --context billing --data '[{"sku":"A-1"}]'`},
		),
	}

	list := &cobra.Command{
		Use:   "list <id-or-name>",
		Short: "Select rows from a table",
		Args:  cobra.ExactArgs(1),
		RunE:  runTableRowsList,
		Example: examples(
			ex{"List rows:", "polyapi table rows list orders --context billing"},
			ex{"Filter and page:", `polyapi table rows list orders --context billing --where '{"status":"open"}' --limit 50 --offset 0 --order-by '{"id":"asc"}'`},
		),
	}
	addTableSelectFlags(list)

	get := &cobra.Command{
		Use:   "get <id-or-name> <row-id>",
		Short: "Get one row by its id",
		Args:  cobra.ExactArgs(2),
		RunE:  runTableRowsGet,
		Example: examples(
			ex{"Get a row:", "polyapi table rows get orders abc123 --context billing"},
		),
	}
	get.Flags().String("context", "", "Context of the table (required when looking up the table by name)")

	insert := &cobra.Command{
		Use:   "insert <id-or-name>",
		Short: "Insert rows",
		Args:  cobra.ExactArgs(1),
		RunE:  runTableRowsInsert,
		Example: examples(
			ex{"Insert one row:", `polyapi table rows insert orders --context billing --data '{"sku":"A-1"}'`},
			ex{"Insert from a file:", "polyapi table rows insert orders --context billing --data-file ./rows.json"},
		),
	}
	addTableRowTableFlag(insert)
	addTableDataFlags(insert)
	insert.MarkFlagsOneRequired("data", "data-file")

	upsert := &cobra.Command{
		Use:   "upsert <id-or-name>",
		Short: "Insert or update rows (match on one unique column)",
		Args:  cobra.ExactArgs(1),
		RunE:  runTableRowsUpsert,
		Example: examples(
			ex{"Upsert by unique sku:", `polyapi table rows upsert orders --context billing --data '{"sku":"A-1","status":"open"}'`},
		),
	}
	addTableRowTableFlag(upsert)
	addTableDataFlags(upsert)
	upsert.MarkFlagsOneRequired("data", "data-file")

	update := &cobra.Command{
		Use:   "update <id-or-name>",
		Short: "Update rows matching --where",
		Args:  cobra.ExactArgs(1),
		RunE:  runTableRowsUpdate,
		Example: examples(
			ex{"Update matching rows:", `polyapi table rows update orders --context billing --where '{"sku":"A-1"}' --data '{"status":"closed"}'`},
		),
	}
	addTableRowTableFlag(update)
	addTableWhereFlags(update, false)
	addTableDataFlags(update)
	update.MarkFlagsOneRequired("data", "data-file")

	del := &cobra.Command{
		Use:   "delete <id-or-name>",
		Short: "Delete rows matching --where",
		Args:  cobra.ExactArgs(1),
		RunE:  runTableRowsDelete,
		Example: examples(
			ex{"Delete matching rows:", `polyapi table rows delete orders --context billing --where '{"sku":"A-1"}'`},
			ex{"Delete every row:", "polyapi table rows delete orders --context billing --all"},
		),
	}
	addTableRowTableFlag(del)
	addTableWhereFlags(del, true)

	query := &cobra.Command{
		Use:   "query <id-or-name>",
		Short: "Select rows and print the full query result (results + pagination)",
		Args:  cobra.ExactArgs(1),
		RunE:  runTableRowsQuery,
		Example: examples(
			ex{"Query with a where clause:", `polyapi table rows query orders --context billing --where '{"status":"open"}' --limit 50 --order-by '{"id":"asc"}'`},
			ex{"Query from a JSON body:", "polyapi table rows query orders --context billing --query-file ./select.json"},
		),
	}
	addTableSelectFlags(query)
	query.Flags().String("query", "", "Full select JSON body (`where`, `limit`, `offset`, `orderBy`)")
	query.Flags().String("query-file", "", "Read the select JSON body from a file")

	count := &cobra.Command{
		Use:   "count <id-or-name>",
		Short: "Count rows matching --where",
		Args:  cobra.ExactArgs(1),
		RunE:  runTableRowsCount,
		Example: examples(
			ex{"Count all rows:", "polyapi table rows count orders --context billing"},
			ex{"Count matching rows:", `polyapi table rows count orders --context billing --where '{"status":"open"}'`},
		),
	}
	addTableRowTableFlag(count)
	count.Flags().String("where", "", "JSON object of column filters")

	rows.AddCommand(list, get, insert, upsert, update, del, query, count)
	table.AddCommand(rows)
}

func addTableRowTableFlag(cmd *cobra.Command) {
	cmd.Flags().String("context", "", "Context of the table (required when looking up the table by name)")
}

func addTableSelectFlags(cmd *cobra.Command) {
	addTableRowTableFlag(cmd)
	cmd.Flags().String("where", "", "JSON object of column filters")
	cmd.Flags().Uint("limit", 0, "Maximum rows to return (platform max 1000)")
	cmd.Flags().Uint("offset", 0, "Number of rows to skip")
	cmd.Flags().String("order-by", "", "JSON object of column to `asc` or `desc` (required when paginating with --offset)")
}

func addTableWhereFlags(cmd *cobra.Command, allowAll bool) {
	cmd.Flags().String("where", "", "JSON object of column filters")
	if allowAll {
		cmd.Flags().Bool("all", false, "Apply to every row (required when --where is omitted)")
	}
}

func addTableDataFlags(cmd *cobra.Command) {
	cmd.Flags().String("data", "", "JSON object or array of row objects")
	cmd.Flags().String("data-file", "", "Read row JSON from a file")
}

func runTableRowsList(cmd *cobra.Command, args []string) error {
	body, err := tableSelectBody(cmd)
	if err != nil {
		return err
	}
	result, err := postTableAction(cmd, args[0], "select", body)
	if err != nil {
		return err
	}
	rows := tableQueryResults(result)
	if len(rows) == 0 {
		if !globalsFrom(cmd).Quiet {
			fmt.Fprintln(cmd.OutOrStdout(), "no rows found")
		}
		return nil
	}
	return writeJSON(cmd.OutOrStdout(), rows)
}

func runTableRowsGet(cmd *cobra.Command, args []string) error {
	body := map[string]any{"where": map[string]any{"id": args[1]}, "limit": 2}
	result, err := postTableAction(cmd, args[0], "select", body)
	if err != nil {
		return err
	}
	rows := tableQueryResults(result)
	switch len(rows) {
	case 0:
		return failUsage(fmt.Sprintf("no row %q in table %s", args[1], args[0]))
	case 1:
		return writeJSON(cmd.OutOrStdout(), rows[0])
	default:
		return failUsage(fmt.Sprintf("unclear row reference %q; matches %d rows", args[1], len(rows)))
	}
}

func runTableRowsInsert(cmd *cobra.Command, args []string) error {
	return runTableRowsWrite(cmd, args[0], "insert")
}

func runTableRowsUpsert(cmd *cobra.Command, args []string) error {
	return runTableRowsWrite(cmd, args[0], "upsert")
}

func runTableRowsWrite(cmd *cobra.Command, idOrName, action string) error {
	data, err := readTableRowData(cmd)
	if err != nil {
		return err
	}
	result, err := postTableAction(cmd, idOrName, action, map[string]any{"data": data})
	if err != nil {
		return err
	}
	return writeJSON(cmd.OutOrStdout(), result)
}

func runTableRowsUpdate(cmd *cobra.Command, args []string) error {
	where, err := requireTableWhere(cmd, false)
	if err != nil {
		return err
	}
	data, err := readTableRowDataObject(cmd)
	if err != nil {
		return err
	}
	body := map[string]any{"data": data}
	if where != nil {
		body["where"] = where
	}
	result, err := postTableAction(cmd, args[0], "update", body)
	if err != nil {
		return err
	}
	return writeJSON(cmd.OutOrStdout(), result)
}

func runTableRowsDelete(cmd *cobra.Command, args []string) error {
	where, err := requireTableWhere(cmd, true)
	if err != nil {
		return err
	}
	body := map[string]any{}
	if where != nil {
		body["where"] = where
	}
	result, err := postTableAction(cmd, args[0], "delete", body)
	if err != nil {
		return err
	}
	return writeJSON(cmd.OutOrStdout(), result)
}

func runTableRowsQuery(cmd *cobra.Command, args []string) error {
	body, err := tableQueryBody(cmd)
	if err != nil {
		return err
	}
	result, err := postTableAction(cmd, args[0], "select", body)
	if err != nil {
		return err
	}
	return writeJSON(cmd.OutOrStdout(), result)
}

func runTableRowsCount(cmd *cobra.Command, args []string) error {
	body := map[string]any{}
	if cmd.Flags().Changed("where") {
		where, err := parseJSONObjectFlag(cmd, "where")
		if err != nil {
			return err
		}
		body["where"] = where
	}
	result, err := postTableAction(cmd, args[0], "count", body)
	if err != nil {
		return err
	}
	return writeJSON(cmd.OutOrStdout(), result)
}

func postTableAction(cmd *cobra.Command, idOrName, action string, body map[string]any) (any, error) {
	client, err := tableClient(cmd)
	if err != nil {
		return nil, err
	}
	ref, err := resolveTable(cmd, client, idOrName)
	if err != nil {
		return nil, err
	}
	if body == nil {
		body = map[string]any{}
	}
	result, err := client.PostAction(tableCollection, ref.ID, action, body)
	if err != nil {
		return nil, fail(err)
	}
	return result, nil
}

func tableQueryResults(result any) []any {
	m, ok := result.(map[string]any)
	if !ok {
		if arr, ok := result.([]any); ok {
			return arr
		}
		if result == nil {
			return nil
		}
		return []any{result}
	}
	if arr, ok := m["results"].([]any); ok {
		return arr
	}
	return nil
}

func tableSelectBody(cmd *cobra.Command) (map[string]any, error) {
	body := map[string]any{}
	if cmd.Flags().Changed("where") {
		where, err := parseJSONObjectFlag(cmd, "where")
		if err != nil {
			return nil, err
		}
		body["where"] = where
	}
	if cmd.Flags().Changed("limit") {
		limit, _ := cmd.Flags().GetUint("limit")
		body["limit"] = limit
	}
	if cmd.Flags().Changed("offset") {
		offset, _ := cmd.Flags().GetUint("offset")
		body["offset"] = offset
		if !cmd.Flags().Changed("order-by") {
			return nil, failUsage("pass --order-by when using --offset so pages stay stable")
		}
	}
	if cmd.Flags().Changed("order-by") {
		order, err := parseOrderByFlag(cmd)
		if err != nil {
			return nil, err
		}
		body["orderBy"] = order
	}
	return body, nil
}

func tableQueryBody(cmd *cobra.Command) (map[string]any, error) {
	if cmd.Flags().Changed("query") && cmd.Flags().Changed("query-file") {
		return nil, failUsage("pass only one of --query or --query-file")
	}
	if cmd.Flags().Changed("query") || cmd.Flags().Changed("query-file") {
		raw, err := readFlagOrFile(cmd, "query", "query-file")
		if err != nil {
			return nil, err
		}
		obj, err := parseJSONObject(raw, "query")
		if err != nil {
			return nil, err
		}
		return obj, nil
	}
	return tableSelectBody(cmd)
}

func requireTableWhere(cmd *cobra.Command, allowAll bool) (map[string]any, error) {
	all := false
	if allowAll {
		all, _ = cmd.Flags().GetBool("all")
	}
	if cmd.Flags().Changed("where") && all {
		return nil, failUsage("pass only one of --where or --all")
	}
	if cmd.Flags().Changed("where") {
		return parseJSONObjectFlag(cmd, "where")
	}
	if all {
		return nil, nil
	}
	if allowAll {
		return nil, failUsage("pass --where or --all")
	}
	return nil, failUsage("pass --where")
}

func readTableRowData(cmd *cobra.Command) ([]any, error) {
	if cmd.Flags().Changed("data") && cmd.Flags().Changed("data-file") {
		return nil, failUsage("pass only one of --data or --data-file")
	}
	if !cmd.Flags().Changed("data") && !cmd.Flags().Changed("data-file") {
		return nil, failUsage("pass --data or --data-file")
	}
	raw, err := readFlagOrFile(cmd, "data", "data-file")
	if err != nil {
		return nil, err
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, failUsage("invalid --data JSON: " + err.Error())
	}
	switch t := v.(type) {
	case []any:
		if len(t) == 0 {
			return nil, failUsage("data must contain at least one row")
		}
		for i, row := range t {
			if _, ok := row.(map[string]any); !ok {
				return nil, failUsage(fmt.Sprintf("data[%d] must be a JSON object", i))
			}
		}
		return t, nil
	case map[string]any:
		return []any{t}, nil
	default:
		return nil, failUsage("--data must be a JSON object or array of objects")
	}
}

func readTableRowDataObject(cmd *cobra.Command) (map[string]any, error) {
	rows, err := readTableRowData(cmd)
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, failUsage("update --data must be a single JSON object of columns to set")
	}
	m, _ := rows[0].(map[string]any)
	if len(m) == 0 {
		return nil, failUsage("update --data must include at least one column")
	}
	return m, nil
}

func parseJSONObjectFlag(cmd *cobra.Command, name string) (map[string]any, error) {
	raw, _ := cmd.Flags().GetString(name)
	return parseJSONObject([]byte(raw), name)
}

func parseJSONObject(raw []byte, name string) (map[string]any, error) {
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return nil, failUsage(fmt.Sprintf("option `%s` must be a JSON object", name))
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, failUsage(fmt.Sprintf("invalid --%s JSON: %s", name, err.Error()))
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, failUsage(fmt.Sprintf("option `%s` must be a JSON object", name))
	}
	return m, nil
}

func parseOrderByFlag(cmd *cobra.Command) (map[string]any, error) {
	order, err := parseJSONObjectFlag(cmd, "order-by")
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	for k, v := range order {
		s, ok := v.(string)
		if !ok {
			return nil, failUsage("option `order-by` values must be asc or desc")
		}
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "asc", "desc":
			out[k] = strings.ToLower(strings.TrimSpace(s))
		default:
			return nil, failUsage("option `order-by` values must be asc or desc")
		}
	}
	if len(out) == 0 {
		return nil, failUsage("option `order-by` must include at least one column")
	}
	return out, nil
}

func readFlagOrFile(cmd *cobra.Command, flagName, fileFlag string) ([]byte, error) {
	if cmd.Flags().Changed(fileFlag) {
		path, _ := cmd.Flags().GetString(fileFlag)
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, failUsage(fmt.Sprintf("read %s: %v", path, err))
		}
		return b, nil
	}
	s, _ := cmd.Flags().GetString(flagName)
	return []byte(s), nil
}
