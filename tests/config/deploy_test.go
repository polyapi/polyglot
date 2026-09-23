package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/polyapi/polyglot/src/config"
)

func TestCurrentBranchFromHEAD(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok := config.CurrentBranch(dir)
	if !ok || got != "main" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
	nested := filepath.Join(dir, "src", "app")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	got, ok = config.CurrentBranch(nested)
	if !ok || got != "main" {
		t.Fatalf("nested %q ok=%v", got, ok)
	}
}

func TestCurrentBranchDetached(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("abcdef1234567890\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok := config.CurrentBranch(dir)
	if !ok || got != "detached@abcdef1" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
}

func TestCurrentBranchMissing(t *testing.T) {
	if _, ok := config.CurrentBranch(t.TempDir()); ok {
		t.Fatal("expected no git repo")
	}
}

func TestEvaluateDeployAllowed(t *testing.T) {
	dir := t.TempDir()
	writeGit(t, dir, "main")
	prod := true
	eval := config.EvaluateDeploy(dir, &config.DeployFile{
		UnallowedBranch: "error",
		Targets: []config.DeployTargetFile{{
			Branches:    []string{"main"},
			Environment: "prod",
			Production:  &prod,
		}},
	}, "")
	if eval.Status != "ok" || !eval.Allowed || eval.Environment != "prod" || !eval.Production {
		t.Fatalf("%+v", eval)
	}
}

func TestEvaluateDeployBlocked(t *testing.T) {
	dir := t.TempDir()
	writeGit(t, dir, "feature/x")
	eval := config.EvaluateDeploy(dir, &config.DeployFile{
		UnallowedBranch: "error",
		Targets: []config.DeployTargetFile{{
			Branches:    []string{"main"},
			Environment: "prod",
		}},
	}, "")
	if eval.Status != "fail" || eval.Allowed {
		t.Fatalf("%+v", eval)
	}
}

func TestEvaluateDeployWarnWhenUnallowedIsWarn(t *testing.T) {
	dir := t.TempDir()
	writeGit(t, dir, "feature/x")
	eval := config.EvaluateDeploy(dir, &config.DeployFile{
		UnallowedBranch: "warn",
		Targets: []config.DeployTargetFile{{
			Branches:    []string{"main"},
			Environment: "prod",
		}},
	}, "")
	if eval.Status != "warn" || eval.Allowed {
		t.Fatalf("%+v", eval)
	}
}

func TestEvaluateDeployGlob(t *testing.T) {
	dir := t.TempDir()
	writeGit(t, dir, "release/1.2")
	eval := config.EvaluateDeploy(dir, &config.DeployFile{
		Targets: []config.DeployTargetFile{{
			Branches:    []string{"release/*"},
			Environment: "staging",
		}},
	}, "")
	if eval.Status != "ok" || eval.Environment != "staging" {
		t.Fatalf("%+v", eval)
	}
}

func TestEvaluateDeployEnvMismatchWarns(t *testing.T) {
	dir := t.TempDir()
	writeGit(t, dir, "main")
	eval := config.EvaluateDeploy(dir, &config.DeployFile{
		Targets: []config.DeployTargetFile{{
			Branches:    []string{"main"},
			Environment: "prod",
		}},
	}, "staging")
	if eval.Status != "warn" || !eval.Allowed || eval.Environment != "prod" {
		t.Fatalf("%+v", eval)
	}
}

func TestEvaluateDeploySkipWithoutGit(t *testing.T) {
	eval := config.EvaluateDeploy(t.TempDir(), &config.DeployFile{
		Targets: []config.DeployTargetFile{{Branches: []string{"main"}, Environment: "prod"}},
	}, "")
	if eval.Status != "skip" {
		t.Fatalf("%+v", eval)
	}
}

func TestEvaluateDeployRequireGitFails(t *testing.T) {
	req := true
	eval := config.EvaluateDeploy(t.TempDir(), &config.DeployFile{
		RequireGit: &req,
		Targets:    []config.DeployTargetFile{{Branches: []string{"main"}, Environment: "prod"}},
	}, "")
	if eval.Status != "fail" {
		t.Fatalf("%+v", eval)
	}
}

func TestMatchDeployTarget(t *testing.T) {
	targets := []config.DeployTargetFile{
		{Branches: []string{"main"}, Environment: "prod"},
		{Branches: []string{"develop"}, Environment: "dev"},
	}
	got, ok := config.MatchDeployTarget("develop", targets)
	if !ok || got.Environment != "dev" {
		t.Fatalf("%+v %v", got, ok)
	}
	if _, ok := config.MatchDeployTarget("other", targets); ok {
		t.Fatal("expected no match")
	}
}

func writeGit(t *testing.T, dir, branch string) {
	t.Helper()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/"+branch+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
