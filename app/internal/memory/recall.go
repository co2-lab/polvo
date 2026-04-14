package memory

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// RecallConfig controls cross-session memory injection.
type RecallConfig struct {
	Enabled    bool
	MaxEntries int           // default: 5
	MinScore   float64       // reserved for future scoring threshold
	MaxAge     time.Duration // only recall entries newer than this; 0 = no limit
	Types      []string      // entry types to consider; default: ["decision", "context"]
}

// DefaultRecallConfig returns a RecallConfig with sensible defaults.
func DefaultRecallConfig() RecallConfig {
	return RecallConfig{
		Enabled:    false,
		MaxEntries: 5,
		MaxAge:     30 * 24 * time.Hour,
		Types:      []string{"decision", "context"},
	}
}

// Recall retrieves relevant memories from past sessions for the given task description.
// Returns a formatted markdown string ready to prepend to a system prompt.
// Returns empty string if no relevant entries found or cfg.Enabled is false.
func Recall(ctx context.Context, store *Store, taskDesc string, cfg RecallConfig) (string, error) {
	if !cfg.Enabled || taskDesc == "" {
		return "", nil
	}

	types := cfg.Types
	if len(types) == 0 {
		types = []string{"decision", "context"}
	}

	maxEntries := cfg.MaxEntries
	if maxEntries <= 0 {
		maxEntries = 5
	}

	var allEntries []Entry
	for _, t := range types {
		filter := Filter{Type: t, Limit: 200}
		entries, err := store.Read(filter)
		if err != nil {
			return "", fmt.Errorf("reading %s entries: %w", t, err)
		}
		// filter by age
		if cfg.MaxAge > 0 {
			cutoff := time.Now().Add(-cfg.MaxAge).UnixNano()
			for _, e := range entries {
				if e.Timestamp >= cutoff {
					allEntries = append(allEntries, e)
				}
			}
		} else {
			allEntries = append(allEntries, entries...)
		}
	}

	if len(allEntries) == 0 {
		return "", nil
	}

	// Rank with TF-IDF
	ranked := TFIDFSearcher{}.Search(allEntries, taskDesc, maxEntries)
	if len(ranked) == 0 {
		return "", nil
	}

	// Format as markdown bullet list
	var sb strings.Builder
	sb.WriteString("## Relevant context from previous sessions:\n")
	for _, e := range ranked {
		t := time.Unix(0, e.Timestamp).Format("2006-01-02")
		sb.WriteString(fmt.Sprintf("- [%s] %s: %s\n", t, e.Type, e.Content))
	}
	return sb.String(), nil
}
