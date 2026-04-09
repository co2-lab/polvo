package webhook

import (
	"log/slog"

	"github.com/google/go-github/v69/github"
)

// PRHandler processes pull request events from GitHub.
type PRHandler struct {
	logger *slog.Logger
}

// NewPRHandler creates a new PR event handler.
func NewPRHandler(logger *slog.Logger) *PRHandler {
	return &PRHandler{logger: logger}
}

// Handle processes a pull request event.
func (h *PRHandler) Handle(event *github.PullRequestEvent) {
	action := event.GetAction()
	pr := event.GetPullRequest()

	h.logger.Info("PR event",
		"action", action,
		"number", pr.GetNumber(),
		"title", pr.GetTitle(),
		"state", pr.GetState(),
	)
}
