package microagent

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)


// MatchResult records a microagent that fired and the trigger that caused it.
type MatchResult struct {
	Microagent Microagent
	Trigger    TriggerType
	Score      float64 // 1.0 for deterministic triggers; 0–1 for agent_decision
}

// Match evaluates all microagents against eval and returns those that should be
// injected into the prompt, sorted by Priority descending.
//
// Trigger evaluation layers (cheapest first):
//   1. always       — free, always matches
//   2. keyword      — O(n·m) case-insensitive substring search
//   3. file_match   — filepath.Match against session files
//   4. content_regex — compiled regex against FileContents (lazy)
//   5. agent_decision — not implemented; skipped (returns no results for this type)
//   6. manual       — never auto-triggered
func Match(agents []Microagent, eval EvalContext) []MatchResult {
	var results []MatchResult
	lowerMsg := strings.ToLower(eval.UserMessage)

	for _, ma := range agents {
		if result, ok := matchAgent(ma, eval, lowerMsg); ok {
			results = append(results, result)
		}
	}

	// Sort by priority descending so callers can simply take the first N.
	sort.Slice(results, func(i, j int) bool {
		return results[i].Microagent.Priority > results[j].Microagent.Priority
	})

	return results
}

// matchAgent checks each trigger for a single microagent, returning the first
// that fires. The ordering follows the cost ladder defined in Match.
func matchAgent(ma Microagent, eval EvalContext, lowerMsg string) (MatchResult, bool) {
	// Layer 1 — always
	for _, t := range ma.Triggers {
		if t.Type == TriggerAlways {
			return MatchResult{Microagent: ma, Trigger: TriggerAlways, Score: 1.0}, true
		}
	}

	// Layer 2 — keyword
	for _, t := range ma.Triggers {
		if t.Type != TriggerKeyword {
			continue
		}
		for _, word := range t.Words {
			if strings.Contains(lowerMsg, strings.ToLower(word)) {
				return MatchResult{Microagent: ma, Trigger: TriggerKeyword, Score: 1.0}, true
			}
		}
	}

	// Layer 3 — file_match
	for _, t := range ma.Triggers {
		if t.Type != TriggerFileMatch {
			continue
		}
		if matchesFilePatterns(eval.SessionFiles, t.Patterns, t.Exclude) {
			return MatchResult{Microagent: ma, Trigger: TriggerFileMatch, Score: 1.0}, true
		}
	}

	// Layer 4 — content_regex
	for _, t := range ma.Triggers {
		if t.Type != TriggerContentRegex {
			continue
		}
		if matchesContentRegex(eval.FileContents, t.Patterns) {
			return MatchResult{Microagent: ma, Trigger: TriggerContentRegex, Score: 1.0}, true
		}
	}

	// Layer 5 — agent_decision: not implemented, skipped intentionally.
	// Layer 6 — manual: never auto-triggered.

	return MatchResult{}, false
}

// matchesFilePatterns returns true when any session file matches at least one
// include pattern and does not match any exclude pattern.
func matchesFilePatterns(sessionFiles, patterns, exclude []string) bool {
	for _, f := range sessionFiles {
		if fileMatchesAny(f, patterns) && !fileMatchesAny(f, exclude) {
			return true
		}
	}
	return false
}

// fileMatchesAny returns true when path matches any of the given glob patterns.
// An empty patterns list is treated as "no match".
// Supports "**" as a recursive path wildcard (e.g. "**/redis/**").
func fileMatchesAny(path string, patterns []string) bool {
	for _, pat := range patterns {
		if globMatch(pat, path) {
			return true
		}
		// Also try matching against the base name for simple patterns.
		if globMatch(pat, filepath.Base(path)) {
			return true
		}
	}
	return false
}

// globMatch implements glob matching with "**" support.
// "**" matches zero or more path segments (including none and including "/").
func globMatch(pattern, name string) bool {
	if !strings.Contains(pattern, "**") {
		ok, _ := filepath.Match(pattern, name)
		return ok
	}
	// Convert the ** pattern to a regex for correct recursive matching.
	re := globPatternToRegex(pattern)
	return re.MatchString(name)
}

// globPatternToRegex converts a glob pattern (with ** support) to a compiled
// *regexp.Regexp. The resulting regex matches the whole string.
func globPatternToRegex(pattern string) *regexp.Regexp {
	var sb strings.Builder
	sb.WriteByte('^')
	i := 0
	for i < len(pattern) {
		if pattern[i] == '*' && i+1 < len(pattern) && pattern[i+1] == '*' {
			// "**" matches any sequence of characters including path separators.
			sb.WriteString(".*")
			i += 2
			// Skip a trailing slash after "**" if present so "**/foo" matches "foo" at root.
			if i < len(pattern) && pattern[i] == '/' {
				sb.WriteString("/?")
				i++
			}
			continue
		}
		switch c := pattern[i]; c {
		case '*':
			// Single "*" matches anything except a path separator.
			sb.WriteString("[^/]*")
		case '?':
			sb.WriteString("[^/]")
		case '.', '+', '(', ')', '{', '}', '[', ']', '^', '$', '|', '\\':
			sb.WriteByte('\\')
			sb.WriteByte(c)
		default:
			sb.WriteByte(c)
		}
		i++
	}
	sb.WriteByte('$')
	re, err := regexp.Compile(sb.String())
	if err != nil {
		// Malformed pattern — return a regex that never matches.
		return regexp.MustCompile(`(?:^$){2}`) // impossible pattern
	}
	return re
}

// matchesContentRegex returns true when any file content in fileContents
// matches at least one of the provided regex patterns.
func matchesContentRegex(fileContents map[string]string, patterns []string) bool {
	if len(fileContents) == 0 || len(patterns) == 0 {
		return false
	}
	for _, pat := range patterns {
		re, err := regexp.Compile(pat)
		if err != nil {
			// Invalid regex — skip this pattern rather than fail-open.
			continue
		}
		for _, content := range fileContents {
			if re.MatchString(content) {
				return true
			}
		}
	}
	return false
}
