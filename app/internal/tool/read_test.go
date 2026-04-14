package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadTool(t *testing.T) {
	t.Run("existing file with line numbers", func(t *testing.T) {
		dir := testSetup(t)
		tool := NewRead(dir, nil)

		res := execTool(t, tool, map[string]any{"path": "main.go"})
		assertSuccess(t, res)

		if !strings.Contains(res.Content, "   1\t") {
			t.Errorf("expected line number format '   1\\t', got: %q", res.Content)
		}
		if !strings.Contains(res.Content, "package main") {
			t.Errorf("expected 'package main' in output, got: %q", res.Content)
		}
	})

	t.Run("line number format single line", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "one.go"), "package main")
		tool := NewRead(dir, nil)

		res := execTool(t, tool, map[string]any{"path": "one.go"})
		assertSuccess(t, res)

		if !strings.HasPrefix(res.Content, "   1\tpackage main\n") {
			t.Errorf("expected '   1\\tpackage main\\n', got: %q", res.Content)
		}
	})

	t.Run("offset and limit", func(t *testing.T) {
		dir := t.TempDir()
		// 5-line file: lines 1..5
		mustWrite(t, filepath.Join(dir, "multi.go"), "line1\nline2\nline3\nline4\nline5\n")
		tool := NewRead(dir, nil)

		// offset=1 (0-based), limit=2 => lines 2 and 3
		res := execTool(t, tool, map[string]any{"path": "multi.go", "offset": 1, "limit": 2})
		assertSuccess(t, res)

		if !strings.Contains(res.Content, "line2") {
			t.Errorf("expected 'line2' in output, got: %q", res.Content)
		}
		if !strings.Contains(res.Content, "line3") {
			t.Errorf("expected 'line3' in output, got: %q", res.Content)
		}
		if strings.Contains(res.Content, "line4") {
			t.Errorf("expected 'line4' to NOT be in output, got: %q", res.Content)
		}
	})

	t.Run("offset beyond end", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "short.go"), "line1\nline2\n")
		tool := NewRead(dir, nil)

		res := execTool(t, tool, map[string]any{"path": "short.go", "offset": 100})
		assertSuccess(t, res)

		if res.Content != "" {
			t.Errorf("expected empty content for offset beyond end, got: %q", res.Content)
		}
	})

	t.Run("limit zero uses default 2000", func(t *testing.T) {
		dir := testSetup(t)
		tool := NewRead(dir, nil)

		res := execTool(t, tool, map[string]any{"path": "main.go", "limit": 0})
		assertSuccess(t, res)

		if !strings.Contains(res.Content, "package main") {
			t.Errorf("expected content with limit=0 (default 2000), got: %q", res.Content)
		}
	})

	t.Run("empty file", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "empty.go"), "")
		tool := NewRead(dir, nil)

		res := execTool(t, tool, map[string]any{"path": "empty.go"})
		assertSuccess(t, res)
		// Empty file is valid — no error
	})

	t.Run("file not found", func(t *testing.T) {
		dir := testSetup(t)
		tool := NewRead(dir, nil)

		res := execTool(t, tool, map[string]any{"path": "missing.go"})
		assertError(t, res, "reading file")
	})

	t.Run("binary file with invalid UTF-8 bytes", func(t *testing.T) {
		dir := t.TempDir()
		// Write binary content with bytes that are invalid UTF-8 sequences.
		// 0xFF and 0xFE are never valid in UTF-8 — utf8.Valid returns false for these.
		// Note: \x00 alone IS valid UTF-8 (Go's utf8.Valid treats it as a valid byte).
		binPath := filepath.Join(dir, "file.bin")
		if err := os.WriteFile(binPath, []byte{0xFF, 0xFE, 0x00, 0x01}, 0644); err != nil {
			t.Fatalf("writing binary file: %v", err)
		}
		tool := NewRead(dir, nil)

		res := execTool(t, tool, map[string]any{"path": "file.bin"})
		assertError(t, res, "binary")
	})

	t.Run("path traversal rejected", func(t *testing.T) {
		dir := testSetup(t)
		tool := NewRead(dir, nil)

		res := execTool(t, tool, map[string]any{"path": "../../etc/passwd"})
		assertError(t, res, "escapes")
	})

	t.Run("ignored path", func(t *testing.T) {
		dir := testSetup(t)
		secretPath := filepath.Join(dir, "secret.key")
		mustWrite(t, secretPath, "secret content")

		ig := &mockIgnorer{ignored: map[string]bool{secretPath: true}}
		tool := NewRead(dir, ig)

		res := execTool(t, tool, map[string]any{"path": "secret.key"})
		assertError(t, res, "excluded by .polvoignore")
	})
}
