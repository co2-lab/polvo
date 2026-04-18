package provider

import (
	"context"
	"errors"
	"log/slog"
)

// ErrContextWindowExhausted is returned when all context fallback layers
// are exhausted without fitting within the context window.
var ErrContextWindowExhausted = errors.New("context window exhausted after all fallback layers")

// Summarizer summarizes a message history using a cheap LLM, keeping the
// recent tail intact. The interface is satisfied by agent.SummarizeKeepTail
// via an adapter, or by any function with this signature.
type Summarizer interface {
	Summarize(ctx context.Context, messages []Message, contextWindow int) ([]Message, error)
}

// ContextFallbackManager applies a transparent cascade when a ChatRequest
// exceeds the model's context window:
//
//  1. Larger model (if ContextWindowFallbacks configured for the current model)
//  2. Summarize history (if Summarizer set)
//  3. Prune messages (always available as last resort)
//
// The caller never needs to handle context overflow — the manager resolves it.
// Each layer activation is logged via slog.
type ContextFallbackManager struct {
	// ContextWindowFallbacks maps model names to ordered fallback model lists.
	// Ex: {"claude-haiku-4-5": ["claude-sonnet-4-6", "claude-opus-4-6"]}
	ContextWindowFallbacks map[string][]string
	// MaxDepth is the maximum model-upgrade hops (default 3).
	MaxDepth int
	// MinOutputTokens is the minimum token budget reserved for output (default 1000).
	MinOutputTokens int
	// Summarizer is an optional cheap-model summarizer (layer 2).
	// When nil, layer 2 is skipped.
	Summarizer Summarizer
}

// WrapChat wraps a Chat call with context window overflow handling.
// provider is the ChatProvider to use, model is the current model name,
// contextWindow is the model's context window size (0 = skip pre-call check).
func (m *ContextFallbackManager) WrapChat(
	ctx context.Context,
	p ChatProvider,
	req ChatRequest,
	contextWindow int,
	depth int,
) (*ChatResponse, error) {
	if depth > m.maxDepth() {
		return nil, ErrContextWindowExhausted
	}

	minOut := m.minOutput()

	// Pre-call check: if messages already exceed limit, go straight to overflow.
	if contextWindow > 0 && !FitsInContext(req.Messages, contextWindow, minOut) {
		return m.handleOverflow(ctx, p, req, contextWindow, depth)
	}

	resp, err := p.Chat(ctx, req)
	if err != nil && isContextWindowError(err) {
		return m.handleOverflow(ctx, p, req, contextWindow, depth)
	}
	return resp, err
}

func (m *ContextFallbackManager) handleOverflow(
	ctx context.Context,
	p ChatProvider,
	req ChatRequest,
	contextWindow int,
	depth int,
) (*ChatResponse, error) {
	if depth >= m.maxDepth() {
		return nil, ErrContextWindowExhausted
	}

	// Layer 1: larger model via ContextWindowFallbacks.
	if next := m.nextLargerModel(req.Model); next != "" {
		slog.Info("context_window_fallback",
			"from_model", req.Model,
			"to_model", next,
			"depth", depth,
		)
		req.Model = next
		return m.WrapChat(ctx, p, req, contextWindow, depth+1)
	}

	// Layer 2: summarize history (if summarizer available).
	if m.Summarizer != nil && contextWindow > 0 {
		summarized, err := m.Summarizer.Summarize(ctx, req.Messages, contextWindow)
		if err == nil && FitsInContext(summarized, contextWindow, m.minOutput()) {
			slog.Info("context_window_summarized",
				"original_messages", len(req.Messages),
				"after_messages", len(summarized),
				"removed", len(req.Messages)-len(summarized),
			)
			req.Messages = summarized
			return m.WrapChat(ctx, p, req, contextWindow, depth+1)
		}
		// Summarization reduced size but still doesn't fit — use as input for pruning.
		if err == nil && len(summarized) < len(req.Messages) {
			req.Messages = summarized
		}
	}

	// Layer 3: prune messages (last resort).
	if contextWindow > 0 {
		pruned := PruneMessages(req.Messages, contextWindow, m.minOutput())
		slog.Warn("context_window_pruned",
			"original_messages", len(req.Messages),
			"after_messages", len(pruned),
			"removed", len(req.Messages)-len(pruned),
		)
		req.Messages = pruned
	}

	// Use WrapChat for the final attempt so any context error is still caught.
	return m.WrapChat(ctx, p, req, contextWindow, depth+1)
}

func (m *ContextFallbackManager) nextLargerModel(model string) string {
	if m.ContextWindowFallbacks == nil {
		return ""
	}
	list := m.ContextWindowFallbacks[model]
	if len(list) == 0 {
		return ""
	}
	return list[0]
}

func (m *ContextFallbackManager) maxDepth() int {
	if m.MaxDepth <= 0 {
		return 3
	}
	return m.MaxDepth
}

func (m *ContextFallbackManager) minOutput() int {
	if m.MinOutputTokens <= 0 {
		return 1000
	}
	return m.MinOutputTokens
}
