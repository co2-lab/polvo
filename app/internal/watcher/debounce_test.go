//go:build goexperiment.synctest

package watcher

import (
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// TestDebouncer_CoalescesRepeatedCalls verifies that N calls with same key
// result in fn being called exactly once.
func TestDebouncer_CoalescesRepeatedCalls(t *testing.T) {
	synctest.Run(func() {
		var mu sync.Mutex
		count := 0
		d := NewDebouncer(100 * time.Millisecond)

		d.Debounce("key", func() { mu.Lock(); count++; mu.Unlock() })
		d.Debounce("key", func() { mu.Lock(); count++; mu.Unlock() })
		d.Debounce("key", func() { mu.Lock(); count++; mu.Unlock() })

		time.Sleep(200 * time.Millisecond) // avança o clock fake do synctest
		synctest.Wait()                    // aguarda todos AfterFunc dispararem

		mu.Lock()
		defer mu.Unlock()
		if count != 1 {
			t.Errorf("got %d calls, want 1", count)
		}
	})
}

// TestDebouncer_SeparateKeys verifies that 3 different keys each trigger their callback once.
func TestDebouncer_SeparateKeys(t *testing.T) {
	synctest.Run(func() {
		var mu sync.Mutex
		counts := map[string]int{}
		d := NewDebouncer(100 * time.Millisecond)

		for _, k := range []string{"a", "b", "c"} {
			key := k
			d.Debounce(key, func() {
				mu.Lock()
				counts[key]++
				mu.Unlock()
			})
		}

		time.Sleep(200 * time.Millisecond)
		synctest.Wait()

		mu.Lock()
		defer mu.Unlock()
		for _, k := range []string{"a", "b", "c"} {
			if counts[k] != 1 {
				t.Errorf("key %q: got %d calls, want 1", k, counts[k])
			}
		}
	})
}

// TestDebouncer_ResetOnRepeat verifies that a second call before the delay resets the timer.
// fn should fire only once, after the second call's delay expires.
func TestDebouncer_ResetOnRepeat(t *testing.T) {
	synctest.Run(func() {
		var mu sync.Mutex
		count := 0
		fired := make(chan struct{}, 1)
		d := NewDebouncer(100 * time.Millisecond)

		d.Debounce("key", func() {
			mu.Lock()
			count++
			mu.Unlock()
			fired <- struct{}{}
		})

		// Advance halfway — timer should NOT fire yet
		time.Sleep(50 * time.Millisecond)
		synctest.Wait()

		mu.Lock()
		c := count
		mu.Unlock()
		if c != 0 {
			t.Errorf("after 50ms: got %d calls, want 0", c)
		}

		// Second call resets the timer
		d.Debounce("key", func() {
			mu.Lock()
			count++
			mu.Unlock()
			fired <- struct{}{}
		})

		// Advance past the new delay
		time.Sleep(200 * time.Millisecond)
		synctest.Wait()

		mu.Lock()
		defer mu.Unlock()
		if count != 1 {
			t.Errorf("after reset: got %d calls, want 1", count)
		}
	})
}

// TestDebouncer_ZeroDelay verifies that a zero-duration debounce fires immediately via AfterFunc(0, fn).
func TestDebouncer_ZeroDelay(t *testing.T) {
	synctest.Run(func() {
		var mu sync.Mutex
		count := 0
		d := NewDebouncer(0)

		d.Debounce("key", func() {
			mu.Lock()
			count++
			mu.Unlock()
		})

		time.Sleep(10 * time.Millisecond)
		synctest.Wait()

		mu.Lock()
		defer mu.Unlock()
		if count != 1 {
			t.Errorf("zero delay: got %d calls, want 1", count)
		}
	})
}

// TestDebouncer_RaceCondition verifies no data races when 10 goroutines call Debounce concurrently.
// Run with -race to detect issues.
func TestDebouncer_RaceCondition(t *testing.T) {
	d := NewDebouncer(50 * time.Millisecond)

	var total atomic.Int64
	var wg sync.WaitGroup

	for i := range 10 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			d.Debounce("shared-key", func() {
				total.Add(1)
			})
		}(i)
	}
	wg.Wait()

	// Wait for debounce timer to expire
	time.Sleep(150 * time.Millisecond)

	// At most 1 callback should fire (race test: just ensure no panic/race)
	got := total.Load()
	if got > 1 {
		t.Errorf("got %d calls, want at most 1", got)
	}
}
