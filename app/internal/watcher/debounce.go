package watcher

import (
	"sync"
	"time"
)

// Debouncer coalesces rapid events for the same file.
type Debouncer struct {
	delay  time.Duration
	timers map[string]*time.Timer
	mu     sync.Mutex
}

// NewDebouncer creates a debouncer with the given delay.
func NewDebouncer(delay time.Duration) *Debouncer {
	return &Debouncer{
		delay:  delay,
		timers: make(map[string]*time.Timer),
	}
}

// Debounce calls fn after delay, resetting the timer on repeated calls for the same key.
func (d *Debouncer) Debounce(key string, fn func()) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if t, ok := d.timers[key]; ok {
		t.Stop()
	}

	d.timers[key] = time.AfterFunc(d.delay, func() {
		d.mu.Lock()
		delete(d.timers, key)
		d.mu.Unlock()
		fn()
	})
}
