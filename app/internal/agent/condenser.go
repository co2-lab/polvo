package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/co2-lab/polvo/internal/provider"
	"github.com/co2-lab/polvo/internal/repomap"
)

// Condenser reduces conversation history to fit within a token budget.
type Condenser interface {
	Condense(ctx context.Context, messages []provider.Message, maxTokens int) ([]provider.Message, error)
}

// SlidingWindowCondenser keeps the system prompt + last N messages.
type SlidingWindowCondenser struct {
	KeepLast int // number of recent messages to retain (default 20)
}

func (c *SlidingWindowCondenser) Condense(_ context.Context, messages []provider.Message, _ int) ([]provider.Message, error) {
	keep := c.KeepLast
	if keep <= 0 {
		keep = 20
	}
	if len(messages) <= keep+1 {
		return messages, nil
	}
	// Always keep the first message (system/initial context) + last N
	first := messages[:1]
	recent := messages[len(messages)-keep:]
	elided := len(messages) - 1 - keep
	notice := provider.Message{
		Role:    "user",
		Content: fmt.Sprintf("[%d earlier messages omitted to fit context window]", elided),
	}
	result := make([]provider.Message, 0, 1+1+keep)
	result = append(result, first...)
	result = append(result, notice)
	result = append(result, recent...)
	return result, nil
}

// ObservationMaskingCondenser replaces old tool result content with a placeholder
// while keeping tool call structure intact.
type ObservationMaskingCondenser struct {
	KeepRecentResults int // how many tool results to keep verbatim (default 5)
}

func (c *ObservationMaskingCondenser) Condense(_ context.Context, messages []provider.Message, maxTokens int) ([]provider.Message, error) {
	keepRecent := c.KeepRecentResults
	if keepRecent <= 0 {
		keepRecent = 5
	}

	// Count tool result messages from the end
	toolResultCount := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "tool" {
			toolResultCount++
		}
	}

	cutoff := toolResultCount - keepRecent
	seen := 0
	result := make([]provider.Message, len(messages))
	for i, m := range messages {
		if m.Role == "tool" {
			seen++
			if seen <= cutoff && repomap.EstimateTokens(m.Content) > 200 {
				m.Content = "[tool output omitted — " + fmt.Sprintf("%d chars", len(m.Content)) + "]"
			}
		}
		result[i] = m
	}
	return result, nil
}

// LLMSummaryCondenser uses a cheap LLM to summarise old messages into one block.
type LLMSummaryCondenser struct {
	Provider provider.ChatProvider
	Model    string
	KeepLast int // keep last N messages verbatim (default 10)
}

func (c *LLMSummaryCondenser) Condense(ctx context.Context, messages []provider.Message, _ int) ([]provider.Message, error) {
	keep := c.KeepLast
	if keep <= 0 {
		keep = 10
	}
	if len(messages) <= keep+1 {
		return messages, nil
	}

	toSummarise := messages[1 : len(messages)-keep] // skip first (system) and last N
	var sb strings.Builder
	for _, m := range toSummarise {
		fmt.Fprintf(&sb, "[%s]: %s\n", m.Role, m.Content)
	}

	req := provider.ChatRequest{
		Model: c.Model,
		System: "You are a conversation summariser. Produce a concise summary of the following conversation excerpt that preserves all important decisions, findings, and actions taken.",
		Messages: []provider.Message{
			{Role: "user", Content: sb.String()},
		},
		MaxTokens: 1024,
	}

	resp, err := c.Provider.Chat(ctx, req)
	if err != nil {
		// Fall back to sliding window on summary failure
		sw := &SlidingWindowCondenser{KeepLast: keep}
		return sw.Condense(ctx, messages, 0)
	}

	summary := provider.Message{
		Role:    "user",
		Content: "[Summary of earlier conversation]:\n" + resp.Message.Content,
	}

	result := make([]provider.Message, 0, 1+1+keep)
	result = append(result, messages[0]) // system/first
	result = append(result, summary)
	result = append(result, messages[len(messages)-keep:]...)
	return result, nil
}

// maxSummarizeDepth is the maximum recursion depth for SummarizeKeepTail.
const maxSummarizeDepth = 3

// SummarizeKeepTail implements Aider-style recursive summarization.
// It splits messages into head (oldest) + tail (most recent that fit in contextWindow/2),
// summarizes head via summaryLLM (cheap model), and returns [summary_message, ...tail].
// Max recursion depth: maxSummarizeDepth (3).
func SummarizeKeepTail(ctx context.Context, messages []provider.Message, contextWindow int, summaryLLM provider.ChatProvider, model string, depth int) ([]provider.Message, error) {
	if depth >= maxSummarizeDepth {
		return messages, nil
	}
	if len(messages) == 0 {
		return messages, nil
	}

	tailBudget := contextWindow / 2

	// Walk from the end, collecting messages that fit in tailBudget.
	tailStart := len(messages)
	tailTokens := 0
	for i := len(messages) - 1; i >= 0; i-- {
		msgTokens := estimateMessageTokens(messages[i])
		if tailTokens+msgTokens > tailBudget {
			break
		}
		tailTokens += msgTokens
		tailStart = i
	}

	// If everything fits in the tail budget, nothing to summarize.
	if tailStart == 0 {
		return messages, nil
	}

	head := messages[:tailStart]
	tail := messages[tailStart:]

	// Build prompt to summarize head.
	var sb strings.Builder
	for _, m := range head {
		fmt.Fprintf(&sb, "[%s]: %s\n", m.Role, m.Content)
	}

	req := provider.ChatRequest{
		Model:  model,
		System: "You are a conversation summariser. Produce a concise summary of the following conversation excerpt that preserves all important decisions, findings, and actions taken.",
		Messages: []provider.Message{
			{Role: "user", Content: sb.String()},
		},
		MaxTokens: 1024,
	}

	resp, err := summaryLLM.Chat(ctx, req)
	if err != nil {
		// On failure, fall back to sliding window truncation of head.
		summary := provider.Message{
			Role:    "user",
			Content: fmt.Sprintf("[%d earlier messages omitted due to context limit]", len(head)),
		}
		result := make([]provider.Message, 0, 1+len(tail))
		result = append(result, summary)
		result = append(result, tail...)
		return result, nil
	}

	summary := provider.Message{
		Role:    "user",
		Content: "[Summary of earlier conversation]:\n" + resp.Message.Content,
	}

	result := make([]provider.Message, 0, 1+len(tail))
	result = append(result, summary)
	result = append(result, tail...)

	// Check if the result still exceeds contextWindow; recurse if needed.
	if !fitsInBudget(result, contextWindow) {
		return SummarizeKeepTail(ctx, result, contextWindow, summaryLLM, model, depth+1)
	}

	return result, nil
}

// estimateMessageTokens estimates tokens for a single message.
func estimateMessageTokens(m provider.Message) int {
	tokens := 4 // base overhead
	if len(m.Content) > 0 {
		tokens += (len(m.Content) + 3) / 4
	}
	for _, tc := range m.ToolCalls {
		tokens += 10
		if len(tc.Input) > 0 {
			tokens += (len(tc.Input) + 3) / 4
		}
	}
	if m.ToolResult != nil {
		tokens += 10
		if len(m.ToolResult.Content) > 0 {
			tokens += (len(m.ToolResult.Content) + 3) / 4
		}
	}
	return tokens
}

// fitsInBudget checks if messages fit in the given token budget.
func fitsInBudget(messages []provider.Message, budget int) bool {
	total := 0
	for _, m := range messages {
		total += estimateMessageTokens(m)
	}
	return total <= budget
}

// estimateHistoryTokens returns a rough token count for a message slice.
func estimateHistoryTokens(messages []provider.Message) int {
	total := 0
	for _, m := range messages {
		total += repomap.EstimateTokens(m.Content) + 4 // 4 tokens overhead per message
	}
	return total
}

// ApplyCondenser checks if messages exceed the threshold and condenses if needed.
// threshold is a fraction of maxTokens (e.g. 0.85).
func ApplyCondenser(ctx context.Context, messages []provider.Message, maxTokens int, threshold float64, c Condenser) ([]provider.Message, error) {
	limit := int(float64(maxTokens) * threshold)
	if estimateHistoryTokens(messages) <= limit {
		return messages, nil
	}
	return c.Condense(ctx, messages, maxTokens)
}
