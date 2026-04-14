package provider

import (
	"errors"
	"sync"
	"time"
)

// ErrCircuitOpen is returned when the circuit breaker is open and blocking requests.
var ErrCircuitOpen = errors.New("circuit breaker open: provider temporarily unavailable")

// cbState represents circuit breaker state.
type cbState int

const (
	cbClosed   cbState = iota // normal operation
	cbOpen                    // blocking all requests
	cbHalfOpen                // allowing one probe request
)

// CircuitBreaker implements the circuit breaker pattern for LLM provider calls.
// After threshold consecutive failures it opens for openDuration, then
// transitions to half-open to probe whether the provider has recovered.
type CircuitBreaker struct {
	mu           sync.Mutex
	state        cbState
	failures     int
	lastFailure  time.Time
	threshold    int           // failures before opening
	openDuration time.Duration // how long to stay open
}

// NewCircuitBreaker creates a CircuitBreaker with sensible defaults.
// threshold=0 → defaults to 5; openDuration=0 → defaults to 30s.
func NewCircuitBreaker(threshold int, openDuration time.Duration) *CircuitBreaker {
	if threshold <= 0 {
		threshold = 5
	}
	if openDuration <= 0 {
		openDuration = 30 * time.Second
	}
	return &CircuitBreaker{
		threshold:    threshold,
		openDuration: openDuration,
	}
}

// Allow returns true if a request should be allowed through.
//   - Closed:   always true.
//   - Open:     true only if openDuration has elapsed (transitions to HalfOpen).
//   - HalfOpen: true only for the first probe request (subsequent callers blocked
//     until the probe result is recorded via RecordSuccess/RecordFailure).
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case cbClosed:
		return true

	case cbOpen:
		if time.Since(cb.lastFailure) >= cb.openDuration {
			cb.state = cbHalfOpen
			return true // first caller gets the probe slot
		}
		return false

	case cbHalfOpen:
		// Only one probe is in flight at a time; block further callers.
		return false
	}
	return false
}

// RecordSuccess resets the circuit breaker to Closed state and clears failure count.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = cbClosed
	cb.failures = 0
}

// RecordFailure increments the failure counter.
// When the threshold is reached the circuit opens.
// If already in HalfOpen (probe failed), it re-opens immediately.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()

	if cb.state == cbHalfOpen || cb.failures >= cb.threshold {
		cb.state = cbOpen
	}
}

// State returns the current state (for observability/logging).
func (cb *CircuitBreaker) State() cbState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}
