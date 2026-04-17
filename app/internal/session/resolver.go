package session

import (
	"context"
	"regexp"
	"strings"
)

// refPattern matches @@task[task#01] or @@question[question#03] etc.
var refPattern = regexp.MustCompile(`@@(?:task|question)\[([^\]]+)\]`)

// Resolver resolves @@task[id] and @@question[id] references in a prompt,
// replacing them with the corresponding work item summary (blocking if needed).
type Resolver struct {
	mgr     *Manager
	runner  *SummaryRunner
}

// NewResolver creates a resolver backed by the given manager and summary runner.
func NewResolver(mgr *Manager, runner *SummaryRunner) *Resolver {
	return &Resolver{mgr: mgr, runner: runner}
}

// Resolve replaces all @@task[id] / @@question[id] references in prompt
// with their summaries. Blocks until all referenced summaries are ready
// (or ctx is cancelled).
func (r *Resolver) Resolve(ctx context.Context, prompt string) (string, error) {
	matches := refPattern.FindAllStringSubmatchIndex(prompt, -1)
	if len(matches) == 0 {
		return prompt, nil
	}

	// Collect unique IDs to fetch.
	ids := make([]string, 0, len(matches))
	seen := map[string]bool{}
	for _, m := range matches {
		id := prompt[m[2]:m[3]]
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}

	// Fetch (or wait for) each summary.
	summaries := make(map[string]string, len(ids))
	for _, id := range ids {
		var text string
		var err error
		if r.runner != nil {
			text, err = r.runner.WaitSummary(ctx, id)
		} else {
			wi, gerr := r.mgr.Get(ctx, id)
			if gerr != nil {
				err = gerr
			} else if wi != nil {
				text = wi.Summary
			}
		}
		if err != nil {
			return prompt, err
		}
		if text == "" {
			// Fallback: use the ID itself so the model sees something meaningful.
			wi, _ := r.mgr.Get(ctx, id)
			if wi != nil {
				text = wi.Prompt
			}
			if text == "" {
				text = id
			}
		}
		summaries[id] = text
	}

	// Replace references from right to left so indices stay valid.
	var sb strings.Builder
	last := 0
	for _, m := range matches {
		id := prompt[m[2]:m[3]]
		sb.WriteString(prompt[last:m[0]])
		sb.WriteString(summaries[id])
		last = m[1]
	}
	sb.WriteString(prompt[last:])
	return sb.String(), nil
}

// HasRefs reports whether prompt contains any @@task/@@question references.
func HasRefs(prompt string) bool {
	return refPattern.MatchString(prompt)
}

// ListRefs returns all unique IDs referenced in prompt.
func ListRefs(prompt string) []string {
	matches := refPattern.FindAllStringSubmatch(prompt, -1)
	seen := map[string]bool{}
	var out []string
	for _, m := range matches {
		id := m[1]
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
