package memory

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRecall_ReturnsFormattedOutput(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()

	store.Write(Entry{Agent: "ag", Type: "decision", Content: "use kubernetes for deployment"})
	store.Write(Entry{Agent: "ag", Type: "context", Content: "project uses microservices architecture"})

	cfg := RecallConfig{
		Enabled:    true,
		MaxEntries: 5,
		Types:      []string{"decision", "context"},
	}

	result, err := Recall(ctx, store, "kubernetes deployment strategy", cfg)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty recall output")
	}
	if !strings.HasPrefix(result, "## Relevant context from previous sessions:\n") {
		t.Errorf("unexpected format: %q", result)
	}
	if !strings.Contains(result, "kubernetes") {
		t.Errorf("expected 'kubernetes' in result, got: %q", result)
	}
}

func TestRecall_DisabledReturnsEmpty(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()

	store.Write(Entry{Agent: "ag", Type: "decision", Content: "use kubernetes for deployment"})

	cfg := RecallConfig{
		Enabled: false,
		Types:   []string{"decision"},
	}

	result, err := Recall(ctx, store, "kubernetes", cfg)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty result when disabled, got: %q", result)
	}
}

func TestRecall_RespectsMaxAge(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()

	// Old entry: 60 days ago
	old := time.Now().Add(-60 * 24 * time.Hour).UnixNano()
	store.Write(Entry{
		Agent:     "ag",
		Type:      "decision",
		Content:   "use kubernetes for deployment",
		Timestamp: old,
	})

	// Recent entry: 1 day ago
	recent := time.Now().Add(-1 * 24 * time.Hour).UnixNano()
	store.Write(Entry{
		Agent:     "ag",
		Type:      "decision",
		Content:   "switched to docker compose",
		Timestamp: recent,
	})

	cfg := RecallConfig{
		Enabled:    true,
		MaxEntries: 10,
		MaxAge:     30 * 24 * time.Hour, // only last 30 days
		Types:      []string{"decision"},
	}

	result, err := Recall(ctx, store, "kubernetes deployment", cfg)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	// Old entry (60 days old) should be filtered out.
	if strings.Contains(result, "kubernetes") {
		t.Errorf("expected old entry to be filtered by MaxAge, but found 'kubernetes' in result: %q", result)
	}
}

func TestRecall_RespectsMaxEntries(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		store.Write(Entry{
			Agent:   "ag",
			Type:    "decision",
			Content: "use golang for backend services",
		})
	}

	cfg := RecallConfig{
		Enabled:    true,
		MaxEntries: 3,
		Types:      []string{"decision"},
	}

	result, err := Recall(ctx, store, "golang backend", cfg)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}

	// Count bullet points in result.
	lines := strings.Split(strings.TrimSpace(result), "\n")
	bulletCount := 0
	for _, l := range lines {
		if strings.HasPrefix(l, "- ") {
			bulletCount++
		}
	}
	if bulletCount > 3 {
		t.Errorf("expected at most 3 entries, got %d", bulletCount)
	}
}

func TestRecall_EmptyStoreReturnsEmpty(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()

	cfg := RecallConfig{
		Enabled:    true,
		MaxEntries: 5,
		Types:      []string{"decision", "context"},
	}

	result, err := Recall(ctx, store, "any task description", cfg)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty result for empty store, got: %q", result)
	}
}
