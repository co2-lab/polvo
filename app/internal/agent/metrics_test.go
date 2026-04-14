package agent

import (
	"testing"

	"github.com/co2-lab/polvo/internal/provider"
)

func TestAgentMetrics_ToolCallCounting(t *testing.T) {
	m := newAgentMetrics()
	m.recordToolCall("read")
	m.recordToolCall("read")
	m.recordToolCall("bash")

	if m.ToolCallCount["read"] != 2 {
		t.Errorf("expected read count=2, got %d", m.ToolCallCount["read"])
	}
	if m.ToolCallCount["bash"] != 1 {
		t.Errorf("expected bash count=1, got %d", m.ToolCallCount["bash"])
	}
	if m.ToolCallCount["grep"] != 0 {
		t.Errorf("expected grep count=0, got %d", m.ToolCallCount["grep"])
	}
}

func TestAgentMetrics_ContextWindowPressure(t *testing.T) {
	m := newAgentMetrics()
	m.TokensUsed = provider.TokenUsage{TotalTokens: 8192}
	m.computePressure(16384)

	expected := 0.5
	if m.ContextWindowPressure != expected {
		t.Errorf("expected pressure=0.5, got %f", m.ContextWindowPressure)
	}
}

func TestAgentMetrics_ZeroDivisionSafe(t *testing.T) {
	m := newAgentMetrics()
	m.TokensUsed = provider.TokenUsage{TotalTokens: 1000}

	// Should not panic with maxTokens=0
	m.computePressure(0)

	if m.ContextWindowPressure != 0 {
		t.Errorf("expected pressure=0 when maxTokens=0, got %f", m.ContextWindowPressure)
	}
}
