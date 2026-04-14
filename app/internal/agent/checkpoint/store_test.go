package checkpoint

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestAppendAndLoadEvents(t *testing.T) {
	store := NewFSStore(t.TempDir())
	sessionID := "sess-1"

	kinds := []EventKind{EventUserMessage, EventAssistant, EventToolCall}
	for _, k := range kinds {
		payload, _ := json.Marshal(MessagePayload{Content: string(k)})
		e := Event{
			Kind:      k,
			Timestamp: time.Now().UnixNano(),
			Payload:   json.RawMessage(payload),
		}
		if err := store.AppendEvent(sessionID, e); err != nil {
			t.Fatalf("AppendEvent(%s): %v", k, err)
		}
	}

	events, err := store.LoadEvents(sessionID, 0)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	if len(events) != len(kinds) {
		t.Fatalf("expected %d events, got %d", len(kinds), len(events))
	}

	// Verify order.
	for i, e := range events {
		if e.Index != i {
			t.Errorf("event[%d].Index = %d, want %d", i, e.Index, i)
		}
		if e.Kind != kinds[i] {
			t.Errorf("event[%d].Kind = %s, want %s", i, e.Kind, kinds[i])
		}
	}
}

func TestLoadEventsFromIndex(t *testing.T) {
	store := NewFSStore(t.TempDir())
	sessionID := "sess-from"

	for i := 0; i < 5; i++ {
		payload, _ := json.Marshal(MessagePayload{Content: "msg"})
		e := Event{
			Kind:      EventUserMessage,
			Timestamp: time.Now().UnixNano(),
			Payload:   json.RawMessage(payload),
		}
		if err := store.AppendEvent(sessionID, e); err != nil {
			t.Fatalf("AppendEvent: %v", err)
		}
	}

	events, err := store.LoadEvents(sessionID, 2)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events from index 2, got %d", len(events))
	}
	if events[0].Index != 2 {
		t.Errorf("first event index = %d, want 2", events[0].Index)
	}
}

func TestSaveAndLoadCheckpoint(t *testing.T) {
	store := NewFSStore(t.TempDir())

	c := Checkpoint{
		ID:            "cp-abc123",
		ParentID:      "",
		SessionID:     "sess-cp",
		EventIndex:    7,
		Timestamp:     time.Now().UnixNano(),
		Description:   "after tool run",
		FilesSnapshot: map[string]string{"main.go": "aGVsbG8="},
		ConversationLen: 5,
	}
	if err := store.SaveCheckpoint(c); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	got, err := store.LoadCheckpoint(c.ID)
	if err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}

	if got.ID != c.ID {
		t.Errorf("ID mismatch: got %s, want %s", got.ID, c.ID)
	}
	if got.EventIndex != c.EventIndex {
		t.Errorf("EventIndex mismatch: got %d, want %d", got.EventIndex, c.EventIndex)
	}
	if got.Description != c.Description {
		t.Errorf("Description mismatch: got %q, want %q", got.Description, c.Description)
	}
	if got.FilesSnapshot["main.go"] != c.FilesSnapshot["main.go"] {
		t.Errorf("FilesSnapshot mismatch")
	}
}

func TestListCheckpointsChronological(t *testing.T) {
	store := NewFSStore(t.TempDir())
	sessionID := "sess-list"

	base := time.Now().UnixNano()
	for i := 0; i < 3; i++ {
		id, err := newUUID()
		if err != nil {
			t.Fatalf("newUUID: %v", err)
		}
		c := Checkpoint{
			ID:        id,
			SessionID: sessionID,
			Timestamp: base + int64(i)*1000,
		}
		if err := store.SaveCheckpoint(c); err != nil {
			t.Fatalf("SaveCheckpoint: %v", err)
		}
	}

	list, err := store.ListCheckpoints(sessionID)
	if err != nil {
		t.Fatalf("ListCheckpoints: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 checkpoints, got %d", len(list))
	}
	for i := 1; i < len(list); i++ {
		if list[i].Timestamp < list[i-1].Timestamp {
			t.Errorf("checkpoints not in chronological order at index %d", i)
		}
	}
}

func TestMultipleSessionsDoNotInterfere(t *testing.T) {
	store := NewFSStore(t.TempDir())

	for _, sid := range []string{"sess-A", "sess-B"} {
		payload, _ := json.Marshal(MessagePayload{Content: sid})
		e := Event{
			Kind:      EventUserMessage,
			Timestamp: time.Now().UnixNano(),
			Payload:   json.RawMessage(payload),
		}
		if err := store.AppendEvent(sid, e); err != nil {
			t.Fatalf("AppendEvent(%s): %v", sid, err)
		}
	}

	for _, sid := range []string{"sess-A", "sess-B"} {
		events, err := store.LoadEvents(sid, 0)
		if err != nil {
			t.Fatalf("LoadEvents(%s): %v", sid, err)
		}
		if len(events) != 1 {
			t.Errorf("session %s: expected 1 event, got %d", sid, len(events))
		}
		var p MessagePayload
		if err := json.Unmarshal(events[0].Payload, &p); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if p.Content != sid {
			t.Errorf("session %s: payload content = %q, want %q", sid, p.Content, sid)
		}
	}
}

// ---------------------------------------------------------------------------
// New gap-coverage tests
// ---------------------------------------------------------------------------

// TestFSStore_CorruptedEventFile verifies behavior when one event file in the
// session directory contains invalid JSON. Current implementation returns an
// error from LoadEvents (fail-closed — it does not silently skip corrupt files).
func TestFSStore_CorruptedEventFile(t *testing.T) {
	store := NewFSStore(t.TempDir())
	sessionID := "sess-corrupt"

	// Write one valid event.
	payload, _ := json.Marshal(MessagePayload{Content: "ok"})
	e := Event{
		Kind:      EventUserMessage,
		Timestamp: time.Now().UnixNano(),
		Payload:   json.RawMessage(payload),
	}
	if err := store.AppendEvent(sessionID, e); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	// Inject a corrupt file alongside the valid one.
	// The filename format must parse as a valid sequence number so LoadEvents
	// attempts to read it. We use index 1 (1-based seq=0002).
	id, err := newUUID()
	if err != nil {
		t.Fatalf("newUUID: %v", err)
	}
	corruptPath := store.eventsDir(sessionID) + "/0002-" + id + ".json"
	if err := os.WriteFile(corruptPath, []byte("{invalid json"), 0o644); err != nil {
		t.Fatalf("writing corrupt file: %v", err)
	}

	// LoadEvents should return an error for the corrupted entry.
	_, loadErr := store.LoadEvents(sessionID, 0)
	if loadErr == nil {
		t.Error("expected LoadEvents to return an error for a corrupted event file, got nil")
	}
	// Document current behavior: fail-closed (returns error, does not skip).
}

// TestFSStore_IdempotentRestore verifies that calling RestoreFiles twice with
// the same checkpoint produces identical results and no error on the second call.
func TestFSStore_IdempotentRestore(t *testing.T) {
	storeDir := t.TempDir()
	workdir := t.TempDir()
	store := NewFSStore(storeDir)
	restorer := NewRestorer(store)
	sessionID := "sess-idempotent"

	content := []byte("idempotent content")
	encoded := base64.StdEncoding.EncodeToString(content)
	checkpointID := makeCheckpoint(t, store, sessionID, 0, map[string]string{
		"idempotent.go": encoded,
	})

	for i := 0; i < 2; i++ {
		if err := restorer.RestoreFiles(checkpointID, workdir); err != nil {
			t.Fatalf("RestoreFiles call %d: %v", i+1, err)
		}
	}

	got, err := os.ReadFile(filepath.Join(workdir, "idempotent.go"))
	if err != nil {
		t.Fatalf("reading restored file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("restored content after second call = %q, want %q", got, content)
	}
}

// TestFSStore_ListCheckpoints_Sorted creates 3 checkpoints with different
// Timestamps and verifies ListCheckpoints returns them sorted ascending.
func TestFSStore_ListCheckpoints_Sorted(t *testing.T) {
	store := NewFSStore(t.TempDir())
	sessionID := "sess-sorted"

	base := time.Now().UnixNano()
	// Create in reverse order so the sort is non-trivial.
	for i := 2; i >= 0; i-- {
		id, err := newUUID()
		if err != nil {
			t.Fatalf("newUUID: %v", err)
		}
		c := Checkpoint{
			ID:        id,
			SessionID: sessionID,
			Timestamp: base + int64(i)*1_000_000,
		}
		if err := store.SaveCheckpoint(c); err != nil {
			t.Fatalf("SaveCheckpoint: %v", err)
		}
	}

	list, err := store.ListCheckpoints(sessionID)
	if err != nil {
		t.Fatalf("ListCheckpoints: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 checkpoints, got %d", len(list))
	}
	for i := 1; i < len(list); i++ {
		if list[i].Timestamp < list[i-1].Timestamp {
			t.Errorf("checkpoints not sorted ascending at index %d: ts[%d]=%d < ts[%d]=%d",
				i, i, list[i].Timestamp, i-1, list[i-1].Timestamp)
		}
	}
}

// TestRecorder_ConcurrentAppend runs 20 goroutines each calling RecordMessage
// simultaneously and verifies the total event count matches. Run with -race.
func TestRecorder_ConcurrentAppend(t *testing.T) {
	t.Parallel()

	store := NewFSStore(t.TempDir())
	sessionID := "sess-concurrent"

	rec, err := NewRecorder(store, sessionID, "concurrent-agent")
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}

	const goroutines = 20
	errCh := make(chan error, goroutines)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			errCh <- rec.RecordMessage(EventUserMessage, fmt.Sprintf("msg-%d", n))
		}(i)
	}
	wg.Wait()
	close(errCh)

	for e := range errCh {
		if e != nil {
			t.Errorf("RecordMessage error: %v", e)
		}
	}

	events, err := store.LoadEvents(sessionID, 0)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	if len(events) != goroutines {
		t.Errorf("expected %d events, got %d", goroutines, len(events))
	}
}

// TestRestorer_DirectoryCreated verifies that RestoreFiles creates the parent
// directory when it does not exist.
func TestRestorer_DirectoryCreated(t *testing.T) {
	storeDir := t.TempDir()
	// workdir does NOT exist yet; we use a subdirectory of TempDir that we
	// never create ourselves.
	workdir := filepath.Join(t.TempDir(), "new-workdir")
	store := NewFSStore(storeDir)
	restorer := NewRestorer(store)
	sessionID := "sess-newdir"

	content := []byte("hello from new dir")
	encoded := base64.StdEncoding.EncodeToString(content)
	checkpointID := makeCheckpoint(t, store, sessionID, 0, map[string]string{
		"hello.go": encoded,
	})

	if err := restorer.RestoreFiles(checkpointID, workdir); err != nil {
		t.Fatalf("RestoreFiles: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(workdir, "hello.go"))
	if err != nil {
		t.Fatalf("reading restored file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("restored content = %q, want %q", got, content)
	}
}

// TestCheckpoint_ParentIDChain creates checkpoint A (no parent), then
// checkpoint B with ParentID = A.ID, and verifies both are stored and
// that the parent chain is correct.
func TestCheckpoint_ParentIDChain(t *testing.T) {
	store := NewFSStore(t.TempDir())
	sessionID := "sess-chain"

	idA, err := newUUID()
	if err != nil {
		t.Fatalf("newUUID A: %v", err)
	}
	cpA := Checkpoint{
		ID:        idA,
		ParentID:  "",
		SessionID: sessionID,
		Timestamp: time.Now().UnixNano(),
	}
	if err := store.SaveCheckpoint(cpA); err != nil {
		t.Fatalf("SaveCheckpoint A: %v", err)
	}

	idB, err := newUUID()
	if err != nil {
		t.Fatalf("newUUID B: %v", err)
	}
	cpB := Checkpoint{
		ID:        idB,
		ParentID:  idA,
		SessionID: sessionID,
		Timestamp: time.Now().UnixNano() + 1000,
	}
	if err := store.SaveCheckpoint(cpB); err != nil {
		t.Fatalf("SaveCheckpoint B: %v", err)
	}

	loadedB, err := store.LoadCheckpoint(idB)
	if err != nil {
		t.Fatalf("LoadCheckpoint B: %v", err)
	}
	if loadedB.ParentID != idA {
		t.Errorf("B.ParentID = %q, want %q", loadedB.ParentID, idA)
	}

	loadedA, err := store.LoadCheckpoint(idA)
	if err != nil {
		t.Fatalf("LoadCheckpoint A: %v", err)
	}
	if loadedA.ParentID != "" {
		t.Errorf("A.ParentID = %q, want empty string", loadedA.ParentID)
	}
}

func TestSaveAndLoadBaseState(t *testing.T) {
	store := NewFSStore(t.TempDir())
	sessionID := "sess-state"

	state := BaseState{
		SessionID: sessionID,
		AgentName: "test-agent",
		StartedAt: time.Now().UnixNano(),
		UpdatedAt: time.Now().UnixNano(),
		Status:    "running",
	}
	if err := store.SaveBaseState(sessionID, state); err != nil {
		t.Fatalf("SaveBaseState: %v", err)
	}

	got, err := store.LoadBaseState(sessionID)
	if err != nil {
		t.Fatalf("LoadBaseState: %v", err)
	}
	if got.AgentName != state.AgentName {
		t.Errorf("AgentName mismatch: got %q, want %q", got.AgentName, state.AgentName)
	}
	if got.Status != state.Status {
		t.Errorf("Status mismatch: got %q, want %q", got.Status, state.Status)
	}
}
