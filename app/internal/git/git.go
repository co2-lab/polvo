// Package git provides local git operations via os/exec.
// NOTE: internal/gitclient/ handles GitHub REST API; this package handles local repo operations.
package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// FileStatus represents a file in the git working tree.
type FileStatus struct {
	Path     string
	Staging  rune // X in 'XY' porcelain output
	Worktree rune // Y
}

// Client wraps local git operations.
type Client interface {
	Status(ctx context.Context) ([]FileStatus, error)
	Diff(ctx context.Context, staged bool) (string, error)
	DiffFiles(ctx context.Context, paths []string, staged bool) (string, error)
	Add(ctx context.Context, paths []string) error
	AddAll(ctx context.Context) error
	Commit(ctx context.Context, message string) error
	CreateBranch(ctx context.Context, name string) error
	CheckoutBranch(ctx context.Context, name string) error
	CurrentBranch(ctx context.Context) (string, error)
	IsCleanWorkingTree(ctx context.Context) (bool, error)
	IsGitRepo(ctx context.Context) bool
	RepoRoot(ctx context.Context) (string, error)

	// Remote operations.
	Push(ctx context.Context, remote, branch string, force bool) error
	SetUpstream(ctx context.Context, remote, branch string) error
	FetchRemote(ctx context.Context, remote string) error
	PullRebase(ctx context.Context, remote, branch string) error
	RemoteURL(ctx context.Context, remote string) (string, error)
	ListRemoteBranches(ctx context.Context, remote string) ([]string, error)
}

// ExecClient implements Client using os/exec.
type ExecClient struct {
	WorkDir string
}

// run executes a git command in the WorkDir and returns trimmed stdout.
func (c *ExecClient) run(ctx context.Context, args ...string) (string, error) {
	allArgs := append([]string{"-C", c.WorkDir}, args...)
	cmd := exec.CommandContext(ctx, "git", allArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

// Status returns the working tree status using `git status --porcelain=v1`.
func (c *ExecClient) Status(ctx context.Context) ([]FileStatus, error) {
	out, err := c.run(ctx, "status", "--porcelain=v1")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	lines := strings.Split(out, "\n")
	statuses := make([]FileStatus, 0, len(lines))
	for _, line := range lines {
		if len(line) < 4 {
			continue
		}
		statuses = append(statuses, FileStatus{
			Staging:  rune(line[0]),
			Worktree: rune(line[1]),
			Path:     line[3:],
		})
	}
	return statuses, nil
}

// Diff returns a unified diff. If staged is true, returns `git diff --cached`;
// otherwise returns `git diff HEAD`.
func (c *ExecClient) Diff(ctx context.Context, staged bool) (string, error) {
	if staged {
		return c.run(ctx, "diff", "--cached")
	}
	return c.run(ctx, "diff", "HEAD")
}

// DiffFiles returns a diff restricted to the given paths.
func (c *ExecClient) DiffFiles(ctx context.Context, paths []string, staged bool) (string, error) {
	if len(paths) == 0 {
		return c.Diff(ctx, staged)
	}
	args := []string{"diff"}
	if staged {
		args = append(args, "--cached")
	} else {
		args = append(args, "HEAD")
	}
	args = append(args, "--")
	args = append(args, paths...)
	return c.run(ctx, args...)
}

// Add stages the given paths.
func (c *ExecClient) Add(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"add", "--"}, paths...)
	_, err := c.run(ctx, args...)
	return err
}

// AddAll stages all changes (`git add -A`).
func (c *ExecClient) AddAll(ctx context.Context) error {
	_, err := c.run(ctx, "add", "-A")
	return err
}

// Commit creates a commit with the given message.
func (c *ExecClient) Commit(ctx context.Context, message string) error {
	_, err := c.run(ctx, "commit", "-m", message)
	return err
}

// CreateBranch creates and checks out a new branch.
func (c *ExecClient) CreateBranch(ctx context.Context, name string) error {
	_, err := c.run(ctx, "checkout", "-b", name)
	return err
}

// CheckoutBranch checks out an existing branch.
func (c *ExecClient) CheckoutBranch(ctx context.Context, name string) error {
	_, err := c.run(ctx, "checkout", name)
	return err
}

// CurrentBranch returns the name of the current branch.
func (c *ExecClient) CurrentBranch(ctx context.Context) (string, error) {
	return c.run(ctx, "rev-parse", "--abbrev-ref", "HEAD")
}

// IsCleanWorkingTree returns true when there are no uncommitted changes.
func (c *ExecClient) IsCleanWorkingTree(ctx context.Context) (bool, error) {
	out, err := c.run(ctx, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return out == "", nil
}

// IsGitRepo returns true when the WorkDir is inside a git repository.
func (c *ExecClient) IsGitRepo(ctx context.Context) bool {
	_, err := c.run(ctx, "rev-parse", "--git-dir")
	return err == nil
}

// RepoRoot returns the absolute path of the repository root.
func (c *ExecClient) RepoRoot(ctx context.Context) (string, error) {
	return c.run(ctx, "rev-parse", "--show-toplevel")
}

// Push pushes the given branch to the remote.
// NOTE: --no-verify is never passed (Polvo policy).
func (c *ExecClient) Push(ctx context.Context, remote, branch string, force bool) error {
	args := []string{"push", remote, branch}
	if force {
		args = append(args, "--force-with-lease")
	}
	_, err := c.run(ctx, args...)
	return err
}

// SetUpstream pushes and sets the upstream tracking branch.
// Use for the first push of a new branch.
func (c *ExecClient) SetUpstream(ctx context.Context, remote, branch string) error {
	_, err := c.run(ctx, "push", "--set-upstream", remote, branch)
	return err
}

// FetchRemote fetches all refs from the remote.
func (c *ExecClient) FetchRemote(ctx context.Context, remote string) error {
	_, err := c.run(ctx, "fetch", remote)
	return err
}

// PullRebase fetches and rebases the current branch onto remote/branch.
func (c *ExecClient) PullRebase(ctx context.Context, remote, branch string) error {
	_, err := c.run(ctx, "pull", "--rebase", remote, branch)
	return err
}

// RemoteURL returns the URL configured for the given remote.
func (c *ExecClient) RemoteURL(ctx context.Context, remote string) (string, error) {
	return c.run(ctx, "remote", "get-url", remote)
}

// ListRemoteBranches returns branch names on the remote (without the "remote/" prefix).
func (c *ExecClient) ListRemoteBranches(ctx context.Context, remote string) ([]string, error) {
	out, err := c.run(ctx, "ls-remote", "--heads", remote)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	var branches []string
	for _, line := range strings.Split(out, "\n") {
		// Format: "<sha>\trefs/heads/<branch>"
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		ref := strings.TrimPrefix(parts[1], "refs/heads/")
		if ref != "" {
			branches = append(branches, ref)
		}
	}
	return branches, nil
}
