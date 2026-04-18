package agent

import (
	"log/slog"
	"sync"
	"time"
)

// MessageType classifies an AgentMessage.
type MessageType int

const (
	MessageFinding   MessageType = iota // peer shares a discovery
	MessageQuestion                      // sender needs information from named recipient
	MessageAnswer                        // reply to a Question
	MessageDirective                     // supervisor instructs worker to change approach
	MessageResult                        // worker reports completion to supervisor
)

func (t MessageType) String() string {
	switch t {
	case MessageFinding:
		return "finding"
	case MessageQuestion:
		return "question"
	case MessageAnswer:
		return "answer"
	case MessageDirective:
		return "directive"
	case MessageResult:
		return "result"
	default:
		return "unknown"
	}
}

// AgentMessage is a single message exchanged between agents via AgentBus.
type AgentMessage struct {
	From      string
	To        string      // "" = broadcast
	Type      MessageType
	Payload   string
	Timestamp time.Time
}

// busOption is a functional option for AgentBus.
type busOption func(*AgentBus)

// WithBusBufferSize sets the per-subscriber channel buffer size.
func WithBusBufferSize(n int) busOption {
	return func(b *AgentBus) { b.bufSize = n }
}

// AgentBus is a thread-safe pub/sub hub for named agent channels.
// Messages are delivered non-blocking; full buffers cause the message to be
// dropped with a slog.Warn (prevents deadlocks from slow subscribers).
type AgentBus struct {
	mu       sync.RWMutex
	channels map[string]chan AgentMessage
	bufSize  int
	closed   bool
	closeOnce sync.Once
}

// NewAgentBus creates an AgentBus with default buffer size 16.
func NewAgentBus(opts ...busOption) *AgentBus {
	b := &AgentBus{
		channels: make(map[string]chan AgentMessage),
		bufSize:  16,
	}
	for _, o := range opts {
		o(b)
	}
	return b
}

// Subscribe returns a receive-only channel for the named agent.
// If the bus is already closed, a closed channel is returned.
func (b *AgentBus) Subscribe(agentName string) <-chan AgentMessage {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		ch := make(chan AgentMessage)
		close(ch)
		return ch
	}
	if ch, ok := b.channels[agentName]; ok {
		return ch
	}
	ch := make(chan AgentMessage, b.bufSize)
	b.channels[agentName] = ch
	return ch
}

// Publish sends a message to the agent named in msg.To.
// If msg.To is empty, Publish is a no-op (use Broadcast instead).
// Non-blocking: drops the message if the subscriber's buffer is full.
func (b *AgentBus) Publish(msg AgentMessage) {
	if msg.To == "" {
		return
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	b.mu.RLock()
	ch, ok := b.channels[msg.To]
	closed := b.closed
	b.mu.RUnlock()

	if closed || !ok {
		return
	}
	select {
	case ch <- msg:
	default:
		slog.Warn("agentbus: buffer full, dropping message",
			"from", msg.From, "to", msg.To, "type", msg.Type)
	}
}

// Broadcast sends msg to all agents except msg.From.
// Non-blocking per subscriber.
func (b *AgentBus) Broadcast(from string, msg AgentMessage) {
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}
	msg.From = from
	msg.To = ""

	b.mu.RLock()
	targets := make(map[string]chan AgentMessage, len(b.channels))
	for name, ch := range b.channels {
		if name != from {
			targets[name] = ch
		}
	}
	closed := b.closed
	b.mu.RUnlock()

	if closed {
		return
	}
	for name, ch := range targets {
		select {
		case ch <- msg:
		default:
			slog.Warn("agentbus: broadcast buffer full, dropping",
				"from", from, "to", name, "type", msg.Type)
		}
	}
}

// Close closes all subscriber channels. Idempotent.
func (b *AgentBus) Close() {
	b.closeOnce.Do(func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.closed = true
		for _, ch := range b.channels {
			close(ch)
		}
	})
}
