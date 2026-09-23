package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/polyapi/polyglot/src/api"
	"github.com/polyapi/polyglot/src/config"
	"github.com/polyapi/polyglot/src/delegate"
	"github.com/polyapi/polyglot/src/exitcode"
	"github.com/polyapi/polyglot/src/version"
	"github.com/spf13/cobra"
)

type doctorCheck struct {
	Name   string
	Status string // ok, fail, warn, skip
	Detail string
	Code   int
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	g := globalsFrom(cmd)
	offline, _ := cmd.Flags().GetBool("offline")
	checks := collectDoctorChecks(g, offline)
	printDoctorReport(cmd.OutOrStdout(), checks)
	if fail := firstDoctorFail(checks); fail != nil {
		return &ExitError{Code: fail.Code, Msg: fail.Detail}
	}
	return nil
}

func collectDoctorChecks(g Globals, offline bool) []doctorCheck {
	info := version.Current()
	checks := []doctorCheck{
		{
			Name:   "binary",
			Status: "ok",
			Detail: fmt.Sprintf("polyapi %s  %s  %s  commit %s", info.Version, info.Platform(), info.Go, info.CommitDisplay()),
		},
		{
			Name:   "protocol",
			Status: "ok",
			Detail: fmt.Sprintf("%d", delegate.Protocol),
		},
	}

	cfg, cfgErr := loadDelegateConfig(g)
	checks = append(checks, doctorConfigCheck(cfg, cfgErr))
	checks = append(checks, doctorAdapterCheck(g, cfg, cfgErr == nil))
	checks = append(checks, doctorAuthCheck(cfg, cfgErr, offline, g.Verbose))
	checks = append(checks, doctorValidateCheck(g, cfg, cfgErr == nil))
	root := projectRoot()
	if cfgErr == nil && cfg.ProjectRoot != "" {
		root = cfg.ProjectRoot
	}
	var deploy *config.DeployFile
	if cfgErr == nil {
		deploy = cfg.File.Deploy
	}
	eval := config.EvaluateDeploy(root, deploy, g.Env)
	checks = append(checks, doctorCheck{
		Name:   "deploy",
		Status: eval.Status,
		Detail: eval.Message,
		Code:   deployExit(eval.Status),
	})
	return checks
}

func doctorConfigCheck(cfg config.Resolved, err error) doctorCheck {
	c := doctorCheck{Name: "config", Code: exitcode.Auth}
	if err != nil {
		c.Status = "fail"
		c.Detail = err.Error()
		type coder interface{ ExitCode() int }
		if x, ok := err.(coder); ok {
			c.Code = x.ExitCode()
		}
		return c
	}
	if cfg.BaseURL == "" || cfg.APIKey == "" {
		c.Status = "fail"
		c.Detail = "no API key or base URL configured; run `polyapi auth login` or set POLY_API_KEY and POLY_API_BASE_URL"
		return c
	}
	inst := cfg.Instance
	if inst == "" {
		inst = cfg.BaseURL
	}
	c.Status = "ok"
	c.Detail = fmt.Sprintf("%s  %s  key=%s  (%s)", inst, cfg.BaseURL, cfg.RedactedKey(), cfg.KeySource.String())
	return c
}

func doctorAdapterCheck(g Globals, cfg config.Resolved, haveCfg bool) doctorCheck {
	c := doctorCheck{Name: "adapter", Code: exitcode.AdapterMissing}
	if !haveCfg {
		c.Status = "skip"
		c.Detail = "skipped (config failed)"
		c.Code = 0
		return c
	}
	start := cfg.ProjectRoot
	if start == "" {
		start = projectRoot()
	}
	loc := delegate.LocateProject(start, g.PolyPath)
	if !loc.Found && g.Lang == "" && g.Adapter == "" && cfg.Language == "" && cfg.AdapterCommand == "" {
		c.Status = "skip"
		c.Detail = "no language project detected"
		c.Code = 0
		return c
	}
	// Doctor never prompts for a language.
	g.NonInteractive = true
	adapter, err := resolveAdapter(g, cfg)
	if err != nil {
		return adapterCheckFromErr(err)
	}
	del := delegate.New(adapter.Root, adapter.Lang, adapter.Argv, g.PolyPath)
	caps, err := del.Capabilities()
	if err != nil {
		return adapterCheckFromErr(err)
	}
	c.Status = "ok"
	c.Code = 0
	c.Detail = fmt.Sprintf("%s  %s  sdk=%s  protocol=%d", adapter.Lang, strings.Join(adapter.Argv, " "), caps.SDKVersion, caps.Protocol)
	if g.Verbose > 0 && len(caps.Ops) > 0 {
		ops := make([]string, len(caps.Ops))
		for i, op := range caps.Ops {
			ops[i] = string(op)
		}
		c.Detail += "  ops=" + strings.Join(ops, ",")
	}
	return c
}

func adapterCheckFromErr(err error) doctorCheck {
	c := doctorCheck{Name: "adapter", Status: "fail", Detail: err.Error(), Code: exitcode.AdapterMissing}
	if de, ok := err.(*delegate.Error); ok {
		c.Code = de.ExitCode()
		switch de.Code {
		case exitcode.AdapterMissing, exitcode.Protocol:
			c.Status = "fail"
		case exitcode.Usage:
			c.Status = "warn"
			c.Code = 0
		default:
			c.Status = "fail"
		}
	}
	return c
}

func doctorAuthCheck(cfg config.Resolved, cfgErr error, offline bool, verbose int) doctorCheck {
	c := doctorCheck{Name: "auth", Code: exitcode.Auth}
	if cfgErr != nil || cfg.BaseURL == "" || cfg.APIKey == "" {
		c.Status = "skip"
		c.Detail = "skipped (no credentials)"
		c.Code = 0
		return c
	}
	if offline {
		c.Status = "skip"
		c.Detail = "skipped (--offline)"
		c.Code = 0
		return c
	}
	client, err := api.FromConfig(cfg)
	if err != nil {
		c.Status = "fail"
		c.Detail = err.Error()
		type coder interface{ ExitCode() int }
		if x, ok := err.(coder); ok {
			c.Code = x.ExitCode()
		}
		return c
	}
	auth, err := client.Auth()
	if err != nil {
		c.Status = "fail"
		c.Detail = err.Error()
		type coder interface{ ExitCode() int }
		if x, ok := err.(coder); ok {
			c.Code = x.ExitCode()
		} else {
			c.Code = exitcode.Failure
		}
		if ae, ok := err.(*api.Error); ok && ae.Kind == "network" {
			c.Code = exitcode.Network
		}
		return c
	}
	var parts []string
	if auth.Tenant != nil && auth.Tenant.ID != "" {
		parts = append(parts, "tenant="+auth.Tenant.ID)
	}
	if auth.Environment != nil && auth.Environment.ID != "" {
		parts = append(parts, "environment="+auth.Environment.ID)
	}
	if len(parts) == 0 {
		parts = append(parts, "authenticated")
	}
	if verbose > 0 {
		var granted []string
		for name, ok := range auth.Permissions {
			if ok {
				granted = append(granted, name)
			}
		}
		sort.Strings(granted)
		if len(granted) > 0 {
			parts = append(parts, "permissions="+strings.Join(granted, ","))
		}
	}
	c.Status = "ok"
	c.Code = 0
	c.Detail = strings.Join(parts, "  ")
	return c
}

func deployExit(status string) int {
	if status == "fail" {
		return exitcode.Failure
	}
	return 0
}

func firstDoctorFail(checks []doctorCheck) *doctorCheck {
	for i := range checks {
		if checks[i].Status == "fail" {
			c := checks[i]
			if c.Code == 0 {
				c.Code = exitcode.Failure
			}
			return &c
		}
	}
	return nil
}

func printDoctorReport(w io.Writer, checks []doctorCheck) {
	nameWidth := 0
	for _, c := range checks {
		if len(c.Name) > nameWidth {
			nameWidth = len(c.Name)
		}
	}
	for _, c := range checks {
		fmt.Fprintf(w, "%s  %-*s  %s\n", doctorStatusLabel(c.Status), nameWidth, c.Name, c.Detail)
	}
}

func doctorStatusLabel(status string) string {
	padded := fmt.Sprintf("%-4s", status)
	switch status {
	case "ok":
		return okText(padded)
	case "fail":
		return failText(padded)
	case "warn":
		return warnText(padded)
	default:
		return infoText(padded)
	}
}
