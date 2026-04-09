package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/co2-lab/polvo/internal/agent"
	"github.com/co2-lab/polvo/internal/config"
	"github.com/co2-lab/polvo/internal/gitclient"
)

// EventPublisher is a minimal interface for publishing agent lifecycle events.
// It is implemented by *dashboard.Bus, defined here to avoid import cycles.
type EventPublisher interface {
	PublishAgentStarted(name, file string)
	PublishAgentDone(name, file string, errMsg string)
}

// Scheduler orchestrates the reaction chain execution.
type Scheduler struct {
	executor   *agent.Executor
	gitClient  gitclient.GitPlatform
	cfg        *config.Config
	chain      []Step
	maxRetries int
	logger     *slog.Logger
	pub        EventPublisher // may be nil
}

// NewScheduler creates a new pipeline scheduler.
func NewScheduler(executor *agent.Executor, gitClient gitclient.GitPlatform, cfg *config.Config, logger *slog.Logger, pub EventPublisher) *Scheduler {
	maxRetries := cfg.Review.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}

	return &Scheduler{
		executor:   executor,
		gitClient:  gitClient,
		cfg:        cfg,
		chain:      DefaultChain(),
		maxRetries: maxRetries,
		logger:     logger,
		pub:        pub,
	}
}

// UpdateDeps swaps in a new executor and config after a config reload.
func (s *Scheduler) UpdateDeps(executor *agent.Executor, cfg *config.Config) {
	s.executor = executor
	s.cfg = cfg
	s.maxRetries = cfg.Review.MaxRetries
	if s.maxRetries == 0 {
		s.maxRetries = 3
	}
}

// HandleEvent processes a file event through the reaction chain.
func (s *Scheduler) HandleEvent(ctx context.Context, event *FileEvent) error {
	s.logger.Info("handling event", "type", event.Type, "file", event.File)

	// Find matching step
	var step *Step
	for i := range s.chain {
		if s.chain[i].Trigger == event.Type {
			step = &s.chain[i]
			break
		}
	}

	if step == nil {
		// Handle inverse flow: interface changed
		if event.Type == EventInterfaceChanged {
			return s.handleInverseFlow(ctx, event)
		}
		s.logger.Debug("no chain step for event", "type", event.Type)
		return nil
	}

	return s.executeStep(ctx, step, event)
}

func (s *Scheduler) executeStep(ctx context.Context, step *Step, event *FileEvent) error {
	// Find interface group for overrides
	_, group := s.cfg.FindInterfaceGroup(event.File)

	// Get the author agent
	ag, err := s.executor.GetAgent(step.Agent, group)
	if err != nil {
		return fmt.Errorf("getting agent %q: %w", step.Agent, err)
	}

	s.logger.Info("executing agent", "agent", step.Agent, "file", event.File)

	// Publish agent started
	if s.pub != nil {
		s.pub.PublishAgentStarted(step.Agent, event.File)
	}

	// Execute the author agent
	input := &agent.Input{
		File:        event.File,
		Content:     event.Content,
		Diff:        event.Diff,
		Event:       string(event.Type),
		ProjectRoot: s.cfg.Project.Name,
	}

	result, err := ag.Execute(ctx, input)

	// Publish agent done (with error if any)
	if s.pub != nil {
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		}
		s.pub.PublishAgentDone(step.Agent, event.File, errMsg)
	}

	if err != nil {
		return fmt.Errorf("executing agent %q: %w", step.Agent, err)
	}

	// Create branch and PR
	branchName := fmt.Sprintf("%s%s/%s-%d",
		s.cfg.Git.BranchPrefix,
		step.Agent,
		sanitizeBranchName(event.File),
		time.Now().Unix(),
	)

	if err := s.gitClient.CreateBranch(ctx, event.Owner, event.Repo, branchName, s.cfg.Git.TargetBranch); err != nil {
		return fmt.Errorf("creating branch: %w", err)
	}

	// Commit the generated content
	commitMsg := fmt.Sprintf("polvo(%s): update %s", step.Agent, event.File)
	if err := s.gitClient.CommitFile(ctx, event.Owner, event.Repo, branchName, event.File, commitMsg, []byte(result.Content)); err != nil {
		return fmt.Errorf("committing file: %w", err)
	}

	// Create PR
	prTitle := fmt.Sprintf("[polvo/%s] %s", step.Agent, event.File)
	pr, err := s.gitClient.CreatePR(ctx, event.Owner, event.Repo, prTitle, result.Summary, branchName, s.cfg.Git.TargetBranch, s.cfg.Git.PRLabels)
	if err != nil {
		return fmt.Errorf("creating PR: %w", err)
	}

	s.logger.Info("PR created", "url", pr.URL, "agent", step.Agent)

	// Run review gates
	return s.runReview(ctx, event, pr, step)
}

func (s *Scheduler) runReview(ctx context.Context, event *FileEvent, pr *gitclient.PRInfo, step *Step) error {
	// Get PR diff for reviewers
	diff, err := s.gitClient.GetPRDiff(ctx, event.Owner, event.Repo, pr.Number)
	if err != nil {
		return fmt.Errorf("getting PR diff: %w", err)
	}

	reviewInput := &agent.Input{
		File:   event.File,
		PRDiff: diff,
		Event:  "pr_review",
	}

	// Find interface group for overrides
	_, group := s.cfg.FindInterfaceGroup(event.File)

	// Run gates: lint, best-practices
	gates := ReviewChain()
	for _, gateName := range gates {
		gate, err := s.executor.GetAgent(gateName, group)
		if err != nil {
			s.logger.Warn("gate agent not available", "gate", gateName, "error", err)
			continue
		}

		result, err := gate.Execute(ctx, reviewInput)
		if err != nil {
			return fmt.Errorf("gate %q execution: %w", gateName, err)
		}

		// Post review
		review := &gitclient.ReviewResult{
			Decision: result.Decision,
			Body:     result.Summary,
		}
		if err := s.gitClient.ReviewPR(ctx, event.Owner, event.Repo, pr.Number, review); err != nil {
			return fmt.Errorf("posting review for gate %q: %w", gateName, err)
		}

		if result.Decision == "REJECT" {
			s.logger.Info("PR rejected by gate", "gate", gateName, "pr", pr.Number)

			if event.RetryCount >= s.maxRetries {
				s.logger.Warn("max retries reached, escalating to human", "pr", pr.Number)
				_ = s.gitClient.CommentPR(ctx, event.Owner, event.Repo, pr.Number,
					fmt.Sprintf("**Polvo**: Maximum retries (%d) reached. Escalating to human review.", s.maxRetries))
				return nil
			}

			// Retry: re-run the step with feedback
			event.RetryCount++
			event.Content = result.Summary // feedback for next attempt
			return s.executeStep(ctx, step, event)
		}
	}

	// All gates passed — merge if auto_merge is on
	if s.cfg.Review.AutoMerge {
		if err := s.gitClient.MergePR(ctx, event.Owner, event.Repo, pr.Number); err != nil {
			return fmt.Errorf("merging PR #%d: %w", pr.Number, err)
		}
		s.logger.Info("PR merged", "pr", pr.Number)

		// Emit next event in the chain
		if step.Next != "" {
			nextEvent := &FileEvent{
				Type:   step.Next,
				File:   event.File,
				Owner:  event.Owner,
				Repo:   event.Repo,
				Branch: s.cfg.Git.TargetBranch,
			}
			return s.HandleEvent(ctx, nextEvent)
		}
	}

	return nil
}

func (s *Scheduler) handleInverseFlow(ctx context.Context, event *FileEvent) error {
	s.logger.Info("inverse flow: checking spec coherence", "file", event.File)

	// Find interface group for overrides and spec resolution
	_, group := s.cfg.FindInterfaceGroup(event.File)
	var specContent string
	if group != nil {
		// Resolve spec path using the new helper with defaults
		specPath := group.ResolveSpecPath(event.File)
		if specPath != "" {
			data, err := os.ReadFile(specPath)
			if err == nil {
				specContent = string(data)
			} else {
				s.logger.Warn("spec file not found", "path", specPath, "error", err)
			}
		}
	}

	specAgent, err := s.executor.GetAgent("spec", group)
	if err != nil {
		return fmt.Errorf("getting spec agent: %w", err)
	}

	if s.pub != nil {
		s.pub.PublishAgentStarted("spec", event.File)
	}

	input := &agent.Input{
		File:      event.File,
		Content:   event.Content,
		Diff:      event.Diff,
		Event:     "interface_changed",
		Interface: event.Content,
		Spec:      specContent,
	}

	result, err := specAgent.Execute(ctx, input)

	if s.pub != nil {
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		}
		s.pub.PublishAgentDone("spec", event.File, errMsg)
	}

	if err != nil {
		return fmt.Errorf("spec coherence check: %w", err)
	}

	s.logger.Info("spec coherence result", "decision", result.Decision, "file", event.File)
	return nil
}

func sanitizeBranchName(name string) string {
	r := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ".", "-")
	return r.Replace(name)
}
