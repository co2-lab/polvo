package provider

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEstimateTokens(t *testing.T) {
	t.Run("empty messages → 0", func(t *testing.T) {
		if got := EstimateTokens(nil, ""); got != 0 {
			t.Errorf("got %d; want 0", got)
		}
	})

	t.Run("single message with 4-char content → 4+1=5", func(t *testing.T) {
		msgs := []Message{{Role: "user", Content: "abcd"}}
		// content: (4+3)/4 = 1, overhead: 4 → total: 5
		if got := EstimateTokens(msgs, ""); got != 5 {
			t.Errorf("got %d; want 5", got)
		}
	})

	t.Run("empty content message → 4 (base overhead only)", func(t *testing.T) {
		msgs := []Message{{Role: "user", Content: ""}}
		if got := EstimateTokens(msgs, ""); got != 4 {
			t.Errorf("got %d; want 4", got)
		}
	})

	t.Run("message with tool calls adds 10 per tool call", func(t *testing.T) {
		msgs := []Message{
			{
				Role:    "assistant",
				Content: "",
				ToolCalls: []ToolCall{
					{ID: "1", Name: "bash", Input: json.RawMessage(`{}`)},
				},
			},
		}
		// base: 4, tool call: 10, input "{}": (2+3)/4=1 → total: 15
		got := EstimateTokens(msgs, "")
		if got != 15 {
			t.Errorf("got %d; want 15", got)
		}
	})

	t.Run("message with tool result adds 10", func(t *testing.T) {
		msgs := []Message{
			{
				Role: "tool",
				ToolResult: &ToolResult{
					ToolCallID: "1",
					Content:    "",
				},
			},
		}
		// base: 4, tool result: 10 → total: 14
		got := EstimateTokens(msgs, "")
		if got != 14 {
			t.Errorf("got %d; want 14", got)
		}
	})

	t.Run("model param ignored (char-based estimation)", func(t *testing.T) {
		msgs := []Message{{Role: "user", Content: strings.Repeat("a", 40)}}
		got1 := EstimateTokens(msgs, "gpt-4")
		got2 := EstimateTokens(msgs, "claude-3-opus")
		if got1 != got2 {
			t.Errorf("model should not affect estimation: %d vs %d", got1, got2)
		}
	})
}

func TestSafetyBuffer(t *testing.T) {
	t.Run("small context window returns 1000", func(t *testing.T) {
		if got := SafetyBuffer(1000); got != 1000 {
			t.Errorf("got %d; want 1000", got)
		}
	})

	t.Run("window=50000 → 2% = 1000", func(t *testing.T) {
		got := SafetyBuffer(50000)
		if got != 1000 {
			t.Errorf("got %d; want 1000", got)
		}
	})

	t.Run("large context window returns 2% of window", func(t *testing.T) {
		// 200000 * 2 / 100 = 4000
		got := SafetyBuffer(200000)
		if got != 4000 {
			t.Errorf("got %d; want 4000", got)
		}
	})

	t.Run("boundary: 50001 → 2% = 1000 (rounds down)", func(t *testing.T) {
		// 50001 * 2 / 100 = 1000 (integer division)
		got := SafetyBuffer(50001)
		if got != 1000 {
			t.Errorf("got %d; want 1000", got)
		}
	})
}

func TestFitsInContext(t *testing.T) {
	t.Run("small messages fit in large context", func(t *testing.T) {
		msgs := []Message{{Role: "user", Content: "hello"}}
		if !FitsInContext(msgs, 100000, 1000) {
			t.Error("expected small messages to fit")
		}
	})

	t.Run("messages that exceed context do not fit", func(t *testing.T) {
		// Create a message with very long content
		msgs := []Message{{Role: "user", Content: strings.Repeat("a", 400000)}}
		// EstimateTokens = (400000+3)/4 = 100000 + base 4 = 100004
		// contextWindow=10000, buffer=1000, minOutput=1000 → available=8000
		// 100004 > 8000
		if FitsInContext(msgs, 10000, 1000) {
			t.Error("expected large messages to not fit")
		}
	})

	t.Run("exactly at limit does not fit (no safety buffer)", func(t *testing.T) {
		// contextWindow=1000, buffer=max(1000,20)=1000, minOutput=0 → available=0
		msgs := []Message{{Role: "user", Content: "a"}}
		// Any non-zero content won't fit in available=0
		if FitsInContext(msgs, 1000, 0) {
			t.Error("expected to not fit when available is 0")
		}
	})
}
