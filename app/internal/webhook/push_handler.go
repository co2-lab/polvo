// Package webhook handles GitHub webhook events.
package webhook

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v69/github"

	"github.com/co2-lab/polvo/internal/config"
	"github.com/co2-lab/polvo/internal/pipeline"
)

// PushHandler processes push events from GitHub.
type PushHandler struct {
	scheduler *pipeline.Scheduler
	cfg       *config.Config
	logger    *slog.Logger
}

// NewPushHandler creates a new push event handler.
func NewPushHandler(scheduler *pipeline.Scheduler, cfg *config.Config, logger *slog.Logger) *PushHandler {
	return &PushHandler{
		scheduler: scheduler,
		cfg:       cfg,
		logger:    logger,
	}
}

// Handle processes a push event.
func (h *PushHandler) Handle(event *github.PushEvent) {
	// Only process pushes to the target branch
	ref := event.GetRef()
	targetRef := "refs/heads/" + h.cfg.Git.TargetBranch
	if ref != targetRef {
		h.logger.Debug("ignoring push to non-target branch", "ref", ref)
		return
	}

	// Skip pushes from polvo branches (avoid loops)
	if strings.Contains(ref, h.cfg.Git.BranchPrefix) {
		h.logger.Debug("ignoring push from polvo branch", "ref", ref)
		return
	}

	owner := event.GetRepo().GetOwner().GetLogin()
	repo := event.GetRepo().GetName()

	// Process each modified file
	for _, commit := range event.Commits {
		files := append(commit.Added, commit.Modified...)
		for _, file := range files {
			if h.isMonitoredFile(file) {
				h.logger.Info("detected change in push", "file", file, "commit", commit.GetID())

				fileEvent := &pipeline.FileEvent{
					Type:   classifyFile(file),
					File:   file,
					Owner:  owner,
					Repo:   repo,
					Branch: h.cfg.Git.TargetBranch,
				}

				if err := h.scheduler.HandleEvent(context.Background(), fileEvent); err != nil {
					h.logger.Error("pipeline error", "error", err, "file", file)
				}
			}
		}
	}
}

func (h *PushHandler) isMonitoredFile(file string) bool {
	if strings.HasSuffix(file, ".spec.md") {
		return true
	}

	for _, pattern := range h.cfg.AllInterfacePatterns() {
		matched, _ := filepath.Match(pattern, file)
		if matched {
			return true
		}
		matched, _ = filepath.Match(pattern, filepath.Base(file))
		if matched {
			return true
		}
	}

	return false
}

func classifyFile(file string) pipeline.EventType {
	if strings.HasSuffix(file, ".spec.md") {
		return pipeline.EventSpecChanged
	}
	return pipeline.EventInterfaceChanged
}
