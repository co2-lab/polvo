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
// Errors are non-fatal.
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
	return nil
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
