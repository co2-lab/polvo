package microagent

import "strings"

const defaultMaxInjected = 5

// Inject formats the top-N matched microagents as a prompt section and returns
// the resulting string. Returns an empty string when matches is empty.
//
// Results are expected to arrive pre-sorted by priority descending (as returned
// by Match). If len(matches) > maxInjected, only the first maxInjected entries
// are included.
//
// Output format:
//
//	[Conhecimento Especializado Ativo]
//	--- name ---
//	content
//	--- name2 ---
//	content2
func Inject(matches []MatchResult, maxInjected int) string {
	return InjectWithBudget(matches, maxInjected, 0)
}

// InjectWithBudget is like Inject but additionally enforces a character budget.
// When maxChars > 0, microagents are added one by one in priority order; if
// adding the next microagent would cause the accumulated output to exceed
// maxChars, that entry (and all subsequent ones) are skipped. Microagents are
// never truncated mid-entry.
//
// Pass maxChars <= 0 to disable the character budget (identical to Inject).
func InjectWithBudget(matches []MatchResult, maxInjected int, maxChars int) string {
	if len(matches) == 0 {
		return ""
	}
	if maxInjected <= 0 {
		maxInjected = defaultMaxInjected
	}
	if len(matches) > maxInjected {
		matches = matches[:maxInjected]
	}

	const header = "[Conhecimento Especializado Ativo]\n"

	var sb strings.Builder
	sb.WriteString(header)

	for _, m := range matches {
		entry := "--- " + m.Microagent.Name + " ---\n" + m.Microagent.Content + "\n"
		if maxChars > 0 && sb.Len()+len(entry) > maxChars {
			continue
		}
		sb.WriteString(entry)
	}

	// If nothing was injected (budget too small for even the header's successors),
	// return just the header that was already written only when at least one entry
	// was added; otherwise return empty to stay consistent with the no-match case.
	if sb.Len() == len(header) {
		return ""
	}

	return sb.String()
}
