package memory

import (
	"testing"
	"time"
)

func TestTopN_OrdersByScore(t *testing.T) {
	store := openStore(t)

	// Write 3 entries for the same agent; read them different numbers of times.
	entries := []Entry{
		{Agent: "ag", Type: "observation", Content: "low", ExpiresAt: 0},
		{Agent: "ag", Type: "observation", Content: "mid", ExpiresAt: 0},
		{Agent: "ag", Type: "observation", Content: "high", ExpiresAt: 0},
	}
	for _, e := range entries {
		if err := store.Write(e); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	// Read to bump usage counts: "high" gets 5 reads, "mid" gets 2, "low" gets 1.
	readContent := func(content string, times int) {
		for i := 0; i < times; i++ {
			_, _ = store.Read(Filter{Agent: "ag", Type: "observation"})
		}
	}
	_ = readContent // all reads go through the same query — bump all counts together

	// Directly bump usage_count via writes at different times to simulate usage.
	// For a proper test, we directly update usage_count in the DB.
	_, err := store.db.Exec(`UPDATE memory SET usage_count = 5 WHERE content = 'high'`)
	if err != nil {
		t.Fatalf("bump high: %v", err)
	}
	_, err = store.db.Exec(`UPDATE memory SET usage_count = 2 WHERE content = 'mid'`)
	if err != nil {
		t.Fatalf("bump mid: %v", err)
	}
	_, err = store.db.Exec(`UPDATE memory SET usage_count = 1 WHERE content = 'low'`)
	if err != nil {
		t.Fatalf("bump low: %v", err)
	}

	top, err := store.TopN("ag", "observation", 3)
	if err != nil {
		t.Fatalf("TopN: %v", err)
	}
	if len(top) != 3 {
		t.Fatalf("expected 3, got %d", len(top))
	}
	if top[0].Content != "high" {
		t.Errorf("expected 'high' first, got %q", top[0].Content)
	}
	if top[1].Content != "mid" {
		t.Errorf("expected 'mid' second, got %q", top[1].Content)
	}
}

func TestTopN_LimitsResults(t *testing.T) {
	store := openStore(t)

	for i := 0; i < 5; i++ {
		if err := store.Write(Entry{Agent: "ag", Type: "decision", Content: "d"}); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	top, err := store.TopN("ag", "decision", 2)
	if err != nil {
		t.Fatalf("TopN: %v", err)
	}
	if len(top) != 2 {
		t.Errorf("expected 2, got %d", len(top))
	}
}

func TestTopN_ExcludesExpired(t *testing.T) {
	store := openStore(t)

	past := time.Now().Add(-1 * time.Hour).UnixNano()
	if err := store.Write(Entry{Agent: "ag", Type: "observation", Content: "expired", ExpiresAt: past}); err != nil {
		t.Fatalf("Write expired: %v", err)
	}
	if err := store.Write(Entry{Agent: "ag", Type: "observation", Content: "live"}); err != nil {
		t.Fatalf("Write live: %v", err)
	}

	top, err := store.TopN("ag", "observation", 10)
	if err != nil {
		t.Fatalf("TopN: %v", err)
	}
	for _, e := range top {
		if e.Content == "expired" {
			t.Errorf("TopN should not return expired entries")
		}
	}
	if len(top) != 1 {
		t.Errorf("expected 1 live entry, got %d", len(top))
	}
}

func TestTopN_AllTypesWhenTypeEmpty(t *testing.T) {
	store := openStore(t)

	for _, typ := range []string{"observation", "decision", "issue"} {
		if err := store.Write(Entry{Agent: "ag2", Type: typ, Content: typ}); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	top, err := store.TopN("ag2", "", 10)
	if err != nil {
		t.Fatalf("TopN: %v", err)
	}
	if len(top) != 3 {
		t.Errorf("expected 3 (all types), got %d", len(top))
	}
}

func TestUsageCount_IncrementedOnRead(t *testing.T) {
	store := openStore(t)

	if err := store.Write(Entry{Agent: "ag", Type: "observation", Content: "watched"}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Read twice
	for i := 0; i < 2; i++ {
		_, err := store.Read(Filter{Agent: "ag"})
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
	}

	// Check usage_count directly
	var count int
	if err := store.db.QueryRow(`SELECT usage_count FROM memory WHERE content = 'watched'`).Scan(&count); err != nil {
		t.Fatalf("QueryRow: %v", err)
	}
	if count != 2 {
		t.Errorf("expected usage_count=2, got %d", count)
	}
}

func TestTopN_ScoreSetOnReturn(t *testing.T) {
	store := openStore(t)

	if err := store.Write(Entry{Agent: "ag", Type: "observation", Content: "score-test"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := store.db.Exec(`UPDATE memory SET usage_count = 10 WHERE content = 'score-test'`); err != nil {
		t.Fatalf("bump: %v", err)
	}

	top, err := store.TopN("ag", "observation", 1)
	if err != nil {
		t.Fatalf("TopN: %v", err)
	}
	if len(top) != 1 {
		t.Fatalf("expected 1, got %d", len(top))
	}
	if top[0].Score <= 0 {
		t.Errorf("expected positive score, got %f", top[0].Score)
	}
}
