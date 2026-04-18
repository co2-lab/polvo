package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initRepo creates a git repo in dir with an initial commit.
func initRepo(t *testing.T, dir string) {
	t.Helper()
	cmds := [][]string{
		{"git", "-C", dir, "init"},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
		{"git", "-C", dir, "commit", "--allow-empty", "-m", "initial"},
	}
	for _, args := range cmds {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
}

// initBareRepo creates a bare git repo at dir.
func initBareRepo(t *testing.T, dir string) {
	t.Helper()
	out, err := exec.Command("git", "init", "--bare", dir).CombinedOutput()
	if err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}
}

func addRemote(t *testing.T, repoDir, remoteName, remoteURL string) {
	t.Helper()
	out, err := exec.Command("git", "-C", repoDir, "remote", "add", remoteName, remoteURL).CombinedOutput()
	if err != nil {
		t.Fatalf("git remote add: %v\n%s", err, out)
	}
}

func TestExecClient_RemoteURL(t *testing.T) {
	repoDir := t.TempDir()
	initRepo(t, repoDir)
	bareDir := t.TempDir()
	initBareRepo(t, bareDir)
	addRemote(t, repoDir, "origin", bareDir)

	c := &ExecClient{WorkDir: repoDir}
	got, err := c.RemoteURL(context.Background(), "origin")
	if err != nil {
		t.Fatalf("RemoteURL: %v", err)
	}
	if got != bareDir {
		t.Errorf("RemoteURL = %q, want %q", got, bareDir)
	}
}

func TestExecClient_FetchRemote(t *testing.T) {
	repoDir := t.TempDir()
	initRepo(t, repoDir)
	bareDir := t.TempDir()
	initBareRepo(t, bareDir)
	addRemote(t, repoDir, "origin", bareDir)

	// First push so there's something to fetch.
	c := &ExecClient{WorkDir: repoDir}
	if err := c.SetUpstream(context.Background(), "origin", "master"); err != nil {
		// Try "main" if "master" fails.
		branch, _ := c.CurrentBranch(context.Background())
		if err2 := c.SetUpstream(context.Background(), "origin", branch); err2 != nil {
			t.Fatalf("SetUpstream: %v", err2)
		}
	}

	// Fetch should succeed.
	if err := c.FetchRemote(context.Background(), "origin"); err != nil {
		t.Fatalf("FetchRemote: %v", err)
	}
	// FETCH_HEAD should exist.
	if _, err := os.Stat(filepath.Join(repoDir, ".git", "FETCH_HEAD")); err != nil {
		t.Error("FETCH_HEAD not found after fetch")
	}
}

func TestExecClient_Push(t *testing.T) {
	repoDir := t.TempDir()
	initRepo(t, repoDir)
	bareDir := t.TempDir()
	initBareRepo(t, bareDir)
	addRemote(t, repoDir, "origin", bareDir)

	c := &ExecClient{WorkDir: repoDir}
	branch, err := c.CurrentBranch(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if err := c.SetUpstream(context.Background(), "origin", branch); err != nil {
		t.Fatalf("SetUpstream: %v", err)
	}

	// Verify commit exists in bare repo.
	out, err := exec.Command("git", "-C", bareDir, "log", "--oneline").CombinedOutput()
	if err != nil {
		t.Fatalf("git log on bare: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "initial") {
		t.Error("expected 'initial' commit in bare repo after push")
	}
}

func TestExecClient_ListRemoteBranches(t *testing.T) {
	repoDir := t.TempDir()
	initRepo(t, repoDir)
	bareDir := t.TempDir()
	initBareRepo(t, bareDir)
	addRemote(t, repoDir, "origin", bareDir)

	c := &ExecClient{WorkDir: repoDir}
	branch, _ := c.CurrentBranch(context.Background())
	if err := c.SetUpstream(context.Background(), "origin", branch); err != nil {
		t.Fatalf("SetUpstream: %v", err)
	}

	// Create a second branch and push it.
	if err := c.CreateBranch(context.Background(), "feature-x"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if err := c.SetUpstream(context.Background(), "origin", "feature-x"); err != nil {
		t.Fatalf("push feature-x: %v", err)
	}

	branches, err := c.ListRemoteBranches(context.Background(), "origin")
	if err != nil {
		t.Fatalf("ListRemoteBranches: %v", err)
	}
	found := map[string]bool{}
	for _, b := range branches {
		found[b] = true
	}
	if !found[branch] {
		t.Errorf("expected branch %q in remote branches %v", branch, branches)
	}
	if !found["feature-x"] {
		t.Errorf("expected 'feature-x' in remote branches %v", branches)
	}
	// Branches should not contain "origin/" prefix.
	for _, b := range branches {
		if strings.HasPrefix(b, "origin/") {
			t.Errorf("branch %q should not have remote prefix", b)
		}
	}
}

func TestExecClient_PullRebase(t *testing.T) {
	// Create a bare remote and two clones.
	bareDir := t.TempDir()
	initBareRepo(t, bareDir)

	// Clone 1: make initial commit + push.
	clone1 := t.TempDir()
	if out, err := exec.Command("git", "clone", bareDir, clone1).CombinedOutput(); err != nil {
		t.Fatalf("clone1: %v\n%s", err, out)
	}
	cmds := [][]string{
		{"git", "-C", clone1, "config", "user.email", "test@test.com"},
		{"git", "-C", clone1, "config", "user.name", "Test"},
		{"git", "-C", clone1, "commit", "--allow-empty", "-m", "from-clone1"},
		{"git", "-C", clone1, "push", "origin", "HEAD"},
	}
	for _, args := range cmds {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	// Clone 2: doesn't have clone1's commit yet.
	clone2 := t.TempDir()
	if out, err := exec.Command("git", "clone", bareDir, clone2).CombinedOutput(); err != nil {
		t.Fatalf("clone2: %v\n%s", err, out)
	}
	for _, args := range [][]string{
		{"git", "-C", clone2, "config", "user.email", "test@test.com"},
		{"git", "-C", clone2, "config", "user.name", "Test"},
	} {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	c2 := &ExecClient{WorkDir: clone2}
	branch, _ := c2.CurrentBranch(context.Background())
	if err := c2.PullRebase(context.Background(), "origin", branch); err != nil {
		t.Fatalf("PullRebase: %v", err)
	}

	// clone2 should now have clone1's commit.
	out, err := exec.Command("git", "-C", clone2, "log", "--oneline").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "from-clone1") {
		t.Errorf("PullRebase did not fetch clone1 commit; log:\n%s", out)
	}
}
