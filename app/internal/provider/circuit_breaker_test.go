package provider

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// TestCircuitBreaker_OpensAfterThreshold
// ---------------------------------------------------------------------------

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(3, 30*time.Second)

	// First two failures: circuit stays closed.
	cb.RecordFailure()
	if cb.State() != cbClosed {
		t.Errorf("after 1 failure: expected Closed, got %v", cb.State())
	}
	cb.RecordFailure()
	if cb.State() != cbClosed {
		t.Errorf("after 2 failures: expected Closed, got %v", cb.State())
	}

	// Third failure reaches threshold: circuit opens.
	cb.RecordFailure()
	if cb.State() != cbOpen {
		t.Errorf("after 3 failures: expected Open, got %v", cb.State())
	}
}

// ---------------------------------------------------------------------------
// TestCircuitBreaker_BlocksWhenOpen
// ---------------------------------------------------------------------------

func TestCircuitBreaker_BlocksWhenOpen(t *testing.T) {
	cb := NewCircuitBreaker(1, 30*time.Second)
	cb.RecordFailure() // opens immediately (threshold=1)

	if cb.State() != cbOpen {
		t.Fatalf("expected Open state, got %v", cb.State())
	}

	// Allow must return false while circuit is open.
	for i := range 5 {
		if cb.Allow() {
			t.Errorf("Allow() call %d: expected false when circuit is open, got true", i)
		}
	}
}

// ---------------------------------------------------------------------------
// TestCircuitBreaker_HalfOpenAfterDuration
// ---------------------------------------------------------------------------

func TestCircuitBreaker_HalfOpenAfterDuration(t *testing.T) {
	openDuration := 50 * time.Millisecond
	cb := NewCircuitBreaker(1, openDuration)
	cb.RecordFailure() // open

	if cb.State() != cbOpen {
		t.Fatalf("expected Open, got %v", cb.State())
	}

	// Immediately after opening: blocked.
	if cb.Allow() {
		t.Error("Allow() should be false immediately after opening")
	}

	// Wait for the open duration to expire.
	time.Sleep(openDuration + 20*time.Millisecond)

	// First Allow() after expiry transitions to HalfOpen and returns true.
	if !cb.Allow() {
		t.Error("Allow() should return true after openDuration elapsed (probe)")
	}
	if cb.State() != cbHalfOpen {
		t.Errorf("expected HalfOpen after first Allow() past openDuration, got %v", cb.State())
	}

	// Subsequent Allow() in HalfOpen state is blocked (only one probe at a time).
	if cb.Allow() {
		t.Error("Allow() should return false in HalfOpen while probe is in flight")
	}
}

// ---------------------------------------------------------------------------
// TestCircuitBreaker_ClosesOnSuccess
// ---------------------------------------------------------------------------

func TestCircuitBreaker_ClosesOnSuccess(t *testing.T) {
	openDuration := 30 * time.Millisecond
	cb := NewCircuitBreaker(1, openDuration)
	cb.RecordFailure() // open

	time.Sleep(openDuration + 10*time.Millisecond)
	cb.Allow() // transitions to HalfOpen, allows probe

	// Probe succeeds → circuit closes.
	cb.RecordSuccess()
	if cb.State() != cbClosed {
		t.Errorf("after RecordSuccess: expected Closed, got %v", cb.State())
	}

	// Allow() should now return true again.
	if !cb.Allow() {
		t.Error("Allow() should return true after circuit closes")
	}
}

// ---------------------------------------------------------------------------
// TestCircuitBreaker_ResetsOnSuccess
// ---------------------------------------------------------------------------

func TestCircuitBreaker_ResetsOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker(5, 30*time.Second)

	// Accumulate some failures without opening.
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != cbClosed {
		t.Fatalf("expected Closed after 3/5 failures, got %v", cb.State())
	}

	// A success resets the failure counter.
	cb.RecordSuccess()
	if cb.State() != cbClosed {
		t.Errorf("expected Closed after success, got %v", cb.State())
	}

	// Now fail threshold times again: the counter was reset, so it should
	// open only after `threshold` new failures (not threshold - 3).
	for i := range 4 {
		cb.RecordFailure()
		if cb.State() != cbClosed {
			t.Errorf("failure %d after reset: expected Closed, got %v", i+1, cb.State())
		}
	}
	cb.RecordFailure() // 5th failure after reset → open
	if cb.State() != cbOpen {
		t.Errorf("expected Open after threshold failures post-reset, got %v", cb.State())
	}
}
