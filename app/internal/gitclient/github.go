package gitclient

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v69/github"
)

// GitHubClient implements GitPlatform using the GitHub API via a GitHub App.
type GitHubClient struct {
	client *github.Client
}

// NewGitHubClient creates a client authenticated as a GitHub App installation.
func NewGitHubClient(appID, installationID int64, privateKeyPath string) (*GitHubClient, error) {
	transport, err := ghinstallation.NewKeyFromFile(http.DefaultTransport, appID, installationID, privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("creating github app transport: %w", err)
	}

	client := github.NewClient(&http.Client{Transport: transport})
	return &GitHubClient{client: client}, nil
}

// NewGitHubClientWithToken creates a client authenticated with a personal access token.
func NewGitHubClientWithToken(token string) *GitHubClient {
	client := github.NewClient(nil).WithAuthToken(token)
	return &GitHubClient{client: client}
}

func (g *GitHubClient) CreateBranch(ctx context.Context, owner, repo, branch, baseBranch string) error {
	// Get the SHA of the base branch
	ref, _, err := g.client.Git.GetRef(ctx, owner, repo, "refs/heads/"+baseBranch)
	if err != nil {
		return fmt.Errorf("getting base branch ref: %w", err)
	}

	// Create new branch
	newRef := &github.Reference{
		Ref:    github.Ptr("refs/heads/" + branch),
		Object: &github.GitObject{SHA: ref.Object.SHA},
	}
	_, _, err = g.client.Git.CreateRef(ctx, owner, repo, newRef)
	if err != nil {
		return fmt.Errorf("creating branch %s: %w", branch, err)
	}

	return nil
}

func (g *GitHubClient) CommitFile(ctx context.Context, owner, repo, branch, path, message string, content []byte) error {
	opts := &github.RepositoryContentFileOptions{
		Message: github.Ptr(message),
		Content: content,
		Branch:  github.Ptr(branch),
	}

	// Check if file exists to get SHA for update
	existing, _, _, err := g.client.Repositories.GetContents(ctx, owner, repo, path, &github.RepositoryContentGetOptions{Ref: branch})
	if err == nil && existing != nil {
		opts.SHA = existing.SHA
	}

	_, _, err = g.client.Repositories.CreateFile(ctx, owner, repo, path, opts)
	if err != nil {
		return fmt.Errorf("committing file %s: %w", path, err)
	}

	return nil
}

func (g *GitHubClient) CreatePR(ctx context.Context, owner, repo, title, body, head, base string, labels []string) (*PRInfo, error) {
	pr, _, err := g.client.PullRequests.Create(ctx, owner, repo, &github.NewPullRequest{
		Title: github.Ptr(title),
		Body:  github.Ptr(body),
		Head:  github.Ptr(head),
		Base:  github.Ptr(base),
	})
	if err != nil {
		return nil, fmt.Errorf("creating PR: %w", err)
	}

	// Add labels
	if len(labels) > 0 {
		_, _, _ = g.client.Issues.AddLabelsToIssue(ctx, owner, repo, pr.GetNumber(), labels)
	}

	return &PRInfo{
		Number: pr.GetNumber(),
		URL:    pr.GetHTMLURL(),
		Title:  pr.GetTitle(),
		State:  pr.GetState(),
		Branch: head,
	}, nil
}

func (g *GitHubClient) ReviewPR(ctx context.Context, owner, repo string, prNumber int, review *ReviewResult) error {
	event := "COMMENT"
	if review.Decision == "APPROVE" {
		event = "APPROVE"
	} else if review.Decision == "REJECT" {
		event = "REQUEST_CHANGES"
	}

	_, _, err := g.client.PullRequests.CreateReview(ctx, owner, repo, prNumber, &github.PullRequestReviewRequest{
		Body:  github.Ptr(review.Body),
		Event: github.Ptr(event),
	})
	if err != nil {
		return fmt.Errorf("reviewing PR #%d: %w", prNumber, err)
	}

	return nil
}

func (g *GitHubClient) MergePR(ctx context.Context, owner, repo string, prNumber int) error {
	_, _, err := g.client.PullRequests.Merge(ctx, owner, repo, prNumber, "", &github.PullRequestOptions{
		MergeMethod: "squash",
	})
	if err != nil {
		return fmt.Errorf("merging PR #%d: %w", prNumber, err)
	}
	return nil
}

func (g *GitHubClient) ClosePR(ctx context.Context, owner, repo string, prNumber int) error {
	_, _, err := g.client.PullRequests.Edit(ctx, owner, repo, prNumber, &github.PullRequest{
		State: github.Ptr("closed"),
	})
	if err != nil {
		return fmt.Errorf("closing PR #%d: %w", prNumber, err)
	}
	return nil
}

func (g *GitHubClient) CommentPR(ctx context.Context, owner, repo string, prNumber int, body string) error {
	_, _, err := g.client.Issues.CreateComment(ctx, owner, repo, prNumber, &github.IssueComment{
		Body: github.Ptr(body),
	})
	if err != nil {
		return fmt.Errorf("commenting on PR #%d: %w", prNumber, err)
	}
	return nil
}

func (g *GitHubClient) GetPRDiff(ctx context.Context, owner, repo string, prNumber int) (string, error) {
	diff, resp, err := g.client.PullRequests.GetRaw(ctx, owner, repo, prNumber, github.RawOptions{Type: github.Diff})
	if err != nil {
		return "", fmt.Errorf("getting PR diff #%d: %w", prNumber, err)
	}
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	return diff, nil
}

func (g *GitHubClient) GetFileContent(ctx context.Context, owner, repo, branch, path string) ([]byte, error) {
	reader, resp, err := g.client.Repositories.DownloadContents(ctx, owner, repo, path, &github.RepositoryContentGetOptions{Ref: branch})
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", path, err)
	}
	defer reader.Close()
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return data, nil
}
