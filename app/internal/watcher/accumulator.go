package watcher

import (
	"sync"
	"time"
)

// EventAccumulator batches watcher events within a short window before emitting.
// Multiple events to the same file are coalesced into a single event (last op wins,
// except create+delete → net delete, delete+create → net create/modify).
type EventAccumulator struct {
	window   time.Duration
	maxBatch int
	mu       sync.Mutex
	pending  map[string]WatchEvent // path → coalesced event
	timer    *time.Timer
	out      chan []WatchEvent
	once     sync.Once
	done     chan struct{}
}

// NewEventAccumulator creates an accumulator with the given window and max batch size.
// window: how long to wait after first event before flushing (default 150ms if zero).
// maxBatch: flush immediately if this many files accumulate (default 50 if zero).
func NewEventAccumulator(window time.Duration, maxBatch int) *EventAccumulator {
	if window <= 0 {
		window = 150 * time.Millisecond
	}
	if maxBatch <= 0 {
		maxBatch = 50
	}
	return &EventAccumulator{
		window:   window,
		maxBatch: maxBatch,
		pending:  make(map[string]WatchEvent),
		out:      make(chan []WatchEvent, 16),
		done:     make(chan struct{}),
	}
}

// Add adds an event to the accumulator. Thread-safe.
// OpChmod events are silently ignored.
func (a *EventAccumulator) Add(e WatchEvent) {
	// OpChmod is ignored entirely
	if e.Op == "chmod" {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	select {
	case <-a.done:
		return
	default:
	}

	existing, exists := a.pending[e.Path]
	if exists {
		e = coalesce(existing, e)
	}
	a.pending[e.Path] = e

	// Start the window timer on the first event in an empty batch
	if !exists && a.timer == nil {
		a.timer = time.AfterFunc(a.window, a.flush)
	}

	// Flush immediately if we've hit the max batch size
	if len(a.pending) >= a.maxBatch {
		if a.timer != nil {
			a.timer.Stop()
			a.timer = nil
		}
		// Flush without holding the lock — but we already hold it here.
		// We do an inline flush instead.
		a.flushLocked()
	}
}

// Events returns the channel that emits batched event slices.
func (a *EventAccumulator) Events() <-chan []WatchEvent {
	return a.out
}

// Close shuts down the accumulator and closes the output channel.
func (a *EventAccumulator) Close() {
	a.once.Do(func() {
		close(a.done)
		a.mu.Lock()
		if a.timer != nil {
			a.timer.Stop()
			a.timer = nil
		}
		a.mu.Unlock()
		close(a.out)
	})
}

// flush is called by the timer goroutine (no lock held).
func (a *EventAccumulator) flush() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.timer = nil
	a.flushLocked()
}

// flushLocked drains pending into out. Must be called with a.mu held.
func (a *EventAccumulator) flushLocked() {
	if len(a.pending) == 0 {
		return
	}
	batch := make([]WatchEvent, 0, len(a.pending))
	for _, ev := range a.pending {
		batch = append(batch, ev)
	}
	a.pending = make(map[string]WatchEvent)

	// Non-blocking send: if the output buffer is full, drop the batch to
	// avoid deadlocking the timer goroutine. A full buffer means the consumer
	// is lagging, which is an operational concern handled upstream.
	select {
	case a.out <- batch:
	default:
	}
}

// coalesce merges two events for the same path.
// Rules:
//   - OpCreate  + OpDelete  → OpDelete
//   - OpDelete  + OpCreate  → OpCreate  (net new file)
//   - OpModify  + OpModify  → OpModify
//   - anything  + anything  → use the later op (last-write wins)
func coalesce(first, second WatchEvent) WatchEvent {
	result := second // inherit path, watcher name, and second op by default

	switch {
	case first.Op == OpCreate && second.Op == OpDelete:
		result.Op = OpDelete
	case first.Op == OpDelete && second.Op == OpCreate:
		result.Op = OpCreate
	// All other combinations: use second.Op (already set)
	}

	return result
}
