package session

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Summarizer generates a short one-liner summary for a work item prompt.
// Implementations should be non-blocking (caller runs them in goroutines).
type Summarizer interface {
	Summarize(ctx context.Context, kind Kind, prompt string) (string, error)
}

// SummaryRunner manages async summary generation for work items.
// It fires goroutines when a work item starts and notifies waiters
// only when WaitSummary is called (lazy blocking).
type SummaryRunner struct {
	mgr        *Manager
	summarizer Summarizer

	mu      sync.Mutex
	pending map[string]chan struct{} // id → closed when summary is ready
}

// NewSummaryRunner creates a runner backed by the given manager and summarizer.
func NewSummaryRunner(mgr *Manager, summarizer Summarizer) *SummaryRunner {
	return &SummaryRunner{
		mgr:        mgr,
		summarizer: summarizer,
		pending:    make(map[string]chan struct{}),
	}
}

// StartAsync fires a goroutine to generate and persist a summary for the
// given work item. Safe to call from any goroutine.
func (r *SummaryRunner) StartAsync(ctx context.Context, wi *WorkItem) {
	if r.summarizer == nil {
		return
	}

	ch := make(chan struct{})
	r.mu.Lock()
	r.pending[wi.ID] = ch
	r.mu.Unlock()

	go func() {
		defer close(ch)
		summary, err := r.summarizer.Summarize(ctx, wi.Kind, wi.Prompt)
		if err != nil {
			// non-fatal: leave summary empty
			return
		}
		_ = r.mgr.SetSummary(context.Background(), wi.ID, summary)
	}()
}

// WaitSummary blocks until the summary for id is ready (or ctx is cancelled)
// and returns the summary text. Returns "" without error if id is unknown.
func (r *SummaryRunner) WaitSummary(ctx context.Context, id string) (string, error) {
	r.mu.Lock()
	ch, ok := r.pending[id]
	r.mu.Unlock()

	if ok {
		select {
		case <-ch:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	wi, err := r.mgr.Get(ctx, id)
	if err != nil {
		return "", err
	}
	if wi == nil {
		return "", nil
	}
	return wi.Summary, nil
}

// TruncatingSummarizer wraps a simple one-liner function.
// It truncates prompts longer than maxPromptChars before sending to the LLM.
type TruncatingSummarizer struct {
	MaxPromptChars int
	Fn             func(ctx context.Context, kind Kind, prompt string) (string, error)
}

func (s TruncatingSummarizer) Summarize(ctx context.Context, kind Kind, prompt string) (string, error) {
	max := s.MaxPromptChars
	if max <= 0 {
		max = 500
	}
	if len(prompt) > max {
		prompt = prompt[:max] + "…"
	}
	return s.Fn(ctx, kind, prompt)
}

// NoopSummarizer returns the first line of the prompt trimmed to 80 chars.
// Useful when no LLM summarizer is available.
type NoopSummarizer struct{}

func (NoopSummarizer) Summarize(_ context.Context, _ Kind, prompt string) (string, error) {
	line := strings.SplitN(prompt, "\n", 2)[0]
	if len(line) > 80 {
		line = line[:77] + "…"
	}
	return fmt.Sprintf("[%s]", line), nil
}
