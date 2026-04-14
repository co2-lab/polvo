package watcher

import (
	"sort"
	"testing"
	"time"
)

// receiveBatch waits up to timeout for a batch from the accumulator.
func receiveBatch(t *testing.T, acc *EventAccumulator, timeout time.Duration) []WatchEvent {
	t.Helper()
	select {
	case batch, ok := <-acc.Events():
		if !ok {
			return nil
		}
		return batch
	case <-time.After(timeout):
		t.Fatal("timed out waiting for batch")
		return nil
	}
}

// sortedPaths returns the paths in a batch, sorted for deterministic comparison.
func sortedPaths(batch []WatchEvent) []string {
	paths := make([]string, len(batch))
	for i, e := range batch {
		paths[i] = e.Path
	}
	sort.Strings(paths)
	return paths
}

// pathOp returns the Op for a given path in a batch.
func pathOp(batch []WatchEvent, path string) (Op, bool) {
	for _, e := range batch {
		if e.Path == path {
			return e.Op, true
		}
	}
	return "", false
}

// ---------------------------------------------------------------------------
// TestAccumulator_BatchesWithinWindow: multiple events within the window are
// delivered as a single batch.
// ---------------------------------------------------------------------------

func TestAccumulator_BatchesWithinWindow(t *testing.T) {
	window := 80 * time.Millisecond
	acc := NewEventAccumulator(window, 100)
	defer acc.Close()

	acc.Add(WatchEvent{WatcherName: "w", Path: "/a.go", Op: OpModify})
	acc.Add(WatchEvent{WatcherName: "w", Path: "/b.go", Op: OpModify})
	acc.Add(WatchEvent{WatcherName: "w", Path: "/c.go", Op: OpCreate})

	batch := receiveBatch(t, acc, 2*time.Second)
	if len(batch) != 3 {
		t.Fatalf("expected 3 events in batch, got %d", len(batch))
	}

	paths := sortedPaths(batch)
	want := []string{"/a.go", "/b.go", "/c.go"}
	for i, p := range want {
		if paths[i] != p {
			t.Errorf("batch[%d]: got %q, want %q", i, paths[i], p)
		}
	}
}

// ---------------------------------------------------------------------------
// TestAccumulator_CoalescesSameFile: two Modify events for the same path are
// merged into a single Modify event.
// ---------------------------------------------------------------------------

func TestAccumulator_CoalescesSameFile(t *testing.T) {
	acc := NewEventAccumulator(80*time.Millisecond, 100)
	defer acc.Close()

	acc.Add(WatchEvent{WatcherName: "w", Path: "/a.go", Op: OpModify})
	acc.Add(WatchEvent{WatcherName: "w", Path: "/a.go", Op: OpModify})

	batch := receiveBatch(t, acc, 2*time.Second)
	if len(batch) != 1 {
		t.Fatalf("expected 1 coalesced event, got %d", len(batch))
	}
	if batch[0].Path != "/a.go" {
		t.Errorf("expected path /a.go, got %q", batch[0].Path)
	}
	if batch[0].Op != OpModify {
		t.Errorf("expected OpModify, got %q", batch[0].Op)
	}
}

// ---------------------------------------------------------------------------
// TestAccumulator_CreateThenDelete: create followed by delete → net DELETE.
// ---------------------------------------------------------------------------

func TestAccumulator_CreateThenDelete(t *testing.T) {
	acc := NewEventAccumulator(80*time.Millisecond, 100)
	defer acc.Close()

	acc.Add(WatchEvent{WatcherName: "w", Path: "/tmp.go", Op: OpCreate})
	acc.Add(WatchEvent{WatcherName: "w", Path: "/tmp.go", Op: OpDelete})

	batch := receiveBatch(t, acc, 2*time.Second)
	if len(batch) != 1 {
		t.Fatalf("expected 1 event, got %d", len(batch))
	}
	if batch[0].Op != OpDelete {
		t.Errorf("create+delete should yield OpDelete, got %q", batch[0].Op)
	}
}

// ---------------------------------------------------------------------------
// TestAccumulator_DeleteThenCreate: delete followed by create → net CREATE.
// ---------------------------------------------------------------------------

func TestAccumulator_DeleteThenCreate(t *testing.T) {
	acc := NewEventAccumulator(80*time.Millisecond, 100)
	defer acc.Close()

	acc.Add(WatchEvent{WatcherName: "w", Path: "/f.go", Op: OpDelete})
	acc.Add(WatchEvent{WatcherName: "w", Path: "/f.go", Op: OpCreate})

	batch := receiveBatch(t, acc, 2*time.Second)
	if len(batch) != 1 {
		t.Fatalf("expected 1 event, got %d", len(batch))
	}
	if batch[0].Op != OpCreate {
		t.Errorf("delete+create should yield OpCreate, got %q", batch[0].Op)
	}
}

// ---------------------------------------------------------------------------
// TestAccumulator_FlushesOnMaxBatch: when maxBatch is reached, the batch is
// emitted immediately without waiting for the window to elapse.
// ---------------------------------------------------------------------------

func TestAccumulator_FlushesOnMaxBatch(t *testing.T) {
	maxBatch := 5
	// Use a very long window so we know the flush must have been triggered
	// by the size limit, not the timer.
	acc := NewEventAccumulator(10*time.Second, maxBatch)
	defer acc.Close()

	for i := range maxBatch {
		acc.Add(WatchEvent{WatcherName: "w", Path: string(rune('a'+i)) + ".go", Op: OpModify})
	}

	// Batch should arrive well before the 10 s window expires.
	batch := receiveBatch(t, acc, 2*time.Second)
	if len(batch) != maxBatch {
		t.Fatalf("expected %d events in immediate flush, got %d", maxBatch, len(batch))
	}
}

// ---------------------------------------------------------------------------
// TestAccumulator_Close: after Close, the output channel is closed and no
// further events are accepted.
// ---------------------------------------------------------------------------

func TestAccumulator_Close(t *testing.T) {
	acc := NewEventAccumulator(80*time.Millisecond, 100)
	acc.Close()

	// The output channel should be closed.
	select {
	case _, ok := <-acc.Events():
		if ok {
			t.Fatal("expected channel to be closed, but received a value")
		}
		// ok == false means channel is properly closed
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected closed channel to be readable immediately")
	}

	// Adding events after Close should not panic.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Add after Close panicked: %v", r)
			}
		}()
		acc.Add(WatchEvent{WatcherName: "w", Path: "/x.go", Op: OpModify})
	}()

	// Calling Close a second time should not panic (sync.Once).
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("second Close panicked: %v", r)
			}
		}()
		acc.Close()
	}()
}
