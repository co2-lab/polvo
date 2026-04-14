package browser

import (
	"errors"
	"sync/atomic"
	"time"
)

// ErrSessionActive is returned when a browser session is already in progress.
var ErrSessionActive = errors.New("browser session is already active")

// BrowserSession tracks whether a browser is currently active.
// While active, other high-risk tools (bash, write) should be blocked.
type BrowserSession struct {
	active    atomic.Bool
	startedAt time.Time
}

// Acquire marks the browser as active.
// Returns ErrSessionActive if a session is already in progress.
func (s *BrowserSession) Acquire() error {
	if !s.active.CompareAndSwap(false, true) {
		return ErrSessionActive
	}
	s.startedAt = time.Now()
	return nil
}

// Release marks the browser as inactive.
func (s *BrowserSession) Release() {
	s.active.Store(false)
}

// IsActive reports whether a browser session is currently in progress.
func (s *BrowserSession) IsActive() bool {
	return s.active.Load()
}

// StartedAt returns the time the session was acquired.
// The zero value is returned if no session is active.
func (s *BrowserSession) StartedAt() time.Time {
	return s.startedAt
}
