package git_test

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/co2-lab/polvo/internal/git"
)

// seedCommit creates a file, stages it, and commits it in the given repo.
func seedCommit(t *testing.T, dir, filename, content string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		allArgs := append([]string{"-C", dir}, args...)
		cmd := exec.Command("git", allArgs...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	writeFile(t, dir, filename, content)
	run("add", "--", filename)
	run("commit", "-m", "chore: seed")
}

// TestOrchestrator_AutoCommit verifies that PostRun commits modified files.
func TestOrchestrator_AutoCommit(t *testing.T) {
	dir := t.TempDir()
	c := initRepo(t, dir)
	ctx := context.Background()

	// Seed repo
	seedCommit(t, dir, "base.txt", "base")

	cfg := git.OrchestratorConfig{AutoCommit: true}
	orch := git.NewOrchestrator(cfg, c, nil)

	// Simulate a file change by the agent
	writeFile(t, dir, "new.txt", "agent output")

	if err := orch.PostRun(ctx, "writer", nil); err != nil {
		t.Fatalf("PostRun: %v", err)
	}

	// Repo should be clean after auto-commit
	clean, err := c.IsCleanWorkingTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !clean {
		t.Error("expected clean working tree after auto-commit")
	}
}

// TestOrchestrator_DirtyCommit verifies that PreRun commits pre-existing dirty files.
func TestOrchestrator_DirtyCommit(t *testing.T) {
	dir := t.TempDir()
	c := initRepo(t, dir)
	ctx := context.Background()

	// Seed repo with a commit
	seedCommit(t, dir, "base.txt", "base")

	// Dirty the repo
	writeFile(t, dir, "dirty.txt", "dirty content before agent run")

	cfg := git.OrchestratorConfig{DirtyCommit: true}
	orch := git.NewOrchestrator(cfg, c, nil)

	if err := orch.PreRun(ctx, "writer"); err != nil {
		t.Fatalf("PreRun: %v", err)
	}

	// Pre-existing dirty file should have been committed
	clean, err := c.IsCleanWorkingTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !clean {
		t.Error("expected clean tree after DirtyCommit pre-run")
	}
}

// TestOrchestrator_BranchPerRun verifies that PreRun creates a new branch.
func TestOrchestrator_BranchPerRun(t *testing.T) {
	dir := t.TempDir()
	c := initRepo(t, dir)
	ctx := context.Background()

	seedCommit(t, dir, "base.txt", "base")

	cfg := git.OrchestratorConfig{
		BranchPerRun:   true,
		BranchTemplate: "polvo/{{agent}}/{{timestamp}}",
	}
	orch := git.NewOrchestrator(cfg, c, nil)

	if err := orch.PreRun(ctx, "writer"); err != nil {
		t.Fatalf("PreRun: %v", err)
	}

	branch, err := c.CurrentBranch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if branch == "main" {
		t.Error("expected branch to have changed from main")
	}
	if len(branch) == 0 {
		t.Error("expected non-empty branch name")
	}
}

// TestRenderBranchTemplate verifies template substitution.
func TestRenderBranchTemplate(t *testing.T) {
	// renderBranchTemplate is unexported; test it via OrchestratorConfig behaviour.
	dir := t.TempDir()
	c := initRepo(t, dir)
	ctx := context.Background()

	seedCommit(t, dir, "base.txt", "base")

	cfg := git.OrchestratorConfig{
		BranchPerRun:   true,
		BranchTemplate: "ci/{{agent}}/run",
	}
	orch := git.NewOrchestrator(cfg, c, nil)
	if err := orch.PreRun(ctx, "my agent"); err != nil {
		t.Fatalf("PreRun: %v", err)
	}

	branch, err := c.CurrentBranch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Agent name "my agent" → "my-agent"; template → "ci/my-agent/run"
	if branch != "ci/my-agent/run" {
		t.Errorf("expected branch 'ci/my-agent/run', got %q", branch)
	}
}

// TestOrchestrator_NonFatalOnGitError verifies that errors from a bad repo don't propagate.
func TestOrchestrator_NonFatalOnGitError(t *testing.T) {
	// Use a non-git directory — all git operations should fail internally but not surface errors.
	dir := t.TempDir()
	c := &git.ExecClient{WorkDir: dir}
	ctx := context.Background()

	cfg := git.OrchestratorConfig{
		AutoCommit:   true,
		DirtyCommit:  true,
		BranchPerRun: true,
	}
	orch := git.NewOrchestrator(cfg, c, nil)

	// Both should return nil (non-fatal)
	if err := orch.PreRun(ctx, "agent"); err != nil {
		t.Errorf("PreRun should be non-fatal, got: %v", err)
	}
	if err := orch.PostRun(ctx, "agent", nil); err != nil {
		t.Errorf("PostRun should be non-fatal, got: %v", err)
	}
}

// Ensure time is imported (used indirectly via renderBranchTemplate logic).
var _ = time.Now
