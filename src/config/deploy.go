package config

import (
	"fmt"
	"path"
	"strings"
)

// ReceiptsEnabled is true when [deploy] receipts = true.
func (d *DeployFile) ReceiptsEnabled() bool {
	return d != nil && d.Receipts != nil && *d.Receipts
}

// WithEnvironment copies r and, when name matches [environments.*], uses that base URL.
func (r Resolved) WithEnvironment(name string) (Resolved, error) {
	name = strings.TrimSpace(name)
	if name == "" || r.File.Environments == nil {
		return r, nil
	}
	env, ok := r.File.Environments[name]
	if !ok || strings.TrimSpace(env.BaseURL) == "" {
		return r, nil
	}
	url, err := ResolveBaseURL(env.BaseURL)
	if err != nil {
		return r, err
	}
	r.BaseURL = url
	r.URLSource = SourceProject
	if inst := InstanceName(url); inst != "" {
		r.Instance = inst
	}
	return r, nil
}

// DeployEval is whether the current git branch may push, and to which env.
type DeployEval struct {
	Branch      string
	GitOK       bool
	Environment string
	Allowed     bool
	Production  bool
	Status      string // ok, warn, fail, skip
	Message     string
}

// EvaluateDeploy matches the current git branch against [deploy] policy.
//
// selectedEnv is the global --env flag (may be empty). Missing git or missing
// deploy.targets is skip, not fail, unless deploy.require_git is true.
func EvaluateDeploy(projectRoot string, deploy *DeployFile, selectedEnv string) DeployEval {
	branch, gitOK := CurrentBranch(projectRoot)
	eval := DeployEval{Branch: branch, GitOK: gitOK}

	if deploy != nil && deploy.RequireGit != nil && *deploy.RequireGit && !gitOK {
		eval.Status = "fail"
		eval.Message = "not a git repository (deploy.require_git = true)"
		return eval
	}
	if !gitOK {
		eval.Status = "skip"
		eval.Message = "not a git repository"
		return eval
	}

	if deploy == nil || len(deploy.Targets) == 0 {
		eval.Status = "skip"
		msg := "branch " + branch + "; no deploy.targets configured"
		if selectedEnv != "" {
			msg += " (--env " + selectedEnv + ")"
		}
		eval.Message = msg
		return eval
	}

	target, ok := MatchDeployTarget(branch, deploy.Targets)
	if !ok {
		eval.Allowed = false
		policy := strings.ToLower(strings.TrimSpace(deploy.UnallowedBranch))
		msg := fmt.Sprintf("branch %s is not in deploy.targets", branch)
		switch policy {
		case "error", "fail", "block":
			eval.Status = "fail"
			eval.Message = msg + " (unallowed_branch = error; push blocked)"
		default:
			eval.Status = "warn"
			eval.Message = msg + " (push not allowed)"
		}
		return eval
	}

	eval.Allowed = true
	eval.Environment = target.Environment
	if target.Production != nil {
		eval.Production = *target.Production
	}
	detail := fmt.Sprintf("branch %s → %s (push allowed)", branch, target.Environment)
	if eval.Production {
		detail = fmt.Sprintf("branch %s → %s (production, push allowed)", branch, target.Environment)
	}
	if selectedEnv != "" && selectedEnv != target.Environment {
		eval.Status = "warn"
		eval.Message = detail + "; --env is " + selectedEnv
		return eval
	}
	eval.Status = "ok"
	eval.Message = detail
	return eval
}

// MatchDeployTarget returns the first target whose branch list matches.
func MatchDeployTarget(branch string, targets []DeployTargetFile) (DeployTargetFile, bool) {
	for _, t := range targets {
		if branchMatches(branch, t.Branches) {
			return t, true
		}
	}
	return DeployTargetFile{}, false
}

func branchMatches(branch string, patterns []string) bool {
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == branch || p == "*" {
			return true
		}
		if ok, err := path.Match(p, branch); err == nil && ok {
			return true
		}
	}
	return false
}
