package agent

import (
	"context"
	"fmt"
)

const (
	DecisionApprove = "APPROVE"
	DecisionReject  = "REJECT"
)

// CriticConfig configures the critic (adversarial review) pattern.
type CriticConfig struct {
	// Author produces the output to be reviewed.
	Author Agent
	// Critic reviews the author's output.
	Critic Agent
	// MaxRetries is the maximum number of author retries after rejection (default 2).
	MaxRetries int
	// Exec is used to run both agents.
	Exec *Executor
	// Bus, when non-nil, receives Author/Critic result messages.
	Bus *AgentBus
}

// CriticResult is the outcome of a critic-reviewed execution.
type CriticResult struct {
	AuthorResult *Result
	CriticResult *Result
	Approved     bool
	Retries      int
}

// RunCritic executes the author/critic adversarial loop.
// The author produces output; the critic reviews it. On rejection, the author
// is re-run with the critic's feedback injected as a finding up to MaxRetries times.
// Returns CriticResult with the final author output and whether it was approved.
func RunCritic(ctx context.Context, cfg CriticConfig, input *Input) (*CriticResult, error) {
	if cfg.Author == nil {
		return nil, fmt.Errorf("critic: Author agent is required")
	}
	if cfg.Critic == nil {
		return nil, fmt.Errorf("critic: Critic agent is required")
	}
	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 2
	}

	var authorResult *Result
	var criticResult *Result
	currentInput := input

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Author produces output.
		ar, err := cfg.Author.Execute(ctx, currentInput)
		if err != nil {
			return nil, fmt.Errorf("critic: author execution failed (attempt %d): %w", attempt+1, err)
		}
		authorResult = ar

		if cfg.Bus != nil {
			cfg.Bus.Publish(AgentMessage{
				From:    cfg.Author.Name(),
				To:      cfg.Critic.Name(),
				Type:    MessageResult,
				Payload: ar.Summary,
			})
		}

		// Critic reviews the author's output.
		reviewInput := &Input{
			Content:         ar.Content,
			Spec:            currentInput.Spec,
			Feature:         currentInput.Feature,
			PreviousReports: currentInput.PreviousReports,
			Diff:            ar.Summary,
		}
		cr, err := cfg.Critic.Execute(ctx, reviewInput)
		if err != nil {
			return nil, fmt.Errorf("critic: critic execution failed (attempt %d): %w", attempt+1, err)
		}
		criticResult = cr

		if cfg.Bus != nil {
			cfg.Bus.Publish(AgentMessage{
				From:    cfg.Critic.Name(),
				To:      cfg.Author.Name(),
				Type:    MessageDirective,
				Payload: cr.Summary,
			})
		}

		// Approved — done.
		if cr.Decision == DecisionApprove || cr.Decision == "" {
			return &CriticResult{
				AuthorResult: authorResult,
				CriticResult: criticResult,
				Approved:     true,
				Retries:      attempt,
			}, nil
		}

		// Rejected: inject critic feedback for next author attempt.
		if attempt < maxRetries {
			feedback := buildFeedback(cr)
			currentInput = &Input{
				File:            input.File,
				Content:         ar.Content,
				Spec:            input.Spec,
				Feature:         input.Feature,
				Diff:            input.Diff,
				Event:           input.Event,
				ProjectRoot:     input.ProjectRoot,
				Interface:       input.Interface,
				PRDiff:          input.PRDiff,
				PRComments:      input.PRComments,
				PreviousReports: feedback,
				FileHistory:     input.FileHistory,
			}
		}
	}

	// Exhausted retries — return with Approved=false.
	return &CriticResult{
		AuthorResult: authorResult,
		CriticResult: criticResult,
		Approved:     false,
		Retries:      maxRetries,
	}, nil
}

// buildFeedback formats critic findings as a previous-reports string for the author.
func buildFeedback(cr *Result) string {
	if cr == nil {
		return ""
	}
	if len(cr.Findings) == 0 {
		return cr.Summary
	}
	out := "Critic rejected. Issues found:\n"
	for _, f := range cr.Findings {
		out += fmt.Sprintf("- [%s] %s: %s", f.Severity, f.Category, f.Message)
		if f.Suggestion != "" {
			out += " Suggestion: " + f.Suggestion
		}
		out += "\n"
	}
	return out
}
