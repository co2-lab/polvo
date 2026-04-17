package checkpoint

import (
	"encoding/json"
	"testing"
	"time"
)

// TestSaverInterface verifies FSStore satisfies the Saver interface at runtime.
func TestSaverInterface(t *testing.T) {
	store := NewFSStore(t.TempDir())
	var _ Saver = store // compile-time + runtime check
}

// TestPendingWritesRoundTrip exercises PutPendingWrites / LoadPendingWrites / ClearPendingWrites.
func TestPendingWritesRoundTrip(t *testing.T) {
	store := NewFSStore(t.TempDir())
	sessionID := "sess-pw"

	writes := []PendingWrite{
		{Kind: EventUserMessage, Payload: json.RawMessage(`{"content":"hello"}`), Seq: 0},
		{Kind: EventAssistant, Payload: json.RawMessage(`{"content":"world"}`), Seq: 1},
	}

	if err := store.PutPendingWrites(sessionID, writes); err != nil {
		t.Fatalf("PutPendingWrites: %v", err)
	}

	got, err := store.LoadPendingWrites(sessionID)
	if err != nil {
		t.Fatalf("LoadPendingWrites: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 pending writes, got %d", len(got))
	}
	if got[0].Kind != EventUserMessage || got[1].Kind != EventAssistant {
		t.Errorf("unexpected kinds: %v %v", got[0].Kind, got[1].Kind)
	}

	if err := store.ClearPendingWrites(sessionID); err != nil {
		t.Fatalf("ClearPendingWrites: %v", err)
	}
	after, err := store.LoadPendingWrites(sessionID)
	if err != nil {
		t.Fatalf("LoadPendingWrites after clear: %v", err)
	}
	if len(after) != 0 {
		t.Errorf("expected empty after clear, got %d", len(after))
	}
}

// TestPendingWritesAbsent verifies LoadPendingWrites returns nil when file is absent.
func TestPendingWritesAbsent(t *testing.T) {
	store := NewFSStore(t.TempDir())
	writes, err := store.LoadPendingWrites("no-such-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if writes != nil {
		t.Errorf("expected nil, got %v", writes)
	}
}

// TestSuspendPointRoundTrip exercises SaveSuspendPoint / LoadSuspendPoint / DeleteSuspendPoint.
func TestSuspendPointRoundTrip(t *testing.T) {
	store := NewFSStore(t.TempDir())
	sessionID := "sess-sp"

	sp := SuspendPoint{
		CheckpointID: "cp-abc",
		Reason:       SuspendReasonError,
		Prompt:       "implement auth",
		TurnCount:    7,
		SuspendedAt:  time.Now().UnixNano(),
	}

	if err := store.SaveSuspendPoint(sessionID, sp); err != nil {
		t.Fatalf("SaveSuspendPoint: %v", err)
	}

	got, err := store.LoadSuspendPoint(sessionID)
	if err != nil {
		t.Fatalf("LoadSuspendPoint: %v", err)
	}
	if got.CheckpointID != sp.CheckpointID {
		t.Errorf("checkpoint ID mismatch: got %q want %q", got.CheckpointID, sp.CheckpointID)
	}
	if got.Reason != SuspendReasonError {
		t.Errorf("reason mismatch: got %q", got.Reason)
	}
	if got.TurnCount != 7 {
		t.Errorf("turn count mismatch: got %d", got.TurnCount)
	}

	if err := store.DeleteSuspendPoint(sessionID); err != nil {
		t.Fatalf("DeleteSuspendPoint: %v", err)
	}
	// LoadSuspendPoint after delete should return an error (file not found).
	if _, err := store.LoadSuspendPoint(sessionID); err == nil {
		t.Error("expected error loading deleted suspend point, got nil")
	}
}

// TestRecorderPendingWriteFlush verifies BeginPendingWrite / FlushPending flow.
func TestRecorderPendingWriteFlush(t *testing.T) {
	store := NewFSStore(t.TempDir())
	rec, err := NewRecorder(store, "sess-flush", "test-agent")
	if err != nil {
		t.Fatal(err)
	}

	rec.BeginPendingWrite(EventUserMessage, map[string]string{"content": "msg1"})
	rec.BeginPendingWrite(EventAssistant, map[string]string{"content": "msg2"})

	// Before flush, pending writes are in memory — event log has only the base state.
	evts, _ := store.LoadEvents("sess-flush", 0)
	if len(evts) != 0 {
		t.Errorf("expected 0 events before flush, got %d", len(evts))
	}

	if err := rec.FlushPending(); err != nil {
		t.Fatalf("FlushPending: %v", err)
	}

	// After flush, both events are in the log.
	evts, _ = store.LoadEvents("sess-flush", 0)
	if len(evts) != 2 {
		t.Errorf("expected 2 events after flush, got %d", len(evts))
	}

	// Pending writes file is cleared.
	pw, _ := store.LoadPendingWrites("sess-flush")
	if len(pw) != 0 {
		t.Errorf("expected pending writes cleared, got %d", len(pw))
	}
}

// TestRecorderDiscard verifies DiscardPending drops buffered writes.
func TestRecorderDiscard(t *testing.T) {
	store := NewFSStore(t.TempDir())
	rec, err := NewRecorder(store, "sess-discard", "agent")
	if err != nil {
		t.Fatal(err)
	}

	rec.BeginPendingWrite(EventUserMessage, map[string]string{"content": "dropped"})
	rec.DiscardPending()

	evts, _ := store.LoadEvents("sess-discard", 0)
	if len(evts) != 0 {
		t.Errorf("expected 0 events after discard, got %d", len(evts))
	}
}

// TestTurnMarksRoundTrip exercises SaveTurnMarks / LoadTurnMarks.
func TestTurnMarksRoundTrip(t *testing.T) {
	store := NewFSStore(t.TempDir())
	sessionID := "sess-tm"

	marks := []TurnMarkRecord{
		{TurnIndex: 0, Mark: 1, Summary: "very helpful turn"},
		{TurnIndex: 1, Mark: 2, Summary: "irrelevant, dismissed"},
		{TurnIndex: 2, Mark: 0},
	}

	if err := store.SaveTurnMarks(sessionID, marks); err != nil {
		t.Fatalf("SaveTurnMarks: %v", err)
	}

	got, err := store.LoadTurnMarks(sessionID)
	if err != nil {
		t.Fatalf("LoadTurnMarks: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 marks, got %d", len(got))
	}
	if got[0].TurnIndex != 0 || got[0].Mark != 1 || got[0].Summary != "very helpful turn" {
		t.Errorf("mark[0] mismatch: %+v", got[0])
	}
	if got[1].Mark != 2 || got[1].Summary != "irrelevant, dismissed" {
		t.Errorf("mark[1] mismatch: %+v", got[1])
	}
}

// TestTurnMarksAbsent verifies LoadTurnMarks returns nil when file is absent.
func TestTurnMarksAbsent(t *testing.T) {
	store := NewFSStore(t.TempDir())
	marks, err := store.LoadTurnMarks("no-such-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if marks != nil {
		t.Errorf("expected nil, got %v", marks)
	}
}

// TestTurnMarksOverwrite verifies that SaveTurnMarks overwrites previous data atomically.
func TestTurnMarksOverwrite(t *testing.T) {
	store := NewFSStore(t.TempDir())
	sessionID := "sess-tm-overwrite"

	if err := store.SaveTurnMarks(sessionID, []TurnMarkRecord{{TurnIndex: 0, Mark: 1, Summary: "first"}}); err != nil {
		t.Fatal(err)
	}
	// Overwrite with updated marks
	if err := store.SaveTurnMarks(sessionID, []TurnMarkRecord{{TurnIndex: 0, Mark: 2, Summary: "updated"}}); err != nil {
		t.Fatal(err)
	}

	got, err := store.LoadTurnMarks(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Mark != 2 || got[0].Summary != "updated" {
		t.Errorf("expected overwritten mark, got %+v", got)
	}
}

// TestRecorderSuspendResumeEvents verifies RecordSuspend and RecordResume write audit events.
func TestRecorderSuspendResumeEvents(t *testing.T) {
	store := NewFSStore(t.TempDir())
	rec, err := NewRecorder(store, "sess-sr", "agent")
	if err != nil {
		t.Fatal(err)
	}

	if err := rec.RecordSuspend(SuspendReasonError); err != nil {
		t.Fatalf("RecordSuspend: %v", err)
	}
	if err := rec.RecordResume("try a different approach"); err != nil {
		t.Fatalf("RecordResume: %v", err)
	}

	evts, _ := store.LoadEvents("sess-sr", 0)
	if len(evts) != 2 {
		t.Fatalf("expected 2 events, got %d", len(evts))
	}
	if evts[0].Kind != EventSuspend {
		t.Errorf("first event: want %q got %q", EventSuspend, evts[0].Kind)
	}
	if evts[1].Kind != EventResume {
		t.Errorf("second event: want %q got %q", EventResume, evts[1].Kind)
	}
}
