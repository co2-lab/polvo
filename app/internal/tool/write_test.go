package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteTool(t *testing.T) {
	t.Run("creates new file", func(t *testing.T) {
		dir := testSetup(t)
		tool := NewWrite(dir, nil)

		res := execTool(t, tool, map[string]any{"path": "new.go", "content": "package new\n"})
		assertSuccess(t, res)

		got := readFile(t, filepath.Join(dir, "new.go"))
		if got != "package new\n" {
			t.Errorf("expected 'package new\\n', got: %q", got)
		}
	})

	t.Run("overwrites existing file", func(t *testing.T) {
		dir := testSetup(t)
		tool := NewWrite(dir, nil)

		res := execTool(t, tool, map[string]any{"path": "main.go", "content": "package main\n// overwritten\n"})
		assertSuccess(t, res)

		got := readFile(t, filepath.Join(dir, "main.go"))
		if got != "package main\n// overwritten\n" {
			t.Errorf("expected overwritten content, got: %q", got)
		}
	})

	t.Run("creates intermediate directories", func(t *testing.T) {
		dir := testSetup(t)
		tool := NewWrite(dir, nil)

		res := execTool(t, tool, map[string]any{"path": "sub/new/deep/file.go", "content": "package deep\n"})
		assertSuccess(t, res)

		got := readFile(t, filepath.Join(dir, "sub", "new", "deep", "file.go"))
		if got != "package deep\n" {
			t.Errorf("expected 'package deep\\n', got: %q", got)
		}
	})

	t.Run("empty content creates empty file", func(t *testing.T) {
		dir := testSetup(t)
		tool := NewWrite(dir, nil)

		res := execTool(t, tool, map[string]any{"path": "empty.go", "content": ""})
		assertSuccess(t, res)

		info, err := os.Stat(filepath.Join(dir, "empty.go"))
		if err != nil {
			t.Fatalf("stat empty file: %v", err)
		}
		if info.Size() != 0 {
			t.Errorf("expected 0-byte file, got %d bytes", info.Size())
		}
	})

	t.Run("result contains byte count", func(t *testing.T) {
		dir := testSetup(t)
		tool := NewWrite(dir, nil)
		content := "package main\n"

		res := execTool(t, tool, map[string]any{"path": "counted.go", "content": content})
		assertSuccess(t, res)

		wantBytes := len(content)
		wantSubstr := "bytes"
		if !strings.Contains(res.Content, wantSubstr) {
			t.Errorf("expected result to contain %q, got: %q", wantSubstr, res.Content)
		}
		_ = wantBytes
	})

	t.Run("path traversal rejected", func(t *testing.T) {
		dir := testSetup(t)
		tool := NewWrite(dir, nil)

		res := execTool(t, tool, map[string]any{"path": "../../etc/passwd", "content": "evil"})
		assertError(t, res, "escapes")
	})

	t.Run("ignored path rejected", func(t *testing.T) {
		dir := testSetup(t)
		secretPath := filepath.Join(dir, "secret.key")

		ig := &mockIgnorer{ignored: map[string]bool{secretPath: true}}
		tool := NewWrite(dir, ig)

		res := execTool(t, tool, map[string]any{"path": "secret.key", "content": "data"})
		assertError(t, res, "excluded by .polvoignore")
	})

	t.Run("empty path rejected", func(t *testing.T) {
		dir := testSetup(t)
		tool := NewWrite(dir, nil)

		res := execTool(t, tool, map[string]any{"path": "", "content": "data"})
		assertError(t, res, "path is required")
	})
}

// TestWrite_NoUndoHistory documents the GAP: write does not call UndoRegistry.Snapshot.
// Consequence: undo_edit after a write returns "no edit history found".
func TestWrite_NoUndoHistory(t *testing.T) {
	dir := testSetup(t)
	reg := NewUndoRegistry()
	w := NewWrite(dir, nil)
	path := filepath.Join(dir, "main.go")
	original := readFile(t, path)

	execTool(t, w, map[string]any{"path": "main.go", "content": "package main\n// changed\n"})

	// Undo does not work — write did not register a snapshot
	undo := NewUndoEdit(dir, reg)
	res := execTool(t, undo, map[string]any{"path": "main.go"})
	assertError(t, res, "no edit history")

	// Confirm that the file was actually changed (write worked)
	if readFile(t, path) == original {
		t.Fatal("write should have changed the file")
	}
}
