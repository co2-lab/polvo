package checkpoint

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// makeCheckpoint is a helper that saves a checkpoint and returns its ID.
func makeCheckpoint(t *testing.T, store *FSStore, sessionID string, eventIndex int, snapshot map[string]string) string {
	t.Helper()
	id, err := newUUID()
	if err != nil {
		t.Fatalf("newUUID: %v", err)
	}
	c := Checkpoint{
		ID:            id,
		SessionID:     sessionID,
		EventIndex:    eventIndex,
		Timestamp:     time.Now().UnixNano(),
		Description:   "test checkpoint",
		FilesSnapshot: snapshot,
	}
	if err := store.SaveCheckpoint(c); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}
	return id
}

func TestRestoreFiles(t *testing.T) {
	storeDir := t.TempDir()
	workdir := t.TempDir()
	store := NewFSStore(storeDir)
	restorer := NewRestorer(store)
	sessionID := "sess-restore"

	originalContent := []byte("package main\n\nfunc main() {}\n")
	encoded := base64.StdEncoding.EncodeToString(originalContent)

	checkpointID := makeCheckpoint(t, store, sessionID, 0, map[string]string{
		"main.go": encoded,
	})

	// Simulate modifying the file after checkpoint.
	modifiedPath := filepath.Join(workdir, "main.go")
	if err := os.WriteFile(modifiedPath, []byte("package main // modified"), 0o644); err != nil {
		t.Fatalf("writing modified file: %v", err)
	}

	// Restore files.
	if err := restorer.RestoreFiles(checkpointID, workdir); err != nil {
		t.Fatalf("RestoreFiles: %v", err)
	}

	// Verify file content matches original.
	got, err := os.ReadFile(modifiedPath)
	if err != nil {
		t.Fatalf("reading restored file: %v", err)
	}
	if string(got) != string(originalContent) {
		t.Errorf("restored content = %q, want %q", got, originalContent)
	}
}

func TestRestoreFilesCreatesSubdirectory(t *testing.T) {
	storeDir := t.TempDir()
	workdir := t.TempDir()
	store := NewFSStore(storeDir)
	restorer := NewRestorer(store)
	sessionID := "sess-subdir"

	content := []byte("nested content")
	encoded := base64.StdEncoding.EncodeToString(content)

	checkpointID := makeCheckpoint(t, store, sessionID, 0, map[string]string{
		"pkg/util/helper.go": encoded,
	})

	if err := restorer.RestoreFiles(checkpointID, workdir); err != nil {
		t.Fatalf("RestoreFiles: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(workdir, "pkg", "util", "helper.go"))
	if err != nil {
		t.Fatalf("reading restored nested file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("nested file content mismatch: got %q, want %q", got, content)
	}
}

func TestLoadConversationHistory(t *testing.T) {
	storeDir := t.TempDir()
	store := NewFSStore(storeDir)
	restorer := NewRestorer(store)
	sessionID := "sess-history"

	// Append 5 events.
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

	// Create checkpoint at event index 2 (0-based, i.e. 3rd event).
	checkpointID := makeCheckpoint(t, store, sessionID, 2, nil)

	history, err := restorer.LoadConversationHistory(checkpointID)
	if err != nil {
		t.Fatalf("LoadConversationHistory: %v", err)
	}

	// Should return events 0, 1, 2 (indices 0..2 inclusive).
	if len(history) != 3 {
		t.Fatalf("expected 3 events, got %d", len(history))
	}
	for _, e := range history {
		if e.Index > 2 {
			t.Errorf("history contains event with index %d > checkpoint EventIndex 2", e.Index)
		}
	}
}
