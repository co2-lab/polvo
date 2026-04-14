package provider

// isContextWindowError detects context window errors from any LLM provider.
// Inspired by OpenHands multi-provider string matching approach.
func isContextWindowError(err error) bool {
	if err == nil {
		return false
	}
	s := toLower(err.Error())
	patterns := []string{
		"contextwindowexceedederror",
		"prompt is too long",
		"input length and `max_tokens` exceed context limit",
		"maximum context length",
		"context_length_exceeded",
		"reduce the length of the messages",
		"token limit",
	}
	for _, p := range patterns {
		if indexOf(s, p) >= 0 {
			return true
		}
	}
	return false
}
