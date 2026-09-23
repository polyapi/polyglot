package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/polyapi/polyglot/src/api"
	"github.com/polyapi/polyglot/src/glide"
	"github.com/spf13/cobra"
)

const jobCollection = "jobs"

type jobRef struct {
	ID            string
	Name          string
	Enabled       bool
	ExecutionType string
	Schedule      any
	Spec          any
}

func addJobCommands(root *cobra.Command) {
	job := &cobra.Command{
		Use:   "job",
		Short: "Manage jobs and inspect executions",
		Example: examples(
			ex{"List jobs:", "polyapi job list"},
			ex{"Scaffold a local job file:", "polyapi job init --name nightly"},
			ex{"Create a cron job:", `polyapi job create --name nightly --cron "0 3 * * *" --function billing.weeklyReport`},
			ex{"Run a job now:", "polyapi job run nightly"},
		),
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "List jobs",
		Args:  cobra.NoArgs,
		RunE:  runJobList,
		Example: examples(
			ex{"List all jobs:", "polyapi job list"},
		),
	}

	get := &cobra.Command{
		Use:   "get <id-or-name>",
		Short: "Get a job by ID or name",
		Args:  cobra.ExactArgs(1),
		RunE:  runJobGet,
		Example: examples(
			ex{"Get by ID:", "polyapi job get abc123"},
			ex{"Get by name:", "polyapi job get nightly"},
		),
	}

	create := &cobra.Command{
		Use:   "create",
		Short: "Create a job",
		Long:  "Create a scheduled or manual job. Jobs have no context; lookup is by ID or name. Functions are server functions (`id` or `context.name`). Schedule types: periodical (cron), interval (minutes), or on_time (ISO 8601).",
		Args:  cobra.NoArgs,
		RunE:  runJobCreate,
		Example: examples(
			ex{"Cron (periodical):", `polyapi job create --name nightly --cron "0 3 * * *" --function billing.weeklyReport`},
			ex{"Every 5 minutes:", "polyapi job create --name heartbeat --interval 5 --function billing.ping"},
			ex{"Once at a time:", `polyapi job create --name cutover --on-time 2026-10-01T00:00:00Z --function billing.cutover`},
			ex{"Manual only (no schedule):", "polyapi job create --name adhoc --function billing.weeklyReport"},
		),
	}
	addJobWriteFlags(create, true)
	_ = create.MarkFlagRequired("name")
	create.MarkFlagsOneRequired("function", "functions")

	update := &cobra.Command{
		Use:   "update <id-or-name>",
		Short: "Update a job by ID or name",
		Args:  cobra.ExactArgs(1),
		RunE:  runJobUpdate,
		Example: examples(
			ex{"Change the cron:", `polyapi job update nightly --cron "0 4 * * *"`},
			ex{"Replace the function list:", "polyapi job update nightly --function billing.weeklyReport --function billing.notify"},
		),
	}
	addJobWriteFlags(update, false)

	del := &cobra.Command{
		Use:   "delete <id-or-name>",
		Short: "Delete a job by ID or name",
		Args:  cobra.ExactArgs(1),
		RunE:  runJobDelete,
		Example: examples(
			ex{"Delete by name:", "polyapi job delete nightly"},
		),
	}

	enable := &cobra.Command{
		Use:   "enable <id-or-name>",
		Short: "Enable a job",
		Args:  cobra.ExactArgs(1),
		RunE:  runJobEnable,
		Example: examples(
			ex{"Enable:", "polyapi job enable nightly"},
		),
	}

	disable := &cobra.Command{
		Use:   "disable <id-or-name>",
		Short: "Disable a job",
		Args:  cobra.ExactArgs(1),
		RunE:  runJobDisable,
		Example: examples(
			ex{"Disable:", "polyapi job disable nightly"},
		),
	}

	runCmd := &cobra.Command{
		Use:   "run <id-or-name>",
		Short: "Queue a job to run now (does not change its schedule)",
		Args:  cobra.ExactArgs(1),
		RunE:  runJobRun,
		Example: examples(
			ex{"Run now:", "polyapi job run nightly"},
		),
	}

	initCmd := newResourceInitCommand(resourceInitKind{
		Use: "job", Noun: "job", Type: glide.TypeJob, NeedsContext: false,
	})

	job.AddCommand(list, get, initCmd, create, update, del, enable, disable, runCmd)
	addJobExecutionCommands(job)
	root.AddCommand(job)
}

func addJobWriteFlags(cmd *cobra.Command, creating bool) {
	if creating {
		cmd.Flags().String("name", "", "Job name")
	} else {
		cmd.Flags().String("name", "", "New job name")
	}
	cmd.Flags().StringArray("function", nil, "Server function (`id`, `context.name`, or JSON `{id,eventPayload,headersPayload,paramsPayload}`). Repeatable")
	cmd.Flags().String("functions", "", "JSON array of function entries (alternative to --function)")
	execDef := ""
	execHelp := "Execution type: sequential or parallel"
	if creating {
		execDef = "sequential"
		execHelp += " (default sequential)"
	}
	cmd.Flags().String("execution-type", execDef, execHelp)
	cmd.Flags().Bool("enabled", true, "Whether the job is enabled")
	cmd.Flags().String("cron", "", "Periodical crontab (Zulu). Example: `0 3 * * *`")
	cmd.Flags().Uint("interval", 0, "Interval schedule in minutes")
	cmd.Flags().String("on-time", "", "One-shot ISO 8601 datetime (Zulu)")
	cmd.Flags().String("schedule", "", "JSON schedule object (`type` + `value`, optional `acceptableDelayTime`)")
	cmd.Flags().Uint("delay-minutes", 0, "Acceptable delay in minutes before a late run is skipped")
	cmd.MarkFlagsMutuallyExclusive("cron", "interval", "on-time", "schedule")
}

func runJobList(cmd *cobra.Command, _ []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	items, err := client.ListAll(jobCollection)
	if err != nil {
		return fail(err)
	}
	var rows []jobRef
	for _, item := range items {
		rows = append(rows, jobFromItem(item))
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
			fmt.Fprintln(out, "no jobs found")
		}
		return nil
	}
	printJobList(out, rows)
	return nil
}

func printJobList(w io.Writer, rows []jobRef) {
	nameW, enW, schW := len("NAME"), len("ENABLED"), len("SCHEDULE")
	schedules := make([]string, len(rows))
	for i, row := range rows {
		schedules[i] = formatJobSchedule(row.Schedule)
		if n := len(row.Name); n > nameW {
			nameW = n
		}
		if n := len(strconv.FormatBool(row.Enabled)); n > enW {
			enW = n
		}
		if n := len(schedules[i]); n > schW {
			schW = n
		}
	}
	fmt.Fprintln(w, infoText(fmt.Sprintf("%-*s  %-*s  %-*s  %s", nameW, "NAME", enW, "ENABLED", schW, "SCHEDULE", "ID")))
	for i, row := range rows {
		name := row.Name
		if name == "" {
			name = "-"
		}
		fmt.Fprintf(w, "%-*s  %-*s  %-*s  %s\n", nameW, name, enW, strconv.FormatBool(row.Enabled), schW, schedules[i], row.ID)
	}
}

func runJobGet(cmd *cobra.Command, args []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveJob(client, args[0])
	if err != nil {
		return err
	}
	return writeJSON(cmd.OutOrStdout(), ref.Spec)
}

func runJobCreate(cmd *cobra.Command, _ []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	payload, err := jobWritePayload(cmd, client, true)
	if err != nil {
		return err
	}
	created, err := client.Create(jobCollection, payload)
	if err != nil {
		return fail(err)
	}
	id := mapString(created, "id")
	name := firstNonEmpty(mapString(created, "name"), mapString(payload, "name"), id)
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("created job %s (%s)", name, id))
	}
	return nil
}

func runJobUpdate(cmd *cobra.Command, args []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveJob(client, args[0])
	if err != nil {
		return err
	}
	payload, err := jobWritePayload(cmd, client, false)
	if err != nil {
		return err
	}
	if len(payload) == 0 {
		return failUsage("pass at least one field to update")
	}
	updated, err := client.Update(jobCollection, ref.ID, payload, nil)
	if err != nil {
		return fail(err)
	}
	id := firstNonEmpty(mapString(updated, "id"), ref.ID)
	name := firstNonEmpty(mapString(updated, "name"), mapString(payload, "name"), ref.Name)
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("updated job %s (%s)", name, id))
	}
	return nil
}

func runJobDelete(cmd *cobra.Command, args []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveJob(client, args[0])
	if err != nil {
		return err
	}
	if err := client.Delete(jobCollection, ref.ID, nil); err != nil {
		return fail(err)
	}
	name := ref.Name
	if name == "" {
		name = ref.ID
	}
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("deleted job %s (%s)", name, ref.ID))
	}
	return nil
}

func runJobEnable(cmd *cobra.Command, args []string) error {
	return setJobEnabled(cmd, args[0], true)
}

func runJobDisable(cmd *cobra.Command, args []string) error {
	return setJobEnabled(cmd, args[0], false)
}

func setJobEnabled(cmd *cobra.Command, idOrName string, enabled bool) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveJob(client, idOrName)
	if err != nil {
		return err
	}
	updated, err := client.Update(jobCollection, ref.ID, map[string]any{"enabled": enabled}, nil)
	if err != nil {
		return fail(err)
	}
	id := firstNonEmpty(mapString(updated, "id"), ref.ID)
	name := firstNonEmpty(mapString(updated, "name"), ref.Name, id)
	verb := "enabled"
	if !enabled {
		verb = "disabled"
	}
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("%s job %s (%s)", verb, name, id))
	}
	return nil
}

func runJobRun(cmd *cobra.Command, args []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveJob(client, args[0])
	if err != nil {
		return err
	}
	result, err := client.PostAction(jobCollection, ref.ID, "trigger", nil)
	if err != nil {
		return fail(err)
	}
	if result == nil {
		if !globalsFrom(cmd).Quiet {
			printOk(cmd.OutOrStdout(), fmt.Sprintf("queued job %s (%s)", firstNonEmpty(ref.Name, ref.ID), ref.ID))
		}
		return nil
	}
	return writeJSON(cmd.OutOrStdout(), result)
}

func jobWritePayload(cmd *cobra.Command, client *api.HTTPClient, creating bool) (map[string]any, error) {
	payload := map[string]any{}
	if creating || cmd.Flags().Changed("name") {
		name, _ := cmd.Flags().GetString("name")
		name = strings.TrimSpace(name)
		if creating && name == "" {
			return nil, failUsage("name is required")
		}
		if name != "" {
			payload["name"] = name
		}
	}
	fns, err := readJobFunctions(cmd, client)
	if err != nil {
		return nil, err
	}
	if fns != nil {
		if creating && len(fns) == 0 {
			return nil, failUsage("functions must contain at least one server function")
		}
		payload["functions"] = fns
	} else if creating {
		return nil, failUsage("pass --function or --functions")
	}
	if creating || cmd.Flags().Changed("execution-type") {
		raw, _ := cmd.Flags().GetString("execution-type")
		et, err := parseJobExecutionType(raw)
		if err != nil {
			return nil, err
		}
		payload["executionType"] = et
	}
	if creating || cmd.Flags().Changed("enabled") {
		payload["enabled"] = boolFlagOr(cmd, "enabled", true)
	}
	schedule, err := readJobSchedule(cmd, creating)
	if err != nil {
		return nil, err
	}
	if schedule != nil {
		payload["schedule"] = schedule
	}
	return payload, nil
}

func readJobFunctions(cmd *cobra.Command, client *api.HTTPClient) ([]any, error) {
	set := cmd.Flags().Changed("function")
	jsonSet := cmd.Flags().Changed("functions")
	if set && jsonSet {
		return nil, failUsage("pass only one of --function or --functions")
	}
	if !set && !jsonSet {
		return nil, nil
	}
	var entries []any
	if jsonSet {
		raw, _ := cmd.Flags().GetString("functions")
		var v any
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, failUsage("invalid --functions JSON: " + err.Error())
		}
		switch t := v.(type) {
		case []any:
			entries = t
		case map[string]any:
			entries = []any{t}
		default:
			return nil, failUsage("option `functions` must be a JSON array")
		}
	} else {
		raw, _ := cmd.Flags().GetStringArray("function")
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
			return nil, failUsage(fmt.Sprintf("functions[%d] must be an object", i))
		}
		id := strings.TrimSpace(firstNonEmpty(mapString(m, "id"), mapString(m, "functionId")))
		if id == "" {
			ctx, name := strings.TrimSpace(mapString(m, "functionContext")), strings.TrimSpace(mapString(m, "functionName"))
			if ctx != "" && name != "" {
				id = ctx + "." + name
			}
		}
		if id == "" {
			return nil, failUsage(fmt.Sprintf("functions[%d] needs id, or functionContext plus functionName", i))
		}
		resolved, err := resolveServerFunctionID(client, id)
		if err != nil {
			return nil, err
		}
		entry := map[string]any{"id": resolved}
		for _, key := range []string{"eventPayload", "headersPayload", "paramsPayload"} {
			if v, ok := m[key]; ok {
				entry[key] = v
			}
		}
		out = append(out, entry)
	}
	return out, nil
}

func readJobSchedule(cmd *cobra.Command, creating bool) (map[string]any, error) {
	cronSet := cmd.Flags().Changed("cron")
	intervalSet := cmd.Flags().Changed("interval")
	onTimeSet := cmd.Flags().Changed("on-time")
	jsonSet := cmd.Flags().Changed("schedule")
	delaySet := cmd.Flags().Changed("delay-minutes")
	if !cronSet && !intervalSet && !onTimeSet && !jsonSet {
		if delaySet && !creating {
			return nil, failUsage("pass --cron, --interval, --on-time, or --schedule with --delay-minutes")
		}
		return nil, nil
	}
	var schedule map[string]any
	switch {
	case jsonSet:
		raw, _ := cmd.Flags().GetString("schedule")
		obj, err := parseJSONObject([]byte(raw), "schedule")
		if err != nil {
			return nil, err
		}
		typ := strings.ToLower(strings.TrimSpace(mapString(obj, "type")))
		switch typ {
		case "periodical", "interval", "on_time":
			obj["type"] = typ
		case "":
			return nil, failUsage("option `schedule` needs type periodical, interval, or on_time")
		default:
			return nil, failUsage("option `schedule` type must be periodical, interval, or on_time")
		}
		if _, ok := obj["value"]; !ok {
			return nil, failUsage("option `schedule` needs a value")
		}
		schedule = obj
	case cronSet:
		cron, _ := cmd.Flags().GetString("cron")
		cron = strings.TrimSpace(cron)
		if cron == "" {
			return nil, failUsage("option `cron` must be a crontab expression")
		}
		schedule = map[string]any{"type": "periodical", "value": cron}
	case intervalSet:
		n, _ := cmd.Flags().GetUint("interval")
		if n == 0 {
			return nil, failUsage("option `interval` must be at least 1 minute")
		}
		schedule = map[string]any{"type": "interval", "value": n}
	case onTimeSet:
		when, _ := cmd.Flags().GetString("on-time")
		when = strings.TrimSpace(when)
		if when == "" {
			return nil, failUsage("option `on-time` must be an ISO 8601 datetime")
		}
		schedule = map[string]any{"type": "on_time", "value": when}
	}
	if delaySet {
		n, _ := cmd.Flags().GetUint("delay-minutes")
		schedule["acceptableDelayTime"] = n
	}
	return schedule, nil
}

func parseJobExecutionType(s string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "sequential":
		return "sequential", nil
	case "parallel":
		return "parallel", nil
	case "":
		return "", failUsage("option `execution-type` must be sequential or parallel")
	default:
		return "", failUsage("option `execution-type` must be sequential or parallel")
	}
}

func resolveJob(client *api.HTTPClient, idOrName string) (jobRef, error) {
	if looksLikeUUID(idOrName) {
		return lookupJobByID(client, idOrName)
	}
	return lookupJobByName(client, idOrName)
}

func lookupJobByID(client *api.HTTPClient, id string) (jobRef, error) {
	spec, err := client.Get(jobCollection, id)
	if err != nil {
		return jobRef{}, fail(err)
	}
	ref := jobFromItem(spec)
	if ref.ID == "" {
		ref.ID = id
	}
	ref.Spec = spec
	return ref, nil
}

func lookupJobByName(client *api.HTTPClient, name string) (jobRef, error) {
	items, err := client.ListAll(jobCollection)
	if err != nil {
		return jobRef{}, fail(err)
	}
	var same, anyCase []jobRef
	for _, item := range items {
		ref := jobFromItem(item)
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
		return jobRef{}, failUsage(fmt.Sprintf("no job named %q", name))
	case 1:
		ref := matches[0]
		if ref.ID == "" {
			return ref, nil
		}
		spec, err := client.Get(jobCollection, ref.ID)
		if err != nil {
			return jobRef{}, fail(err)
		}
		full := jobFromItem(spec)
		full.ID = ref.ID
		full.Spec = spec
		return full, nil
	default:
		return jobRef{}, failUsage(fmt.Sprintf("unclear job reference %q; matches %d jobs", name, len(matches)))
	}
}

func jobFromItem(item any) jobRef {
	m, _ := item.(map[string]any)
	return jobRef{
		ID:            firstMapString(m, "id"),
		Name:          mapString(m, "name"),
		Enabled:       boolMapField(m, "enabled"),
		ExecutionType: mapString(m, "executionType"),
		Schedule:      m["schedule"],
		Spec:          item,
	}
}

func boolMapField(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	switch v := m[key].(type) {
	case bool:
		return v
	case string:
		b, err := strconv.ParseBool(v)
		return err == nil && b
	default:
		return false
	}
}

func formatJobSchedule(v any) string {
	if v == nil {
		return "-"
	}
	if s, ok := v.(string); ok {
		if strings.TrimSpace(s) == "" {
			return "-"
		}
		return s
	}
	m, ok := v.(map[string]any)
	if !ok {
		return "-"
	}
	typ := strings.ToLower(mapString(m, "type"))
	switch typ {
	case "periodical":
		s := strings.TrimSpace(fmt.Sprint(m["value"]))
		if s == "" || s == "<nil>" {
			return "-"
		}
		return s
	case "interval":
		n := formatJobNumber(m["value"])
		if n == "" {
			return "-"
		}
		return "every " + n + " min"
	case "on_time":
		s := strings.TrimSpace(fmt.Sprint(m["value"]))
		if s == "" || s == "<nil>" {
			return "-"
		}
		return s
	default:
		return "-"
	}
}

func formatJobNumber(v any) string {
	switch n := v.(type) {
	case nil:
		return ""
	case int:
		return strconv.Itoa(n)
	case int64:
		return strconv.FormatInt(n, 10)
	case uint:
		return strconv.FormatUint(uint64(n), 10)
	case float64:
		if n == float64(int64(n)) {
			return strconv.FormatInt(int64(n), 10)
		}
		return strconv.FormatFloat(n, 'f', -1, 64)
	case json.Number:
		return n.String()
	case string:
		return strings.TrimSpace(n)
	default:
		s := strings.TrimSpace(fmt.Sprint(v))
		if s == "" || s == "<nil>" {
			return ""
		}
		return s
	}
}
