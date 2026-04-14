package provider

// EstimateTokens estimates the total token count for a slice of messages.
// Uses character-based approximation: (len(text)+3)/4 chars per token.
// Per message overhead: +4 tokens base.
// Per tool call: +10 tokens.
// Per tool result: +10 tokens.
func EstimateTokens(messages []Message, _ string) int {
	total := 0
	for _, m := range messages {
		// Base overhead per message
		total += 4
		// Content tokens
		if len(m.Content) > 0 {
			total += (len(m.Content) + 3) / 4
		}
		// Tool calls overhead
		for _, tc := range m.ToolCalls {
			total += 10
			if len(tc.Input) > 0 {
				total += (len(tc.Input) + 3) / 4
			}
		}
		// Tool result overhead
		if m.ToolResult != nil {
			total += 10
			if len(m.ToolResult.Content) > 0 {
				total += (len(m.ToolResult.Content) + 3) / 4
			}
		}
	}
	return total
}

// SafetyBuffer returns the safety buffer to subtract from the context window.
// Returns max(1000, contextWindow * 2 / 100) using integer math.
func SafetyBuffer(contextWindow int) int {
	buf := contextWindow * 2 / 100
	if buf < 1000 {
		return 1000
	}
	return buf
}

// FitsInContext returns true if the estimated token count of messages fits
// within contextWindow minus safety buffer minus minOutputTokens.
func FitsInContext(messages []Message, contextWindow, minOutputTokens int) bool {
	estimated := EstimateTokens(messages, "")
	available := contextWindow - SafetyBuffer(contextWindow) - minOutputTokens
	return estimated <= available
}
