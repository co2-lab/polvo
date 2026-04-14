package memory

import (
	"testing"
	"time"
)

func TestStore_TTLSetOnWrite(t *testing.T) {
	store := openStore(t)

	before := time.Now()
	if err := store.Write(Entry{Agent: "ag", Type: "observation", Content: "obs"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	after := time.Now()

	entries, err := store.Read(Filter{Agent: "ag"})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	e := entries[0]
	if e.ExpiresAt == 0 {
		t.Fatal("expected ExpiresAt to be set, got 0")
	}

	// expires_at should be approximately now + 7 days (Observation TTL)
	expectedMin := before.Add(store.ttl.Observation).UnixNano()
	expectedMax := after.Add(store.ttl.Observation).UnixNano()
	if e.ExpiresAt < expectedMin || e.ExpiresAt > expectedMax {
		t.Errorf("ExpiresAt %d not in expected range [%d, %d]", e.ExpiresAt, expectedMin, expectedMax)
	}
}

func TestStore_PruneDeletesExpired(t *testing.T) {
	store := openStore(t)

	// Write an entry with an already-expired timestamp.
	past := time.Now().Add(-1 * time.Hour).UnixNano()
	if err := store.Write(Entry{
		Agent:     "ag",
		Type:      "observation",
		Content:   "expired",
		ExpiresAt: past,
	}); err != nil {
		t.Fatalf("Write expired: %v", err)
	}

	// Write a non-expiring entry.
	if err := store.Write(Entry{
		Agent:     "ag",
		Type:      "observation",
		Content:   "permanent",
		ExpiresAt: 0,
	}); err != nil {
		t.Fatalf("Write permanent: %v", err)
	}

	n, err := store.Prune()
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 deleted, got %d", n)
	}

	entries, err := store.Read(Filter{Agent: "ag"})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 1 || entries[0].Content != "permanent" {
		t.Errorf("expected only permanent entry, got %v", entries)
	}
}

func TestStore_PruneKeepsNonExpired(t *testing.T) {
	store := openStore(t)

	future := time.Now().Add(24 * time.Hour).UnixNano()
	for i := 0; i < 3; i++ {
		if err := store.Write(Entry{
			Agent:     "ag",
			Type:      "observation",
			Content:   "future",
			ExpiresAt: future,
		}); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	n, err := store.Prune()
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 deleted, got %d", n)
	}

	entries, err := store.Read(Filter{Agent: "ag"})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries to survive, got %d", len(entries))
	}
}

func TestStore_PruneReturnsCount(t *testing.T) {
	store := openStore(t)

	past := time.Now().Add(-1 * time.Hour).UnixNano()
	for i := 0; i < 5; i++ {
		if err := store.Write(Entry{
			Agent:     "ag",
			Type:      "observation",
			Content:   "expired",
			ExpiresAt: past,
		}); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	n, err := store.Prune()
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if n != 5 {
		t.Errorf("expected count=5, got %d", n)
	}
}

func TestTTLConfig_ZeroMeansNoExpiry(t *testing.T) {
	// Open store with zero TTL for all types.
	root := t.TempDir()
	store, err := OpenWithTTL(root, TTLConfig{}) // all zero durations
	if err != nil {
		t.Fatalf("OpenWithTTL: %v", err)
	}
	defer store.Close()

	if err := store.Write(Entry{Agent: "ag", Type: "observation", Content: "no-expiry"}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	entries, err := store.Read(Filter{Agent: "ag"})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].ExpiresAt != 0 {
		t.Errorf("expected ExpiresAt=0 for zero TTL, got %d", entries[0].ExpiresAt)
	}

	// Prune should not delete the entry.
	n, err := store.Prune()
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 pruned for zero-TTL entry, got %d", n)
	}
}
