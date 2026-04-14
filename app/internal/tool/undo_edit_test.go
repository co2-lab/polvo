package tool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUndoEdit_BasicFlow(t *testing.T) {
	dir := testSetup(t)
	reg := NewUndoRegistry()
	path := filepath.Join(dir, "main.go")
	original := readFile(t, path)

	// Snapshot before editing
	if err := reg.Snapshot(path); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	// Edit the file
	mustWrite(t, path, "package main\n// edited\n")

	// Undo
	undo := NewUndoEdit(dir, reg)
	res := execTool(t, undo, map[string]any{"path": "main.go"})
	assertSuccess(t, res)

	// Confirm restoration
	if got := readFile(t, path); got != original {
		t.Errorf("after undo: got %q, want %q", got, original)
	}
}

func TestUndoEdit_NoHistory(t *testing.T) {
	dir := testSetup(t)
	reg := NewUndoRegistry()
	undo := NewUndoEdit(dir, reg)

	// No snapshot was made
	res := execTool(t, undo, map[string]any{"path": "main.go"})
	// Pattern from openhands-aci: 'No edit history found'
	assertError(t, res, "no edit history")
}

func TestUndoEdit_NewFile(t *testing.T) {
	dir := testSetup(t)
	reg := NewUndoRegistry()
	newPath := filepath.Join(dir, "new_file.go")

	// Snapshot of non-existent path → prev = nil
	if err := reg.Snapshot(newPath); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	// Create the file
	mustWrite(t, newPath, "package main\n")

	// Undo → should delete
	undo := NewUndoEdit(dir, reg)
	res := execTool(t, undo, map[string]any{"path": "new_file.go"})
	assertSuccess(t, res)

	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Fatal("file should have been deleted after undo")
	}
}

func TestUndoEdit_EntryConsumed(t *testing.T) {
	dir := testSetup(t)
	reg := NewUndoRegistry()
	path := filepath.Join(dir, "main.go")

	reg.Snapshot(path)
	mustWrite(t, path, "package main\n// v2\n")

	undo := NewUndoEdit(dir, reg)
	execTool(t, undo, map[string]any{"path": "main.go"}) // first undo: ok

	// Second undo: entry was deleted from the map (undo_edit.go: delete)
	res := execTool(t, undo, map[string]any{"path": "main.go"})
	assertError(t, res, "no edit history")
}
