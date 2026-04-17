package agent

import (
	"sync"
	"testing"
	"time"
)

func TestEventStream_SubscribeAndReceive(t *testing.T) {
	s := NewEventStream(8)
	defer s.Close()

	ch := s.Subscribe()
	s.Emit(StreamEvent{Kind: EventTurnEnd, Message: "hello"})

	select {
	case ev := <-ch:
		if ev.Kind != EventTurnEnd {
			t.Errorf("want EventTurnEnd, got %v", ev.Kind)
		}
		if ev.Message != "hello" {
			t.Errorf("want 'hello', got %q", ev.Message)
		}
		if ev.Timestamp.IsZero() {
			t.Error("Timestamp should be set automatically")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestEventStream_FanOut(t *testing.T) {
	s := NewEventStream(8)
	defer s.Close()

	ch1 := s.Subscribe()
	ch2 := s.Subscribe()
	ch3 := s.Subscribe()

	s.Emit(StreamEvent{Kind: EventToolCall, ToolName: "bash"})

	for _, ch := range []<-chan StreamEvent{ch1, ch2, ch3} {
		select {
		case ev := <-ch:
			if ev.ToolName != "bash" {
				t.Errorf("want 'bash', got %q", ev.ToolName)
			}
		case <-time.After(time.Second):
			t.Fatal("timeout — fan-out failed for one subscriber")
		}
	}
}

func TestEventStream_DropOnFullBuffer(t *testing.T) {
	s := NewEventStream(1) // buffer size 1
	defer s.Close()

	ch := s.Subscribe()
	// Fill buffer
	s.Emit(StreamEvent{Kind: EventTurnStart})
	// This should be dropped (buffer full, non-blocking)
	s.Emit(StreamEvent{Kind: EventDone, Message: "should-drop"})

	// Only 1 event in buffer
	select {
	case ev := <-ch:
		if ev.Kind != EventTurnStart {
			t.Errorf("expected TurnStart, got %v", ev.Kind)
		}
	default:
		t.Fatal("expected at least one buffered event")
	}

	// Second event should not be there
	select {
	case ev := <-ch:
		t.Errorf("unexpected event after drop: %v", ev.Kind)
	default:
		// correct: buffer was full, second event dropped
	}
}

func TestEventStream_CloseSignalsSubscribers(t *testing.T) {
	s := NewEventStream(4)
	ch := s.Subscribe()
	s.Close()

	// After close, channel should be drained and closed
	// Drain any buffered events
	for range ch {
	}
	// Channel is closed — range above exits
}

func TestEventStream_EmitAfterCloseIsNoop(t *testing.T) {
	s := NewEventStream(4)
	s.Close()
	// Should not panic
	s.Emit(StreamEvent{Kind: EventDone})
}

func TestEventStream_SubscriberCount(t *testing.T) {
	s := NewEventStream(4)
	defer s.Close()

	if s.SubscriberCount() != 0 {
		t.Errorf("expected 0 subscribers initially")
	}
	s.Subscribe()
	s.Subscribe()
	if s.SubscriberCount() != 2 {
		t.Errorf("expected 2, got %d", s.SubscriberCount())
	}
}

func TestEventStream_Race(t *testing.T) {
	s := NewEventStream(16)
	defer s.Close()

	var wg sync.WaitGroup
	// Concurrent subscribers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch := s.Subscribe()
			_ = ch
		}()
	}
	// Concurrent emitters
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Emit(StreamEvent{Kind: EventToolCall, ToolName: "bash"})
		}()
	}
	wg.Wait()
}

func TestEventStream_KindString(t *testing.T) {
	tests := []struct {
		k    EventKind
		want string
	}{
		{EventTurnStart, "turn_start"},
		{EventTurnEnd, "turn_end"},
		{EventToolCall, "tool_call"},
		{EventToolResult, "tool_result"},
		{EventApproval, "approval"},
		{EventApprovalDone, "approval_done"},
		{EventStepComplete, "step_complete"},
		{EventError, "error"},
		{EventDone, "done"},
	}
	for _, tc := range tests {
		if got := tc.k.String(); got != tc.want {
			t.Errorf("%d: got %q, want %q", tc.k, got, tc.want)
		}
	}
}
