package agent

import (
	"time"

	"github.com/co2-lab/polvo/internal/provider"
)

// AgentMetrics captures performance and cost data for a single agent loop run.
type AgentMetrics struct {
	TurnCount             int
	TokensUsed            provider.TokenUsage
	CostUSD               float64
	ToolCallCount         map[string]int
	StuckCount            int
	ReflectionCount       int
	Duration              time.Duration
	ContextWindowPressure float64 // tokens_used / max_tokens (0–1)
}

func newAgentMetrics() *AgentMetrics {
	return &AgentMetrics{ToolCallCount: make(map[string]int)}
}

func (m *AgentMetrics) recordToolCall(name string) {
	m.ToolCallCount[name]++
}

func (m *AgentMetrics) computePressure(maxTokens int) {
	if maxTokens > 0 {
		m.ContextWindowPressure = float64(m.TokensUsed.TotalTokens) / float64(maxTokens)
	}
}
