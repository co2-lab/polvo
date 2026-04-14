package tool_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/co2-lab/polvo/internal/git"
	"github.com/co2-lab/polvo/internal/tool"
)

// initDiffRepo initialises a git repo in dir and returns a git.Client.
func initDiffRepo(t *testing.T, dir string) git.Client {
	t.Helper()
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

func writeDiffFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestDiffTool_ShowsChanges checks that the diff tool returns non-empty output
// when there are uncommitted changes.
func TestDiffTool_ShowsChanges(t *testing.T) {
	dir := t.TempDir()
	c := initDiffRepo(t, dir)
	ctx := context.Background()

	// Create a base commit
	writeDiffFile(t, dir, "file.txt", "original\n")
	run := func(args ...string) {
		t.Helper()
		allArgs := append([]string{"-C", dir}, args...)
		cmd := exec.Command("git", allArgs...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("add", "file.txt")
	run("commit", "-m", "init")

	// Modify the file — creates an unstaged diff
	writeDiffFile(t, dir, "file.txt", "modified\n")
	run("add", "file.txt") // stage it so diff HEAD works

	diffTool := tool.NewDiff(c)

	input, _ := json.Marshal(map[string]interface{}{
		"staged": false,
		"paths":  []string{},
	})
	result, err := diffTool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "modified") && result.Content == "no changes" {
		t.Errorf("expected diff to contain changes, got: %s", result.Content)
	}
}

// TestDiffTool_EmptyOnCleanRepo checks that the diff tool returns "no changes"
// when the working tree is clean.
func TestDiffTool_EmptyOnCleanRepo(t *testing.T) {
	dir := t.TempDir()
	c := initDiffRepo(t, dir)
	ctx := context.Background()

	run := func(args ...string) {
		t.Helper()
		allArgs := append([]string{"-C", dir}, args...)
		cmd := exec.Command("git", allArgs...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// Commit a file so HEAD exists
	writeDiffFile(t, dir, "clean.txt", "clean\n")
	run("add", "clean.txt")
	run("commit", "-m", "init")

	diffTool := tool.NewDiff(c)

	input, _ := json.Marshal(map[string]interface{}{
		"staged": false,
	})
	result, err := diffTool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content)
	}
	if result.Content != "no changes" {
		t.Errorf("expected 'no changes' on clean repo, got: %q", result.Content)
	}
}
