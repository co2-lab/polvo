package agent

import (
	"testing"
)

func TestDrainSteer_Nil(t *testing.T) {
	l := &Loop{cfg: LoopConfig{SteerCh: nil}, conv: NewConversation()}
	l.drainSteer() // must not panic
}

func TestDrainSteer_Empty(t *testing.T) {
	ch := make(chan string, 4)
	l := &Loop{cfg: LoopConfig{SteerCh: ch}, conv: NewConversation()}
	l.drainSteer() // nothing in channel — must not block
	if len(l.conv.Messages()) != 0 {
		t.Errorf("expected 0 messages, got %d", len(l.conv.Messages()))
	}
}

func TestDrainSteer_InjectsMessages(t *testing.T) {
	ch := make(chan string, 4)
	ch <- "focus on auth.go"
	ch <- "ignore ui files"
	l := &Loop{cfg: LoopConfig{SteerCh: ch}, conv: NewConversation()}
	l.drainSteer()

	msgs := l.conv.Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 injected messages, got %d", len(msgs))
	}
	if msgs[0].Content != "focus on auth.go" {
		t.Errorf("unexpected first message: %q", msgs[0].Content)
	}
	if msgs[1].Content != "ignore ui files" {
		t.Errorf("unexpected second message: %q", msgs[1].Content)
	}
}

func TestDrainSteer_SkipsEmpty(t *testing.T) {
	ch := make(chan string, 4)
	ch <- ""           // empty — should be skipped
	ch <- "real steer" // should be injected
	l := &Loop{cfg: LoopConfig{SteerCh: ch}, conv: NewConversation()}
	l.drainSteer()

	msgs := l.conv.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 injected message, got %d", len(msgs))
	}
	if msgs[0].Content != "real steer" {
		t.Errorf("got %q", msgs[0].Content)
	}
}

func TestDrainSteer_EmitsToEventStream(t *testing.T) {
	ch := make(chan string, 4)
	ch <- "steer me"

	es := NewEventStream(4)
	defer es.Close()
	evCh := es.Subscribe()

	l := &Loop{
		cfg: LoopConfig{
			SteerCh:     ch,
			EventStream: es,
			Model:       "test",
		},
		conv: NewConversation(),
	}
	l.drainSteer()

	select {
	case ev := <-evCh:
		if ev.Kind != EventTurnStart {
			t.Errorf("expected EventTurnStart, got %v", ev.Kind)
		}
	default:
		t.Error("expected an event to be emitted")
	}
}
