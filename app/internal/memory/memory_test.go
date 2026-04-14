package memory

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func openStore(t *testing.T) *Store {
	t.Helper()
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// ---------------------------------------------------------------------------
// TestMemoryOpen
// ---------------------------------------------------------------------------

func TestMemoryOpen(t *testing.T) {
	t.Run("existing root", func(t *testing.T) {
		root := t.TempDir()
		store, err := Open(root)
		if err != nil {
			t.Fatalf("Open existing root: %v", err)
		}
		store.Close()
	})

	t.Run("non-existing root creates dirs", func(t *testing.T) {
		root := t.TempDir() + "/newdir/subdir"
		store, err := Open(root)
		if err != nil {
			t.Fatalf("Open non-existing root: %v", err)
		}
		store.Close()
	})

	t.Run("idempotent second open", func(t *testing.T) {
		root := t.TempDir()
		store1, err := Open(root)
		if err != nil {
			t.Fatalf("first Open: %v", err)
		}
		store1.Close()

		store2, err := Open(root)
		if err != nil {
			t.Fatalf("second Open: %v", err)
		}
		store2.Close()
	})

	t.Run("Close without error", func(t *testing.T) {
		root := t.TempDir()
		store, err := Open(root)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		if err := store.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// TestMemoryWrite_Read
// ---------------------------------------------------------------------------

func TestMemoryWrite_Read(t *testing.T) {
	t.Run("write 1 entry read back", func(t *testing.T) {
		store := openStore(t)
		e := Entry{Agent: "agent-a", Type: "observation", Content: "hello"}
		if err := store.Write(e); err != nil {
			t.Fatalf("Write: %v", err)
		}
		entries, err := store.Read(Filter{})
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(entries))
		}
		if entries[0].Content != "hello" {
			t.Errorf("content mismatch: %q", entries[0].Content)
		}
	})

	t.Run("write 3 entries read all", func(t *testing.T) {
		store := openStore(t)
		for i := 0; i < 3; i++ {
			if err := store.Write(Entry{Agent: "ag", Type: "observation", Content: fmt.Sprintf("c%d", i)}); err != nil {
				t.Fatalf("Write %d: %v", i, err)
			}
		}
		entries, err := store.Read(Filter{})
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if len(entries) != 3 {
			t.Errorf("expected 3, got %d", len(entries))
		}
	})

	t.Run("filter by Agent", func(t *testing.T) {
		store := openStore(t)
		store.Write(Entry{Agent: "a", Type: "observation", Content: "x"})
		store.Write(Entry{Agent: "b", Type: "observation", Content: "y"})
		entries, err := store.Read(Filter{Agent: "a"})
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if len(entries) != 1 || entries[0].Agent != "a" {
			t.Errorf("expected 1 entry for agent 'a', got %v", entries)
		}
	})

	t.Run("filter by Type", func(t *testing.T) {
		store := openStore(t)
		store.Write(Entry{Agent: "ag", Type: "observation", Content: "o"})
		store.Write(Entry{Agent: "ag", Type: "decision", Content: "d"})
		entries, err := store.Read(Filter{Type: "decision"})
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if len(entries) != 1 || entries[0].Type != "decision" {
			t.Errorf("expected 1 decision entry, got %v", entries)
		}
	})

	t.Run("filter by File", func(t *testing.T) {
		store := openStore(t)
		store.Write(Entry{Agent: "ag", Type: "observation", File: "main.go", Content: "a"})
		store.Write(Entry{Agent: "ag", Type: "observation", File: "other.go", Content: "b"})
		entries, err := store.Read(Filter{File: "main.go"})
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if len(entries) != 1 || entries[0].File != "main.go" {
			t.Errorf("expected 1 entry for main.go, got %v", entries)
		}
	})

	t.Run("filter by SessionID", func(t *testing.T) {
		store := openStore(t)
		store.StartSession("sess-1", "ag")
		store.Write(Entry{Agent: "ag", Type: "observation", SessionID: "sess-1", Content: "s1"})
		store.Write(Entry{Agent: "ag", Type: "observation", Content: "no-sess"})
		entries, err := store.Read(Filter{SessionID: "sess-1"})
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if len(entries) != 1 || entries[0].SessionID != "sess-1" {
			t.Errorf("expected 1 entry for sess-1, got %v", entries)
		}
	})

	t.Run("Limit=2 with 5 entries", func(t *testing.T) {
		store := openStore(t)
		for i := 0; i < 5; i++ {
			store.Write(Entry{Agent: "ag", Type: "observation", Content: fmt.Sprintf("c%d", i)})
		}
		entries, err := store.Read(Filter{Limit: 2})
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if len(entries) != 2 {
			t.Errorf("expected 2 with Limit=2, got %d", len(entries))
		}
	})
}

// ---------------------------------------------------------------------------
// TestMemoryRead_Ordering
// ---------------------------------------------------------------------------

func TestMemoryRead_Ordering(t *testing.T) {
	store := openStore(t)

	t1 := time.Now().Add(-2 * time.Second).UnixNano()
	t2 := time.Now().UnixNano()

	store.Write(Entry{Agent: "ag", Type: "observation", Content: "older", Timestamp: t1})
	store.Write(Entry{Agent: "ag", Type: "observation", Content: "newer", Timestamp: t2})

	entries, err := store.Read(Filter{})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	// DESC order: entries[0] should be newer (larger timestamp)
	if entries[0].Timestamp <= entries[1].Timestamp {
		t.Errorf("expected DESC order: entries[0].Timestamp (%d) > entries[1].Timestamp (%d)",
			entries[0].Timestamp, entries[1].Timestamp)
	}
}

// ---------------------------------------------------------------------------
// TestMemoryRead_EmptyResult
// ---------------------------------------------------------------------------

func TestMemoryRead_EmptyResult(t *testing.T) {
	store := openStore(t)
	// Filter that matches nothing — Read returns nil, nil (not empty slice)
	entries, err := store.Read(Filter{Agent: "nonexistent"})
	if err != nil {
		t.Fatal(err)
	}
	// The implementation uses append-only loop; no entries appended → slice is nil
	if entries != nil {
		t.Errorf("expected nil slice for empty result, got %v", entries)
	}
}

// ---------------------------------------------------------------------------
// TestMemorySession
// ---------------------------------------------------------------------------

func TestMemorySession(t *testing.T) {
	store := openStore(t)

	t.Run("StartSession and EndSession", func(t *testing.T) {
		if err := store.StartSession("sess-x", "agent-a"); err != nil {
			t.Fatalf("StartSession: %v", err)
		}
		if err := store.EndSession("sess-x"); err != nil {
			t.Fatalf("EndSession: %v", err)
		}
	})

	t.Run("EndSession non-existent tolerant", func(t *testing.T) {
		// UPDATE on non-existent id — 0 rows affected, no error
		if err := store.EndSession("nonexistent-session"); err != nil {
			t.Errorf("EndSession non-existent should be tolerant, got: %v", err)
		}
	})

	t.Run("StartSession duplicate returns error", func(t *testing.T) {
		if err := store.StartSession("sess-dup", "agent-a"); err != nil {
			t.Fatalf("first StartSession: %v", err)
		}
		// INSERT INTO with same PRIMARY KEY should fail
		err := store.StartSession("sess-dup", "agent-b")
		if err == nil {
			t.Error("expected error on duplicate StartSession, got nil")
		}
	})
}

// ---------------------------------------------------------------------------
// TestMemoryNull_ViaReadWrite (testing nullStr indirectly)
// ---------------------------------------------------------------------------

func TestMemoryNull_ViaReadWrite(t *testing.T) {
	store := openStore(t)

	t.Run("empty File round-trip", func(t *testing.T) {
		if err := store.Write(Entry{Agent: "ag", Type: "observation", File: "", Content: "no-file"}); err != nil {
			t.Fatalf("Write: %v", err)
		}
		entries, err := store.Read(Filter{Agent: "ag"})
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if len(entries) == 0 {
			t.Fatal("expected 1 entry")
		}
		if entries[0].File != "" {
			t.Errorf("expected empty File, got %q", entries[0].File)
		}
	})

	t.Run("non-empty File round-trip", func(t *testing.T) {
		store2 := openStore(t)
		if err := store2.Write(Entry{Agent: "ag", Type: "observation", File: "main.go", Content: "with-file"}); err != nil {
			t.Fatalf("Write: %v", err)
		}
		entries, err := store2.Read(Filter{Agent: "ag"})
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if len(entries) == 0 {
			t.Fatal("expected 1 entry")
		}
		if entries[0].File != "main.go" {
			t.Errorf("expected 'main.go', got %q", entries[0].File)
		}
	})
}

// ---------------------------------------------------------------------------
// TestMemoryConcurrentWrite
// ---------------------------------------------------------------------------

func TestMemoryConcurrentWrite(t *testing.T) {
	store := openStore(t)
	const n = 10
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			store.Write(Entry{
				Agent:   fmt.Sprintf("agent-%d", idx),
				Type:    "observation",
				Content: "x",
			})
		}(i)
	}
	wg.Wait()

	entries, err := store.Read(Filter{})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != n {
		t.Errorf("expected %d entries after concurrent writes, got %d", n, len(entries))
	}
}

// ---------------------------------------------------------------------------
// TestMemoryWriteAfterClose
// ---------------------------------------------------------------------------

func TestMemoryWriteAfterClose(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Write after close should return an error, not panic
	err = store.Write(Entry{Agent: "ag", Type: "observation", Content: "after close"})
	if err == nil {
		t.Error("expected error after Write on closed store, got nil")
	}
}
