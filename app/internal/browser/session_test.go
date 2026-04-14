package browser

import (
	"testing"
)

func TestBrowserSession_AcquireRelease(t *testing.T) {
	s := &BrowserSession{}

	if s.IsActive() {
		t.Fatal("new session should not be active")
	}

	if err := s.Acquire(); err != nil {
		t.Fatalf("first Acquire failed: %v", err)
	}
	if !s.IsActive() {
		t.Fatal("session should be active after Acquire")
	}

	s.Release()
	if s.IsActive() {
		t.Fatal("session should not be active after Release")
	}
}

func TestBrowserSession_DoubleAcquire(t *testing.T) {
	s := &BrowserSession{}

	if err := s.Acquire(); err != nil {
		t.Fatalf("first Acquire failed: %v", err)
	}
	defer s.Release()

	if err := s.Acquire(); err == nil {
		t.Fatal("second Acquire should return an error while session is active")
	}
}

func TestBrowserSession_AcquireAfterRelease(t *testing.T) {
	s := &BrowserSession{}

	if err := s.Acquire(); err != nil {
		t.Fatalf("first Acquire failed: %v", err)
	}
	s.Release()

	// Should be acquirable again after release.
	if err := s.Acquire(); err != nil {
		t.Fatalf("second Acquire (after Release) failed: %v", err)
	}
	s.Release()
}

// ---------------------------------------------------------------------------
// New gap-filling tests
// ---------------------------------------------------------------------------

// TestBrowserSession_IndependentInstances verifies that two separate
// BrowserSession instances are fully independent: acquiring on one does not
// block or affect acquisition on the other.
func TestBrowserSession_IndependentInstances(t *testing.T) {
	s1 := &BrowserSession{}
	s2 := &BrowserSession{}

	// Acquire on instance 1.
	if err := s1.Acquire(); err != nil {
		t.Fatalf("s1.Acquire failed: %v", err)
	}
	defer s1.Release()

	// Instance 2 is a completely separate object; it must be acquirable
	// regardless of s1's state.
	if err := s2.Acquire(); err != nil {
		t.Fatalf("s2.Acquire failed while s1 is active: %v — sessions should be independent", err)
	}
	defer s2.Release()

	if !s1.IsActive() {
		t.Error("s1 should still be active")
	}
	if !s2.IsActive() {
		t.Error("s2 should be active after Acquire")
	}

	// Release s1; s2 must remain active.
	s1.Release()
	if s1.IsActive() {
		t.Error("s1 should not be active after Release")
	}
	if !s2.IsActive() {
		t.Error("s2 should still be active after s1 was released")
	}
}
