package git

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// OrchestratorConfig mirrors the git section of config.GitConfig.
type OrchestratorConfig struct {
	AutoCommit     bool
	DirtyCommit    bool   // commit pre-existing dirty files before run
	BranchPerRun   bool
	BranchTemplate string // default: "polvo/{{agent}}/{{timestamp}}"
	Attribution    string // default: "(polvo)"

	// Remote / PR fields (Plan 46)
	PushAfterCommit bool      // push to RemoteName after AutoCommit
	PROnCompletion  bool      // open PR after push
	PRDraft         bool      // draft PR (conservative default)
	PRBase          string    // base branch for PR (default "main")
	PRTitleTemplate string    // "{{agent}}: {{task_short}}"
	RemoteName      string    // default "origin"
	PRCreator       PRCreator // nil = PR disabled
}

// Orchestrator wraps pre/post run git operations.
type Orchestrator struct {
	cfg    OrchestratorConfig
	client Client
	msgGen MessageGenerator // interface for commit message generation
}

// MessageGenerator generates a commit message from a diff.
type MessageGenerator interface {
	Generate(ctx context.Context, diff string) (string, error)
}

// NewOrchestrator creates an Orchestrator. msgGen may be nil (uses fallback message).
func NewOrchestrator(cfg OrchestratorConfig, client Client, msgGen MessageGenerator) *Orchestrator {
	return &Orchestrator{
		cfg:    cfg,
		client: client,
		msgGen: msgGen,
	}
}

// PreRun runs before the agent executes:
// 1. If DirtyCommit: commit pre-existing dirty files.
// 2. If BranchPerRun: create and checkout a new branch.
// Errors are non-fatal (logged at WARN).
func (o *Orchestrator) PreRun(ctx context.Context, agentName string) error {
	if !o.client.IsGitRepo(ctx) {
		slog.Warn("git_orchestrator: not a git repo, skipping pre-run")
		return nil
	}

	if o.cfg.DirtyCommit {
		if err := o.commitDirty(ctx, agentName); err != nil {
			slog.Warn("git_orchestrator: dirty commit failed", "agent", agentName, "err", err)
			// non-fatal: continue
		}
	}

	if o.cfg.BranchPerRun {
		tmpl := o.cfg.BranchTemplate
		if tmpl == "" {
			tmpl = "polvo/{{agent}}/{{timestamp}}"
		}
		branchName := renderBranchTemplate(tmpl, agentName, time.Now())
		if err := o.client.CreateBranch(ctx, branchName); err != nil {
			slog.Warn("git_orchestrator: create branch failed", "agent", agentName, "branch", branchName, "err", err)
			// non-fatal: continue
		} else {
			slog.Info("git_orchestrator: created branch", "branch", branchName)
		}
	}

	return nil
}

// PostRun runs after a successful agent execution:
// 1. If AutoCommit: stage modified files + commit with generated message.
// 2. If PushAfterCommit: conflict check + push.
// 3. If PROnCompletion: check for existing PR or create a new one.
// All steps are non-fatal (errors logged at WARN).
func (o *Orchestrator) PostRun(ctx context.Context, agentName string, modifiedFiles []string) error {
	if !o.cfg.AutoCommit {
		return nil
	}

	if !o.client.IsGitRepo(ctx) {
		slog.Warn("git_orchestrator: not a git repo, skipping post-run")
		return nil
	}

	// Stage files
	var stageErr error
	if len(modifiedFiles) > 0 {
		stageErr = o.client.Add(ctx, modifiedFiles)
	} else {
		stageErr = o.client.AddAll(ctx)
	}
	if stageErr != nil {
		slog.Warn("git_orchestrator: staging failed", "agent", agentName, "err", stageErr)
		return nil
	}

	// Check if there is anything to commit
	clean, err := o.client.IsCleanWorkingTree(ctx)
	if err != nil {
		slog.Warn("git_orchestrator: status check failed", "agent", agentName, "err", err)
		return nil
	}
	if clean {
		slog.Info("git_orchestrator: nothing to commit", "agent", agentName)
		return nil
	}

	// Generate commit message
	msg := o.buildCommitMessage(ctx, agentName)

	if err := o.client.Commit(ctx, msg); err != nil {
		slog.Warn("git_orchestrator: commit failed", "agent", agentName, "err", err)
		return nil
	}
	slog.Info("git_orchestrator: committed changes", "agent", agentName, "message", msg)

	if !o.cfg.PushAfterCommit {
		return nil
	}

	remote := o.cfg.RemoteName
	if remote == "" {
		remote = "origin"
	}

	// Conflict detection: fetch + check if remote branch has diverged.
	branch, err := o.client.CurrentBranch(ctx)
	if err != nil {
		slog.Warn("git_orchestrator: cannot get current branch", "err", err)
		return nil
	}
	if diverged := o.checkConflict(ctx, remote, branch); diverged {
		slog.Warn("git_orchestrator: remote branch has diverged — skipping push; run git pull --rebase manually",
			"remote", remote, "branch", branch)
		return nil
	}

	// Try SetUpstream first (first push of new branch); fall back to plain Push.
	if err := o.client.SetUpstream(ctx, remote, branch); err != nil {
		if err2 := o.client.Push(ctx, remote, branch, false); err2 != nil {
			slog.Warn("git_orchestrator: push failed", "agent", agentName, "remote", remote, "branch", branch, "err", err2)
			return nil
		}
	}
	slog.Info("git_orchestrator: pushed branch", "remote", remote, "branch", branch)

	if !o.cfg.PROnCompletion || o.cfg.PRCreator == nil {
		return nil
	}

	// Check for existing PR.
	existingURL, _ := o.cfg.PRCreator.ExistingPRURL(ctx, branch)
	if existingURL != "" {
		slog.Info("git_orchestrator: PR already exists", "url", existingURL)
		return nil
	}

	// Resolve owner/repo from remote URL.
	remoteURL, err := o.client.RemoteURL(ctx, remote)
	if err != nil {
		slog.Warn("git_orchestrator: cannot get remote URL", "err", err)
		return nil
	}
	owner, repo := ParseOwnerRepo(remoteURL)
	if owner == "" || repo == "" {
		slog.Warn("git_orchestrator: cannot parse owner/repo from remote URL", "url", remoteURL)
		return nil
	}

	base := o.cfg.PRBase
	if base == "" {
		base = "main"
	}
	title := renderPRTitle(o.cfg.PRTitleTemplate, agentName)
	body := BuildPRBody(agentName, "", modifiedFiles)

	pr, err := o.cfg.PRCreator.CreatePR(ctx, PROptions{
		Owner: owner,
		Repo:  repo,
		Title: title,
		Body:  body,
		Head:  branch,
		Base:  base,
		Draft: o.cfg.PRDraft,
	})
	if err != nil {
		slog.Warn("git_orchestrator: PR creation failed", "agent", agentName, "err", err)
		return nil
	}
	slog.Info("git_orchestrator: PR created", "url", pr.URL, "draft", pr.Draft)
	return nil
}

// checkConflict returns true if the remote branch has diverged from local HEAD.
// A failure to fetch is treated as no-divergence (conservative: let push fail naturally).
func (o *Orchestrator) checkConflict(ctx context.Context, remote, branch string) bool {
	if err := o.client.FetchRemote(ctx, remote); err != nil {
		slog.Warn("git_orchestrator: fetch failed during conflict check", "err", err)
		return false
	}
	// git merge-base returns the common ancestor; if it equals the remote tip, no divergence.
	execC, ok := o.client.(*ExecClient)
	if !ok {
		return false
	}
	remoteBranch := remote + "/" + branch
	mergeBase, err := execC.run(ctx, "merge-base", "HEAD", remoteBranch)
	if err != nil {
		// Remote branch doesn't exist yet — new branch, no conflict.
		return false
	}
	remoteTip, err := execC.run(ctx, "rev-parse", remoteBranch)
	if err != nil {
		return false
	}
	return strings.TrimSpace(mergeBase) != strings.TrimSpace(remoteTip)
}

// renderPRTitle substitutes {{agent}} in the title template.
func renderPRTitle(tmpl, agentName string) string {
	if tmpl == "" {
		return fmt.Sprintf("%s: automated changes", agentName)
	}
	return strings.ReplaceAll(tmpl, "{{agent}}", agentName)
}

// commitDirty stages and commits any pre-existing dirty files before the run.
func (o *Orchestrator) commitDirty(ctx context.Context, agentName string) error {
	clean, err := o.client.IsCleanWorkingTree(ctx)
	if err != nil {
		return fmt.Errorf("checking working tree: %w", err)
	}
	if clean {
		return nil
	}

	if err := o.client.AddAll(ctx); err != nil {
		return fmt.Errorf("staging dirty files: %w", err)
	}

	attribution := o.cfg.Attribution
	if attribution == "" {
		attribution = "(polvo)"
	}
	msg := fmt.Sprintf("chore(%s): save pre-run state %s", sanitizeBranchComponent(agentName), attribution)
	return o.client.Commit(ctx, msg)
}

// buildCommitMessage generates a commit message, falling back to a default.
func (o *Orchestrator) buildCommitMessage(ctx context.Context, agentName string) string {
	attribution := o.cfg.Attribution
	if attribution == "" {
		attribution = "(polvo)"
	}

	if o.msgGen != nil {
		diff, err := o.client.Diff(ctx, true) // staged diff
		if err == nil && diff != "" {
			msg, err := o.msgGen.Generate(ctx, diff)
			if err == nil && msg != "" {
				return msg
			}
		}
	}

	return fmt.Sprintf("chore(%s): apply agent changes %s", sanitizeBranchComponent(agentName), attribution)
}

// renderBranchTemplate substitutes {{agent}} and {{timestamp}} placeholders.
func renderBranchTemplate(tmpl, agentName string, t time.Time) string {
	s := strings.ReplaceAll(tmpl, "{{agent}}", sanitizeBranchComponent(agentName))
	s = strings.ReplaceAll(s, "{{timestamp}}", t.Format("20060102-150405"))
	return s
}

// sanitizeBranchComponent replaces characters that are invalid in git branch names with '-'.
func sanitizeBranchComponent(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
