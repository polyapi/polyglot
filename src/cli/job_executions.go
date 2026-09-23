package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

func addJobExecutionCommands(job *cobra.Command) {
	execs := &cobra.Command{
		Use:   "executions",
		Short: "Inspect job executions",
		Example: examples(
			ex{"List recent runs:", "polyapi job executions list nightly"},
			ex{"Get one run:", "polyapi job executions get nightly abc123"},
		),
	}

	list := &cobra.Command{
		Use:   "list <id-or-name>",
		Short: "List executions for a job",
		Args:  cobra.ExactArgs(1),
		RunE:  runJobExecutionsList,
		Example: examples(
			ex{"List executions:", "polyapi job executions list nightly"},
			ex{"Errors in the last day:", "polyapi job executions list nightly --status job_error --last-days 1"},
		),
	}
	list.Flags().String("status", "", "Filter: finished, job_error, with_call_error, max_execution_time_reached, scheduling_error")
	list.Flags().Uint("last-hours", 0, "Look back this many hours")
	list.Flags().Uint("last-days", 0, "Look back this many days")
	list.Flags().Uint("limit", 0, "Maximum executions to return")

	get := &cobra.Command{
		Use:   "get <id-or-name> <execution-id>",
		Short: "Get one execution",
		Args:  cobra.ExactArgs(2),
		RunE:  runJobExecutionGet,
		Example: examples(
			ex{"Get an execution:", "polyapi job executions get nightly abc123"},
		),
	}

	del := &cobra.Command{
		Use:   "delete <id-or-name> [execution-id]",
		Short: "Delete one execution, or all executions with --all",
		Args:  cobra.RangeArgs(1, 2),
		RunE:  runJobExecutionDelete,
		Example: examples(
			ex{"Delete one:", "polyapi job executions delete nightly abc123"},
			ex{"Delete all:", "polyapi job executions delete nightly --all"},
		),
	}
	del.Flags().Bool("all", false, "Delete every execution for this job")

	execs.AddCommand(list, get, del)
	job.AddCommand(execs)
}

func runJobExecutionsList(cmd *cobra.Command, args []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveJob(client, args[0])
	if err != nil {
		return err
	}
	query, err := jobExecutionQuery(cmd)
	if err != nil {
		return err
	}
	items, err := client.ListAllQuery(jobCollection+"/"+ref.ID+"/executions", query)
	if err != nil {
		return fail(err)
	}
	out := cmd.OutOrStdout()
	if len(items) == 0 {
		if !globalsFrom(cmd).Quiet {
			fmt.Fprintln(out, "no executions found")
		}
		return nil
	}
	printJobExecutionList(out, items)
	return nil
}

func printJobExecutionList(w io.Writer, items []any) {
	type row struct {
		ID        string
		Status    string
		Duration  string
		Processed string
	}
	rows := make([]row, 0, len(items))
	idW, stW, durW := len("ID"), len("STATUS"), len("DURATION")
	for _, item := range items {
		m, _ := item.(map[string]any)
		r := row{
			ID:        firstMapString(m, "id"),
			Status:    firstNonEmpty(mapString(m, "status"), "-"),
			Duration:  formatJobNumber(m["duration"]),
			Processed: firstNonEmpty(fmtJobTime(m["processedOn"]), "-"),
		}
		if r.Duration == "" {
			r.Duration = "-"
		}
		if n := len(r.ID); n > idW {
			idW = n
		}
		if n := len(r.Status); n > stW {
			stW = n
		}
		if n := len(r.Duration); n > durW {
			durW = n
		}
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Processed != rows[j].Processed {
			return rows[i].Processed > rows[j].Processed
		}
		return rows[i].ID < rows[j].ID
	})
	fmt.Fprintln(w, infoText(fmt.Sprintf("%-*s  %-*s  %-*s  %s", idW, "ID", stW, "STATUS", durW, "DURATION", "PROCESSED")))
	for _, r := range rows {
		fmt.Fprintf(w, "%-*s  %-*s  %-*s  %s\n", idW, r.ID, stW, r.Status, durW, r.Duration, r.Processed)
	}
}

func runJobExecutionGet(cmd *cobra.Command, args []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveJob(client, args[0])
	if err != nil {
		return err
	}
	spec, err := client.Get(jobCollection+"/"+ref.ID+"/executions", args[1])
	if err != nil {
		return fail(err)
	}
	return writeJSON(cmd.OutOrStdout(), spec)
}

func runJobExecutionDelete(cmd *cobra.Command, args []string) error {
	all, _ := cmd.Flags().GetBool("all")
	if len(args) == 2 && all {
		return failUsage("pass an execution id or --all, not both")
	}
	if len(args) == 1 && !all {
		return failUsage("pass an execution id or --all")
	}
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveJob(client, args[0])
	if err != nil {
		return err
	}
	if all {
		if err := client.DeleteAction(jobCollection, ref.ID, "executions"); err != nil {
			return fail(err)
		}
		if !globalsFrom(cmd).Quiet {
			printOk(cmd.OutOrStdout(), fmt.Sprintf("deleted executions for job %s (%s)", firstNonEmpty(ref.Name, ref.ID), ref.ID))
		}
		return nil
	}
	execID := args[1]
	if err := client.Delete(jobCollection+"/"+ref.ID+"/executions", execID, nil); err != nil {
		return fail(err)
	}
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("deleted execution %s", execID))
	}
	return nil
}

func jobExecutionQuery(cmd *cobra.Command) ([][2]string, error) {
	var q [][2]string
	if cmd.Flags().Changed("status") {
		status, _ := cmd.Flags().GetString("status")
		status = strings.ToLower(strings.TrimSpace(status))
		switch status {
		case "finished", "job_error", "with_call_error", "max_execution_time_reached", "scheduling_error":
			q = append(q, [2]string{"status", status})
		case "":
			return nil, failUsage("option `status` must be finished, job_error, with_call_error, max_execution_time_reached, or scheduling_error")
		default:
			return nil, failUsage("option `status` must be finished, job_error, with_call_error, max_execution_time_reached, or scheduling_error")
		}
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
	return q, nil
}

func fmtJobTime(v any) string {
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "" || s == "<nil>" {
		return ""
	}
	return s
}
