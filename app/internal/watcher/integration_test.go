//go:build integration

package watcher

import (
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"
)

// eventSeparatorMs is the minimum time between filesystem operations.
// The OS may coalesce events that arrive too close together, and the
// debounce delay needs time to fire. Derived from fsnotify/helpers_test.go.
const eventSeparatorMs = 50 * time.Millisecond

// eventCollector gathers WatchEvents from a channel in a background goroutine.
type eventCollector struct {
	events []WatchEvent
	mu     sync.Mutex
	done   chan struct{}
}

func newCollector(ch <-chan WatchEvent) *eventCollector {
	c := &eventCollector{done: make(chan struct{})}
	go func() {
		for {
			select {
			case ev := <-ch:
				c.mu.Lock()
				c.events = append(c.events, ev)
				c.mu.Unlock()
			case <-c.done:
				return
			}
		}
	}()
	return c
}

func (c *eventCollector) stop() []WatchEvent {
	close(c.done)
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]WatchEvent(nil), c.events...)
}

// makeWatcher creates a Watcher over root with a 50ms debounce, returns the channel.
func makeWatcher(t *testing.T, root string, patterns []string) (*Watcher, chan WatchEvent) {
	t.Helper()
	ch := make(chan WatchEvent, 10)
	w := New("test", root, patterns, 50 /* debounce ms */, ch, slog.Default())
	t.Cleanup(func() { w.Stop() })
	go func() {
		if err := w.Start(); err != nil {
			// Start returns nil when done is closed — only log real errors
			t.Logf("watcher.Start returned: %v", err)
		}
	}()
	// Give the watcher time to add directories
	time.Sleep(eventSeparatorMs)
	return w, ch
}

// recv waits for one event from ch with a 2-second timeout.
func recv(t *testing.T, ch <-chan WatchEvent) WatchEvent {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
		return WatchEvent{}
	}
}

// TestWatcher_CreateEvent verifies that creating a file emits OpCreate.
func TestWatcher_CreateEvent(t *testing.T) {
	root := t.TempDir()
	_, ch := makeWatcher(t, root, []string{"*.go"})
	col := newCollector(ch)
	defer col.stop()

	f := root + "/foo.go"
	if err := os.WriteFile(f, []byte("package main"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	time.Sleep(eventSeparatorMs)

	ev := recv(t, ch)
	if ev.Op != OpCreate && ev.Op != OpModify {
		t.Errorf("expected OpCreate or OpModify, got %q", ev.Op)
	}
	if ev.WatcherName != "test" {
		t.Errorf("expected watcher name 'test', got %q", ev.WatcherName)
	}
}

// TestWatcher_ModifyEvent verifies that writing to a file emits OpModify.
func TestWatcher_ModifyEvent(t *testing.T) {
	root := t.TempDir()
	// Create file before watcher starts to ensure it exists
	f := root + "/bar.go"
	if err := os.WriteFile(f, []byte("package main"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	time.Sleep(eventSeparatorMs)

	_, ch := makeWatcher(t, root, []string{"*.go"})
	// Drain any create event
	time.Sleep(eventSeparatorMs)
	for len(ch) > 0 {
		<-ch
	}

	// Modify the file
	if err := os.WriteFile(f, []byte("package main\n// modified"), 0o644); err != nil {
		t.Fatalf("WriteFile modify: %v", err)
	}
	time.Sleep(eventSeparatorMs)

	ev := recv(t, ch)
	if ev.Op != OpModify && ev.Op != OpCreate {
		t.Errorf("expected OpModify, got %q", ev.Op)
	}
}

// TestWatcher_DeleteEvent verifies that deleting a file emits OpDelete.
func TestWatcher_DeleteEvent(t *testing.T) {
	root := t.TempDir()
	f := root + "/baz.go"
	if err := os.WriteFile(f, []byte("package main"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	time.Sleep(eventSeparatorMs)

	_, ch := makeWatcher(t, root, []string{"*.go"})
	// Drain any initial events
	time.Sleep(eventSeparatorMs)
	for len(ch) > 0 {
		<-ch
	}

	if err := os.Remove(f); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	time.Sleep(eventSeparatorMs)

	ev := recv(t, ch)
	if ev.Op != OpDelete {
		t.Errorf("expected OpDelete, got %q", ev.Op)
	}
}

// TestWatcher_ExcludedFileNoEvent verifies that a file matching an exclude pattern
// produces no event on the channel.
func TestWatcher_ExcludedFileNoEvent(t *testing.T) {
	root := t.TempDir()
	_, ch := makeWatcher(t, root, []string{"*.go", "!*_test.go"})

	f := root + "/foo_test.go"
	if err := os.WriteFile(f, []byte("package main"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	time.Sleep(300 * time.Millisecond) // wait well past debounce

	if len(ch) > 0 {
		ev := <-ch
		t.Errorf("expected no event for excluded file, got %+v", ev)
	}
}

// TestWatcher_StopNoLeak verifies that Stop() terminates the goroutine cleanly.
func TestWatcher_StopNoLeak(t *testing.T) {
	root := t.TempDir()
	ch := make(chan WatchEvent, 10)
	w := New("test", root, nil, 50, ch, slog.Default())

	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		close(started)
		w.Start()
		close(done)
	}()

	<-started
	time.Sleep(eventSeparatorMs)
	w.Stop()

	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("watcher goroutine did not stop after Stop()")
	}
}
