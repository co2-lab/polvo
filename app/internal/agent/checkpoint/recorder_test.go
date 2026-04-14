package checkpoint

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRecorderRecordsMessages(t *testing.T) {
	store := NewFSStore(t.TempDir())
	sessionID := "sess-rec"

	rec, err := NewRecorder(store, sessionID, "test-agent")
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}

	if err := rec.RecordMessage(EventUserMessage, "hello"); err != nil {
		t.Fatalf("RecordMessage(user): %v", err)
	}
	if err := rec.RecordMessage(EventAssistant, "hi there"); err != nil {
		t.Fatalf("RecordMessage(assistant): %v", err)
	}

	events, err := store.LoadEvents(sessionID, 0)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Kind != EventUserMessage {
		t.Errorf("events[0].Kind = %s, want %s", events[0].Kind, EventUserMessage)
	}
	if events[1].Kind != EventAssistant {
		t.Errorf("events[1].Kind = %s, want %s", events[1].Kind, EventAssistant)
	}

	// Verify payload content.
	var p MessagePayload
	if err := json.Unmarshal(events[0].Payload, &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if p.Content != "hello" {
		t.Errorf("user message content = %q, want %q", p.Content, "hello")
	}
}

func TestRecorderRecordsToolCallAndResult(t *testing.T) {
	store := NewFSStore(t.TempDir())
	sessionID := "sess-tools"

	rec, err := NewRecorder(store, sessionID, "tool-agent")
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}

	input := json.RawMessage(`{"path":"/tmp/foo"}`)
	if err := rec.RecordToolCall("read_file", input); err != nil {
		t.Fatalf("RecordToolCall: %v", err)
	}
	if err := rec.RecordToolResult("read_file", "file contents", false); err != nil {
		t.Fatalf("RecordToolResult: %v", err)
	}

	events, err := store.LoadEvents(sessionID, 0)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	if events[0].Kind != EventToolCall {
		t.Errorf("events[0].Kind = %s, want %s", events[0].Kind, EventToolCall)
	}
	if events[1].Kind != EventToolResult {
		t.Errorf("events[1].Kind = %s, want %s", events[1].Kind, EventToolResult)
	}

	var tc ToolCallPayload
	if err := json.Unmarshal(events[0].Payload, &tc); err != nil {
		t.Fatalf("unmarshal tool call payload: %v", err)
	}
	if tc.ToolName != "read_file" {
		t.Errorf("tool name = %q, want %q", tc.ToolName, "read_file")
	}
}

func TestRecorderCreateCheckpoint(t *testing.T) {
	storeDir := t.TempDir()
	store := NewFSStore(storeDir)
	sessionID := "sess-cp"

	rec, err := NewRecorder(store, sessionID, "cp-agent")
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}

	if err := rec.RecordMessage(EventUserMessage, "do something"); err != nil {
		t.Fatalf("RecordMessage: %v", err)
	}
	if err := rec.RecordMessage(EventAssistant, "doing it"); err != nil {
		t.Fatalf("RecordMessage: %v", err)
	}

	// Build a files snapshot.
	content := base64.StdEncoding.EncodeToString([]byte("file content"))
	snapshot := map[string]string{"main.go": content}

	cpID, err := rec.CreateCheckpoint("after first exchange", snapshot)
	if err != nil {
		t.Fatalf("CreateCheckpoint: %v", err)
	}
	if cpID == "" {
		t.Fatal("checkpoint ID is empty")
	}

	// Verify the checkpoint file exists.
	cpPath := filepath.Join(storeDir, "sessions", sessionID, "checkpoints", cpID+".json")
	if _, err := os.Stat(cpPath); err != nil {
		t.Fatalf("checkpoint file not found: %v", err)
	}

	// Load and verify checkpoint fields.
	cp, err := store.LoadCheckpoint(cpID)
	if err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if cp.SessionID != sessionID {
		t.Errorf("SessionID = %q, want %q", cp.SessionID, sessionID)
	}
	if cp.Description != "after first exchange" {
		t.Errorf("Description = %q, want %q", cp.Description, "after first exchange")
	}
	if cp.EventIndex != 1 {
		// Two messages recorded (index 0 and 1); checkpoint should reference index 1.
		t.Errorf("EventIndex = %d, want 1", cp.EventIndex)
	}
	if cp.FilesSnapshot["main.go"] != content {
		t.Errorf("FilesSnapshot mismatch")
	}

	// After CreateCheckpoint, a checkpoint_marker event should also have been appended.
	events, err := store.LoadEvents(sessionID, 0)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	found := false
	for _, e := range events {
		if e.Kind == EventCheckpoint {
			found = true
			break
		}
	}
	if !found {
		t.Error("no checkpoint_marker event found after CreateCheckpoint")
	}
}

func TestRecorderFinish(t *testing.T) {
	store := NewFSStore(t.TempDir())
	sessionID := "sess-finish"

	rec, err := NewRecorder(store, sessionID, "finish-agent")
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}

	if err := rec.Finish("completed"); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	state, err := store.LoadBaseState(sessionID)
	if err != nil {
		t.Fatalf("LoadBaseState: %v", err)
	}
	if state.Status != "completed" {
		t.Errorf("Status = %q, want %q", state.Status, "completed")
	}
}

func TestRecorderFileModified(t *testing.T) {
	store := NewFSStore(t.TempDir())
	sessionID := "sess-filemod"

	rec, err := NewRecorder(store, sessionID, "file-agent")
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}

	if err := rec.RecordFileModified("/project/main.go"); err != nil {
		t.Fatalf("RecordFileModified: %v", err)
	}

	events, err := store.LoadEvents(sessionID, 0)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Kind != EventFileModified {
		t.Errorf("Kind = %s, want %s", events[0].Kind, EventFileModified)
	}

	var p FileModifiedPayload
	if err := json.Unmarshal(events[0].Payload, &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if p.Path != "/project/main.go" {
		t.Errorf("Path = %q, want %q", p.Path, "/project/main.go")
	}
}
