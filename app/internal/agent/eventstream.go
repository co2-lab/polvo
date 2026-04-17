package agent

import (
	"log/slog"
	"sync"
	"time"
)

// EventKind classifies an agent loop event.
type EventKind int

const (
	EventTurnStart    EventKind = iota // LLM turn begins
	EventTurnEnd                       // LLM turn finished (message received)
	EventToolCall                      // tool about to be executed
	EventToolResult                    // tool execution result
	EventApproval                      // user approval requested
	EventApprovalDone                  // user approval decision received
	EventStepComplete                  // agent step (turn+tools) complete
	EventError                         // non-fatal error logged
	EventDone                          // agent loop finished
)

func (k EventKind) String() string {
	switch k {
	case EventTurnStart:
		return "turn_start"
	case EventTurnEnd:
		return "turn_end"
	case EventToolCall:
		return "tool_call"
	case EventToolResult:
		return "tool_result"
	case EventApproval:
		return "approval"
	case EventApprovalDone:
		return "approval_done"
	case EventStepComplete:
		return "step_complete"
	case EventError:
		return "error"
	case EventDone:
		return "done"
	default:
		return "unknown"
	}
}

// StreamEvent is a single typed event emitted by the agent loop.
type StreamEvent struct {
	Kind      EventKind
	AgentName string
	SessionID string
	Timestamp time.Time

	// Kind-specific payload — only one is set per event.
	ToolName   string // EventToolCall, EventToolResult
	ToolInput  string // EventToolCall (JSON)
	ToolOutput string // EventToolResult
	Message    string // EventTurnEnd (assistant text), EventError
	Step       int    // EventStepComplete
	RiskLevel  string // EventApproval
	Preview    string // EventApproval
	Decision   string // EventApprovalDone
}

// EventStream is a typed, fan-out event stream for agent loop events.
// Subscribers receive events via buffered channels.
// Non-blocking: events are dropped (with a slog.Warn) if a subscriber is full.
type EventStream struct {
	mu          sync.RWMutex
	subscribers []chan StreamEvent
	bufSize     int
	closed      bool
	closeOnce   sync.Once
}

// NewEventStream creates an EventStream with the given per-subscriber buffer size (default 32).
func NewEventStream(bufSize int) *EventStream {
	if bufSize <= 0 {
		bufSize = 32
	}
	return &EventStream{bufSize: bufSize}
}

// Subscribe returns a receive-only channel that will receive all future events.
// Returns a closed channel if the stream is already closed.
func (s *EventStream) Subscribe() <-chan StreamEvent {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		ch := make(chan StreamEvent)
		close(ch)
		return ch
	}
	ch := make(chan StreamEvent, s.bufSize)
	s.subscribers = append(s.subscribers, ch)
	return ch
}

// Emit broadcasts an event to all subscribers.
// The Timestamp field is set automatically if zero.
// Non-blocking per subscriber — full buffers cause a drop.
func (s *EventStream) Emit(ev StreamEvent) {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}

	s.mu.RLock()
	subs := s.subscribers
	closed := s.closed
	s.mu.RUnlock()

	if closed {
		return
	}
	for _, ch := range subs {
		select {
		case ch <- ev:
		default:
			slog.Warn("eventstream: subscriber buffer full, dropping event",
				"kind", ev.Kind, "agent", ev.AgentName)
		}
	}
}

// Close closes all subscriber channels. Idempotent.
func (s *EventStream) Close() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.closed = true
		for _, ch := range s.subscribers {
			close(ch)
		}
	})
}

// SubscriberCount returns the number of active subscribers (for testing/metrics).
func (s *EventStream) SubscriberCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.subscribers)
}
