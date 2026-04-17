package agent

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBus_PublishDelivers(t *testing.T) {
	b := NewAgentBus()
	defer b.Close()

	ch := b.Subscribe("alice")
	b.Publish(AgentMessage{From: "bob", To: "alice", Type: MessageFinding, Payload: "hello"})

	select {
	case msg := <-ch:
		if msg.Payload != "hello" {
			t.Errorf("payload = %q, want hello", msg.Payload)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for message")
	}
}

func TestBus_BroadcastExcludes(t *testing.T) {
	b := NewAgentBus()
	defer b.Close()

	aCh := b.Subscribe("A")
	bCh := b.Subscribe("B")
	cCh := b.Subscribe("C")

	b.Broadcast("A", AgentMessage{Type: MessageFinding, Payload: "broadcast"})

	// B and C should receive; A should not.
	expectMsg := func(ch <-chan AgentMessage, name string, wantMsg bool) {
		t.Helper()
		timer := time.NewTimer(50 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ch:
			if !wantMsg {
				t.Errorf("%s should not receive broadcast from itself", name)
			}
		case <-timer.C:
			if wantMsg {
				t.Errorf("%s did not receive broadcast", name)
			}
		}
	}

	expectMsg(bCh, "B", true)
	expectMsg(cCh, "C", true)
	expectMsg(aCh, "A", false)
}

func TestBus_OrderPreserved(t *testing.T) {
	b := NewAgentBus(WithBusBufferSize(32))
	defer b.Close()

	ch := b.Subscribe("target")
	payloads := []string{"first", "second", "third"}
	for _, p := range payloads {
		b.Publish(AgentMessage{From: "sender", To: "target", Payload: p})
	}

	for _, want := range payloads {
		select {
		case msg := <-ch:
			if msg.Payload != want {
				t.Errorf("expected %q, got %q", want, msg.Payload)
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for %q", want)
		}
	}
}

func TestBus_BufferFullDrops(t *testing.T) {
	// Buffer size 1; fill it, then publish — should not block.
	b := NewAgentBus(WithBusBufferSize(1))
	defer b.Close()

	b.Subscribe("slow")
	b.Publish(AgentMessage{To: "slow", Payload: "fill"})

	done := make(chan struct{})
	go func() {
		b.Publish(AgentMessage{To: "slow", Payload: "overflow"}) // should drop, not block
		close(done)
	}()

	select {
	case <-done:
		// good — did not block
	case <-time.After(500 * time.Millisecond):
		t.Error("Publish blocked on full buffer")
	}
}

func TestBus_CloseIdempotent(t *testing.T) {
	b := NewAgentBus()
	b.Close()
	b.Close() // should not panic
}

func TestBus_SubscribeAfterClose(t *testing.T) {
	b := NewAgentBus()
	b.Close()
	ch := b.Subscribe("late")
	// Channel should be closed.
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected closed channel after bus.Close()")
		}
	default:
		// Closed channels always have a zero-value ready to receive.
		// If default fires, the channel may be blocking — that's a failure.
		t.Error("channel should be closed (immediately readable)")
	}
}

func TestBus_Race(t *testing.T) {
	b := NewAgentBus(WithBusBufferSize(64))
	defer b.Close()

	const goroutines = 10
	const msgsEach = 100

	// Subscribe all agents first.
	for i := 0; i < goroutines; i++ {
		b.Subscribe(agentName(i))
	}

	var wg sync.WaitGroup
	var dropped atomic.Int64

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			from := agentName(idx)
			to := agentName((idx + 1) % goroutines)
			for j := 0; j < msgsEach; j++ {
				b.Publish(AgentMessage{From: from, To: to, Type: MessageFinding, Payload: "x"})
				_ = dropped.Load()
			}
		}(i)
	}
	wg.Wait()
}

func agentName(i int) string {
	return string(rune('A' + i))
}
