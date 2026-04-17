package tool

import "fmt"

// DefaultMaxObservationChars is the default maximum character count for tool
// observation output before truncation. Matches OpenHands LLMConfig.max_message_chars.
const DefaultMaxObservationChars = 30_000

// TruncateObservation truncates long tool output keeping head and tail halves,
// with a marker showing how many chars were removed. This prevents a single
// tool result from overwhelming the LLM context window.
//
// If maxChars <= 0, DefaultMaxObservationChars is used.
// Content within the limit is returned unchanged.
func TruncateObservation(content string, maxChars int) string {
	if maxChars <= 0 {
		maxChars = DefaultMaxObservationChars
	}
	if len(content) <= maxChars {
		return content
	}
	half := maxChars / 2
	removed := len(content) - maxChars
	marker := fmt.Sprintf("\n\n[... %d chars truncados ...]\n\n", removed)
	return content[:half] + marker + content[len(content)-half:]
}
