package agent

import (
	"context"
	"testing"
	"time"
)

func TestLoopControl_New(t *testing.T) {
	ctrl := NewLoopControl()
	if ctrl.Interrupt == nil || ctrl.Abort == nil || ctrl.Resume == nil {
		t.Error("NewLoopControl should initialize all channels")
	}
}

func TestCheckInterrupt_NilControl(t *testing.T) {
	l := &Loop{cfg: LoopConfig{}, conv: NewConversation()}
	if l.checkInterrupt(context.Background()) {
		t.Error("nil control should never interrupt")
	}
}

func TestCheckInterrupt_NoSignal(t *testing.T) {
	ctrl := NewLoopControl()
	l := &Loop{cfg: LoopConfig{Control: ctrl}, conv: NewConversation()}
	if l.checkInterrupt(context.Background()) {
		t.Error("no signal should return false")
	}
}

func TestCheckInterrupt_AbortSignal(t *testing.T) {
	ctrl := NewLoopControl()
	close(ctrl.Abort)
	l := &Loop{cfg: LoopConfig{Control: ctrl}, conv: NewConversation()}
	if !l.checkInterrupt(context.Background()) {
		t.Error("abort signal should return true")
	}
}

func TestCheckInterrupt_InterruptThenResume(t *testing.T) {
	ctrl := NewLoopControl()
	close(ctrl.Interrupt)

	l := &Loop{cfg: LoopConfig{Control: ctrl}, conv: NewConversation()}

	// Resume in background after a short delay.
	go func() {
		time.Sleep(10 * time.Millisecond)
		close(ctrl.Resume)
	}()

	if l.checkInterrupt(context.Background()) {
		t.Error("interrupt+resume should return false (loop continues)")
	}
}

func TestCheckInterrupt_InterruptThenAbort(t *testing.T) {
	ctrl := NewLoopControl()
	close(ctrl.Interrupt)

	l := &Loop{cfg: LoopConfig{Control: ctrl}, conv: NewConversation()}

	// Abort while paused.
	go func() {
		time.Sleep(10 * time.Millisecond)
		close(ctrl.Abort)
	}()

	if !l.checkInterrupt(context.Background()) {
		t.Error("interrupt+abort should return true")
	}
}

func TestCheckInterrupt_InterruptThenCtxCancel(t *testing.T) {
	ctrl := NewLoopControl()
	close(ctrl.Interrupt)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	l := &Loop{cfg: LoopConfig{Control: ctrl}, conv: NewConversation()}
	if !l.checkInterrupt(ctx) {
		t.Error("interrupt+ctx cancel should return true")
	}
}
