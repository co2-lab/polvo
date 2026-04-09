package server

import (
	"encoding/json"
	"os"
	"sync"
)

const logRingSize = 200

// Bus is a thread-safe fan-out event publisher with a recent-log ring buffer.
type Bus struct {
	mu          sync.RWMutex
	subscribers map[int]chan Event
	nextID      int

	logMu   sync.RWMutex
	logRing []string

	agentMu sync.RWMutex
	agents  map[string]*AgentStatus // key: file path

	watchingMu sync.RWMutex
	watching   bool
}

// NewBus creates an empty Bus.
func NewBus() *Bus {
	return &Bus{
		subscribers: make(map[int]chan Event),
		agents:      make(map[string]*AgentStatus),
	}
}

// Publish sends an event to all current subscribers and updates internal state.
func (b *Bus) Publish(e Event) {
	b.updateState(e)

	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subscribers {
		select {
		case ch <- e:
		default:
			// subscriber too slow — drop rather than block
		}
	}
}

// Subscribe returns a channel that receives all future events and an unsubscribe
// function. Call unsub when done to avoid leaks.
func (b *Bus) Subscribe() (<-chan Event, func()) {
	b.mu.Lock()
	id := b.nextID
	b.nextID++
	ch := make(chan Event, 64)
	b.subscribers[id] = ch
	b.mu.Unlock()

	unsub := func() {
		b.mu.Lock()
		delete(b.subscribers, id)
		close(ch)
		b.mu.Unlock()
	}
	return ch, unsub
}

// Snapshot returns the current state for new SSE subscribers.
func (b *Bus) Snapshot() SnapshotPayload {
	b.logMu.RLock()
	log := make([]string, len(b.logRing))
	copy(log, b.logRing)
	b.logMu.RUnlock()

	b.agentMu.RLock()
	var agents []*AgentStatus
	for _, a := range b.agents {
		cp := *a
		agents = append(agents, &cp)
	}
	b.agentMu.RUnlock()

	b.watchingMu.RLock()
	watching := b.watching
	b.watchingMu.RUnlock()

	cwd, _ := os.Getwd()
	return SnapshotPayload{
		Status:    StatusResponse{Watching: watching, Cwd: cwd},
		Agents:    agents,
		RecentLog: log,
	}
}

// Watching returns whether the watcher is currently active.
func (b *Bus) Watching() bool {
	b.watchingMu.RLock()
	defer b.watchingMu.RUnlock()
	return b.watching
}

// ActiveAgents returns a copy of all currently tracked agent statuses.
func (b *Bus) ActiveAgents() []*AgentStatus {
	b.agentMu.RLock()
	defer b.agentMu.RUnlock()
	out := make([]*AgentStatus, 0, len(b.agents))
	for _, a := range b.agents {
		cp := *a
		out = append(out, &cp)
	}
	return out
}

func (b *Bus) updateState(e Event) {
	switch e.Kind {
	case EventLogLine:
		var p struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(e.Payload, &p) == nil && p.Text != "" {
			b.logMu.Lock()
			b.logRing = append(b.logRing, p.Text)
			if len(b.logRing) > logRingSize {
				b.logRing = b.logRing[len(b.logRing)-logRingSize:]
			}
			b.logMu.Unlock()
		}
	case EventAgentStarted:
		var p AgentStatus
		if json.Unmarshal(e.Payload, &p) == nil {
			b.agentMu.Lock()
			b.agents[p.File] = &p
			b.agentMu.Unlock()
		}
	case EventAgentDone:
		var p AgentStatus
		if json.Unmarshal(e.Payload, &p) == nil {
			b.agentMu.Lock()
			if a, ok := b.agents[p.File]; ok {
				a.Done = true
				a.Error = p.Error
			}
			b.agentMu.Unlock()
		}
	case EventWatchStarted:
		b.watchingMu.Lock()
		b.watching = true
		b.watchingMu.Unlock()
	case EventWatchStopped:
		b.watchingMu.Lock()
		b.watching = false
		b.watchingMu.Unlock()
	}
}
