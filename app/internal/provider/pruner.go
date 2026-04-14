package provider

import "strings"

// isSticky returns true if a message should never be removed during pruning.
// A message is sticky if its content contains "<!-- sticky -->" or if it
// comes from a microagent injection (indicated by "<!-- microagent -->").
func isSticky(m Message) bool {
	return strings.Contains(m.Content, "<!-- sticky -->") ||
		strings.Contains(m.Content, "<!-- microagent -->")
}

// PruneMessages removes messages until the estimated token count fits within
// contextWindow minus safetyBuffer minus minOutputTokens.
//
// Removal priority (removed first): oldest non-system, non-sticky messages.
// NEVER removes: system messages (role=="system"), the last user message,
// sticky messages, or individual halves of a tool_call/tool_result pair.
// Tool calls and their results are always removed together.
func PruneMessages(messages []Message, contextWindow, minOutputTokens int) []Message {
	if FitsInContext(messages, contextWindow, minOutputTokens) {
		return messages
	}

	// Build a working copy.
	work := make([]Message, len(messages))
	copy(work, messages)

	// Identify the last user message index (must never be removed).
	lastUserIdx := -1
	for i := len(work) - 1; i >= 0; i-- {
		if work[i].Role == "user" {
			lastUserIdx = i
			break
		}
	}

	// Build tool-call to tool-result pairing.
	// For each assistant message with tool calls, find the subsequent tool
	// result messages that share the same tool call IDs.
	// We store pairs as (callIdx, resultIdx) for removal together.
	type pair struct{ callIdx, resultIdx int }
	var pairs []pair
	for i := 0; i < len(work); i++ {
		if work[i].Role == "assistant" && len(work[i].ToolCalls) > 0 {
			// Find the matching tool result immediately after.
			for j := i + 1; j < len(work); j++ {
				if work[j].Role == "tool" && work[j].ToolResult != nil {
					pairs = append(pairs, pair{i, j})
					break
				}
			}
		}
	}

	// Iteratively remove the oldest removable message/pair until it fits.
	for !FitsInContext(work, contextWindow, minOutputTokens) {
		removed := false

		// Try to find the oldest removable index from the front.
		for i := 0; i < len(work); i++ {
			m := work[i]

			// Never remove system messages.
			if m.Role == "system" {
				continue
			}
			// Never remove sticky messages.
			if isSticky(m) {
				continue
			}
			// Never remove the last user message.
			if i == lastUserIdx {
				continue
			}

			// Check if this is part of a tool pair.
			pairIdx := -1
			for pi, p := range pairs {
				if p.callIdx == i || p.resultIdx == i {
					pairIdx = pi
					break
				}
			}

			if pairIdx >= 0 {
				// Remove both the tool call and its result together.
				p := pairs[pairIdx]
				// Remove higher index first to preserve lower index validity.
				higher := p.resultIdx
				lower := p.callIdx
				if p.callIdx > p.resultIdx {
					higher = p.callIdx
					lower = p.resultIdx
				}
				work = append(work[:higher], work[higher+1:]...)
				work = append(work[:lower], work[lower+1:]...)
				// Remove pair entry.
				pairs = append(pairs[:pairIdx], pairs[pairIdx+1:]...)
				// Rebuild last user index and pair indices after removal.
				lastUserIdx = -1
				for idx := len(work) - 1; idx >= 0; idx-- {
					if work[idx].Role == "user" {
						lastUserIdx = idx
						break
					}
				}
				// Adjust remaining pair indices.
				for pi := range pairs {
					if pairs[pi].callIdx > higher {
						pairs[pi].callIdx--
					}
					if pairs[pi].resultIdx > higher {
						pairs[pi].resultIdx--
					}
					if pairs[pi].callIdx > lower {
						pairs[pi].callIdx--
					}
					if pairs[pi].resultIdx > lower {
						pairs[pi].resultIdx--
					}
				}
				removed = true
				break
			}

			// Not part of a pair — remove directly.
			work = append(work[:i], work[i+1:]...)
			// Rebuild last user index.
			lastUserIdx = -1
			for idx := len(work) - 1; idx >= 0; idx-- {
				if work[idx].Role == "user" {
					lastUserIdx = idx
					break
				}
			}
			// Adjust pair indices.
			for pi := range pairs {
				if pairs[pi].callIdx > i {
					pairs[pi].callIdx--
				}
				if pairs[pi].resultIdx > i {
					pairs[pi].resultIdx--
				}
			}
			removed = true
			break
		}

		// If we couldn't remove anything more, stop to avoid infinite loop.
		if !removed {
			break
		}
	}

	return work
}
