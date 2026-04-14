package git_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/co2-lab/polvo/internal/git"
)

// initRepo initialises a new git repository in dir with a user identity.
// Returns a Client configured for that directory.
func initRepo(t *testing.T, dir string) *git.ExecClient {
	t.Helper()
	ctx := context.Background()
	_ = ctx

	run := func(args ...string) {
		t.Helper()
		allArgs := append([]string{"-C", dir}, args...)
		cmd := exec.Command("git", allArgs...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init", "-b", "main")
	run("config", "user.email", "test@polvo.test")
	run("config", "user.name", "Polvo Test")

	return &git.ExecClient{WorkDir: dir}
}

// writeFile writes content to a file inside dir, creating it if needed.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("writeFile %s: %v", name, err)
	}
}

func TestExecClient_IsGitRepo_False(t *testing.T) {
	dir := t.TempDir()
	c := &git.ExecClient{WorkDir: dir}
	if c.IsGitRepo(context.Background()) {
		t.Error("expected non-git dir to return false")
	}
}

func TestExecClient_Status(t *testing.T) {
	dir := t.TempDir()
	c := initRepo(t, dir)
	ctx := context.Background()

	// Empty repo — no untracked files yet
	statuses, err := c.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(statuses) != 0 {
		t.Errorf("expected 0 statuses in fresh repo, got %d", len(statuses))
	}

	// Write an untracked file
	writeFile(t, dir, "hello.txt", "hello")
	statuses, err = c.Status(ctx)
	if err != nil {
		t.Fatalf("Status after write: %v", err)
	}
	if len(statuses) == 0 {
		t.Error("expected at least one status entry for untracked file")
	}
	found := false
	for _, s := range statuses {
		if s.Path == "hello.txt" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected hello.txt in status, got %+v", statuses)
	}
}

func TestExecClient_AddAndCommit(t *testing.T) {
	dir := t.TempDir()
	c := initRepo(t, dir)
	ctx := context.Background()

	writeFile(t, dir, "file.txt", "content")
	if err := c.Add(ctx, []string{"file.txt"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := c.Commit(ctx, "test: initial commit"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	clean, err := c.IsCleanWorkingTree(ctx)
	if err != nil {
		t.Fatalf("IsCleanWorkingTree: %v", err)
	}
	if !clean {
		t.Error("expected clean tree after commit")
	}
}

func TestExecClient_CreateBranch(t *testing.T) {
	dir := t.TempDir()
	c := initRepo(t, dir)
	ctx := context.Background()

	// Need at least one commit before branching
	writeFile(t, dir, "seed.txt", "seed")
	if err := c.Add(ctx, []string{"seed.txt"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := c.Commit(ctx, "chore: seed commit"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if err := c.CreateBranch(ctx, "feature/test"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	branch, err := c.CurrentBranch(ctx)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if branch != "feature/test" {
		t.Errorf("expected branch 'feature/test', got %q", branch)
	}
}

func TestExecClient_IsCleanWorkingTree(t *testing.T) {
	dir := t.TempDir()
	c := initRepo(t, dir)
	ctx := context.Background()

	writeFile(t, dir, "a.txt", "a")
	if err := c.AddAll(ctx); err != nil {
		t.Fatalf("AddAll: %v", err)
	}
	if err := c.Commit(ctx, "init"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	clean, err := c.IsCleanWorkingTree(ctx)
	if err != nil {
		t.Fatalf("IsCleanWorkingTree: %v", err)
	}
	if !clean {
		t.Error("expected clean tree after commit")
	}

	// Modify the file — should make the tree dirty
	writeFile(t, dir, "a.txt", "modified")
	clean, err = c.IsCleanWorkingTree(ctx)
	if err != nil {
		t.Fatalf("IsCleanWorkingTree after modify: %v", err)
	}
	if clean {
		t.Error("expected dirty tree after file modification")
	}
}

func TestExecClient_CurrentBranch(t *testing.T) {
	dir := t.TempDir()
	c := initRepo(t, dir)
	ctx := context.Background()

	// Need a commit to resolve HEAD
	writeFile(t, dir, "f.txt", "f")
	if err := c.Add(ctx, []string{"f.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Commit(ctx, "init"); err != nil {
		t.Fatal(err)
	}

	branch, err := c.CurrentBranch(ctx)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if branch == "" {
		t.Error("expected non-empty branch name")
	}
}
