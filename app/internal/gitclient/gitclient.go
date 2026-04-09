// Package gitclient provides an abstraction for Git and GitHub operations.
package gitclient

import "context"

// PRInfo represents a pull request.
type PRInfo struct {
	Number  int
	URL     string
	Title   string
	State   string
	Branch  string
	DiffURL string
}

// ReviewResult represents the outcome of a PR review.
type ReviewResult struct {
	Decision string // APPROVE or REJECT
	Body     string
}

// GitPlatform is the interface for Git hosting operations.
type GitPlatform interface {
	// CreateBranch creates a new branch from the target branch.
	CreateBranch(ctx context.Context, owner, repo, branch, baseBranch string) error

	// CommitFile creates or updates a file on a branch.
	CommitFile(ctx context.Context, owner, repo, branch, path, message string, content []byte) error

	// CreatePR opens a pull request.
	CreatePR(ctx context.Context, owner, repo, title, body, head, base string, labels []string) (*PRInfo, error)

	// ReviewPR posts a review on a PR.
	ReviewPR(ctx context.Context, owner, repo string, prNumber int, review *ReviewResult) error

	// MergePR merges a pull request.
	MergePR(ctx context.Context, owner, repo string, prNumber int) error

	// ClosePR closes a pull request without merging.
	ClosePR(ctx context.Context, owner, repo string, prNumber int) error

	// CommentPR posts a comment on a PR.
	CommentPR(ctx context.Context, owner, repo string, prNumber int, body string) error

	// GetPRDiff returns the diff of a PR.
	GetPRDiff(ctx context.Context, owner, repo string, prNumber int) (string, error)

	// GetFileContent reads a file from a branch.
	GetFileContent(ctx context.Context, owner, repo, branch, path string) ([]byte, error)
}
