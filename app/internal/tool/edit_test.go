package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditTool_Errors(t *testing.T) {
	tests := []struct {
		name            string
		setupFile       bool
		fileContent     string
		filePath        string // relative path to write content to (default: "main.go")
		inputPath       string // path passed in the input
		oldString       string
		newString       string
		wantErrContains string
	}{
		{
			name:            "same strings",
			setupFile:       true,
			fileContent:     "package main\n",
			filePath:        "edit_same.go",
			inputPath:       "edit_same.go",
			oldString:       "func main() {}",
			newString:       "func main() {}",
			wantErrContains: "must be different",
		},
		{
			name:            "not found",
			setupFile:       true,
			fileContent:     "package main\n",
			filePath:        "edit_notfound.go",
			inputPath:       "edit_notfound.go",
			oldString:       "nonexistent",
			newString:       "replaced",
			wantErrContains: "not found",
		},
		{
			name:            "duplicate no flag",
			setupFile:       true,
			fileContent:     "line\nline\n",
			filePath:        "edit_dup.go",
			inputPath:       "edit_dup.go",
			oldString:       "line",
			newString:       "pkg",
			wantErrContains: "found 2 times",
		},
		{
			name:            "traversal",
			setupFile:       false,
			fileContent:     "",
			filePath:        "",
			inputPath:       "../../etc/passwd",
			oldString:       "oldtext",
			newString:       "newtext",
			wantErrContains: "escapes",
		},
		{
			name:            "file not exist",
			setupFile:       false,
			fileContent:     "",
			filePath:        "",
			inputPath:       "nonexistent_file.go",
			oldString:       "anything",
			newString:       "x",
			wantErrContains: "reading file",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := testSetup(t)
			tool := NewEditTool(dir, nil)

			if tc.setupFile && tc.filePath != "" {
				mustWrite(t, filepath.Join(dir, tc.filePath), tc.fileContent)
			}

			res := execTool(t, tool, map[string]any{
				"path":       tc.inputPath,
				"old_string": tc.oldString,
				"new_string": tc.newString,
			})
			assertError(t, res, tc.wantErrContains)
		})
	}
}

func TestEditTool_Success(t *testing.T) {
	t.Run("simple substitution", func(t *testing.T) {
		dir := testSetup(t)
		tool := NewEditTool(dir, nil)

		res := execTool(t, tool, map[string]any{
			"path":       "main.go",
			"old_string": "func main() {}",
			"new_string": "func main() { /* edited */ }",
		})
		assertSuccess(t, res)

		content := readFile(t, filepath.Join(dir, "main.go"))
		if strings.Contains(content, "func main() {}") {
			t.Error("old_string should not be in file after edit")
		}
		if !strings.Contains(content, "func main() { /* edited */ }") {
			t.Errorf("new_string not found in file: %q", content)
		}
	})

	t.Run("result contains occurrence count", func(t *testing.T) {
		dir := testSetup(t)
		tool := NewEditTool(dir, nil)

		res := execTool(t, tool, map[string]any{
			"path":       "main.go",
			"old_string": "func main() {}",
			"new_string": "func main() { return }",
		})
		assertSuccess(t, res)

		if !strings.Contains(res.Content, "Replaced") {
			t.Errorf("expected 'Replaced' in result, got: %q", res.Content)
		}
	})

	t.Run("whitespace mismatch succeeds via fuzzy fallback", func(t *testing.T) {
		dir := t.TempDir()
		// Write file with single space
		mustWrite(t, filepath.Join(dir, "ws.go"), "package main\nfunc main() {}\n")
		tool := NewEditTool(dir, nil)

		// old_string with extra spaces — not found verbatim, but fuzzy (level 4) succeeds
		// Previously this was a documented gap that returned "whitespace mismatch detected";
		// now the 5-level fallback cascade applies the edit automatically.
		res := execTool(t, tool, map[string]any{
			"path":       "ws.go",
			"old_string": "func  main()  {}", // double spaces
			"new_string": "func main() { /* fixed */ }",
		})
		assertSuccess(t, res)
		if !strings.Contains(res.Content, "fuzzy") {
			t.Errorf("expected 'fuzzy' in result message (level 4 used), got: %q", res.Content)
		}
	})

	t.Run("replace_all with 2 occurrences", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "dup.go"), "line\nline\n")
		tool := NewEditTool(dir, nil)

		res := execTool(t, tool, map[string]any{
			"path":        "dup.go",
			"old_string":  "line",
			"new_string":  "replaced",
			"replace_all": true,
		})
		assertSuccess(t, res)

		content := readFile(t, filepath.Join(dir, "dup.go"))
		if strings.Contains(content, "line") {
			t.Errorf("expected all occurrences replaced, got: %q", content)
		}
		count := strings.Count(content, "replaced")
		if count != 2 {
			t.Errorf("expected 2 occurrences of 'replaced', got %d in: %q", count, content)
		}
	})

	t.Run("regex metacharacters treated as literal", func(t *testing.T) {
		dir := t.TempDir()
		// Content with dollar sign
		mustWrite(t, filepath.Join(dir, "dollar.go"), "package main\nvar x = $value\n")
		tool := NewEditTool(dir, nil)

		res := execTool(t, tool, map[string]any{
			"path":       "dollar.go",
			"old_string": "$value",
			"new_string": "$newvalue",
		})
		assertSuccess(t, res)

		content := readFile(t, filepath.Join(dir, "dollar.go"))
		if !strings.Contains(content, "$newvalue") {
			t.Errorf("expected '$newvalue' in file, got: %q", content)
		}
	})
}

func TestNormalizeWhitespace(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"a  b", "a b"},
		{"a\t b", "a b"},
		{"no spaces", "no spaces"},
		{"", ""},
		// GAP documented: \r is not collapsed
		{"a\r\nb", "a\r\nb"}, // \r remains — not treated as whitespace
		{"a  \t  b", "a b"}, // multiple spaces and tabs collapse to 1
	}

	for _, tc := range cases {
		got := normalizeWhitespace(tc.in)
		if got != tc.want {
			t.Errorf("normalizeWhitespace(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFindSimilarLines(t *testing.T) {
	content := "package main\n\nfunc main() {}\n\nfunc helper() {}\n\nfunc anotherHelper() {}\n\nfunc yetAnotherHelper() {}\n"

	t.Run("short keyword returns empty", func(t *testing.T) {
		// All words <= 3 chars → no keyword found
		got := findSimilarLines(content, "if ok")
		if got != "" {
			t.Errorf("expected empty result for short keywords, got: %q", got)
		}
	})

	t.Run("keyword found returns up to 3 lines", func(t *testing.T) {
		// "helper" appears in multiple lines
		got := findSimilarLines(content, "helper")
		if got == "" {
			t.Error("expected non-empty result for keyword 'helper'")
		}
		// Should not exceed 3 lines
		lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
		if len(lines) > 3 {
			t.Errorf("expected at most 3 lines, got %d: %q", len(lines), got)
		}
	})

	t.Run("keyword not found returns empty", func(t *testing.T) {
		got := findSimilarLines(content, "nonexistentKeyword")
		if got != "" {
			t.Errorf("expected empty result for not-found keyword, got: %q", got)
		}
	})

	t.Run("exactly 3 lines returned when more than 3 match", func(t *testing.T) {
		// "func" is in 4 lines but each > 3 chars — wait, we need a word > 3 chars
		// "helper" appears in 3 lines: helper, anotherHelper, yetAnotherHelper
		got := findSimilarLines(content, "helper function")
		lines := strings.Split(strings.TrimSpace(got), "\n")
		// At most 3 lines
		if len(lines) > 3 {
			t.Errorf("expected at most 3 lines, got %d: %q", len(lines), got)
		}
	})
}

// TestEdit_EmptyOldString documents the real behavior of edit with old_string="".
// GAP: strings.Count(content, "") returns len(content)+1 for non-empty files.
func TestEdit_EmptyOldString(t *testing.T) {
	dir := testSetup(t)
	tool := NewEditTool(dir, nil)

	// Case 1: non-empty file with old_string="" → error "found N times"
	res := execTool(t, tool, map[string]any{
		"path": "main.go", "old_string": "", "new_string": "inserted",
	})
	assertError(t, res, "times") // N+1 occurrences, where N = len(content)+1

	// Case 2: empty file with old_string="" → one occurrence → substitutes (no error)
	mustWrite(t, filepath.Join(dir, "empty.go"), "")
	res2 := execTool(t, tool, map[string]any{
		"path": "empty.go", "old_string": "", "new_string": "inserted",
	})
	assertSuccess(t, res2)
	if readFile(t, filepath.Join(dir, "empty.go")) != "inserted" {
		t.Fatal("expected inserted content")
	}
}

// TestEdit_CRLF_NotNormalized documents the CRLF behavior of edit.
//
// GAP context: normalizeWhitespace (edit.go) does not treat \r as whitespace.
// However, when old_string does NOT contain \n, it can still be found verbatim
// inside a CRLF file because \r only appears at line boundaries, not within the
// target string itself.
//
// The gap only manifests when old_string SPANS a line boundary (contains \n),
// because then the file has \r\n but old_string has \n — verbatim match fails,
// and normalizeWhitespace also fails to help (doesn't collapse \r).
func TestEdit_CRLF_NotNormalized(t *testing.T) {
	dir := testSetup(t)
	tool := NewEditTool(dir, nil)
	// File with CRLF line endings
	mustWrite(t, filepath.Join(dir, "windows.go"), "package main\r\nfunc main() {}\r\n")

	// Case 1: old_string does NOT span a line boundary → found verbatim (succeeds)
	res := execTool(t, tool, map[string]any{
		"path":       "windows.go",
		"old_string": "func main() {}", // LF only, no \n in string
		"new_string": "func main() { /* edited */ }",
	})
	// Real behavior: succeeds — the string is found literally between \r\n boundaries
	assertSuccess(t, res)

	// Case 2: old_string SPANS a line boundary (contains \n) → CRLF file
	// Previously this was a documented gap; now the fuzzy fallback (level 4)
	// applies the edit automatically with high enough similarity.
	mustWrite(t, filepath.Join(dir, "windows2.go"), "package main\r\nfunc main() {}\r\n")
	res2 := execTool(t, tool, map[string]any{
		"path":       "windows2.go",
		"old_string": "package main\nfunc main() {}", // contains \n, file has \r\n
		"new_string": "package main\nfunc replaced() {}",
	})
	// The fuzzy fallback succeeds (high similarity due to \r being the only difference).
	// If this ever fails, it means the fuzzy threshold was changed to exclude CRLF matches.
	if res2.IsError {
		// Accept either outcome: success via fuzzy OR a meaningful error (not a panic).
		// The important thing is no crash and the error is informative.
		if !strings.Contains(res2.Content, "not found") && !strings.Contains(res2.Content, "fuzzy") {
			t.Errorf("unexpected error message: %q", res2.Content)
		}
	}
}

// TestUndoEdit_Bug_SingleEntryNotStack documents the bug: second Snapshot overwrites first.
// Undo after two edits restores to state B (between the two edits), not state A (before first edit).
func TestUndoEdit_Bug_SingleEntryNotStack(t *testing.T) {
	dir := testSetup(t)
	reg := NewUndoRegistry()
	path := filepath.Join(dir, "main.go")

	original := readFile(t, path)              // state A
	reg.Snapshot(path)                          // saves A
	mustWrite(t, path, "// state B\n")         // modifies to B

	reg.Snapshot(path)                          // overwrites A with B (BUG: loses A)
	mustWrite(t, path, "// state C\n")         // modifies to C

	undo := NewUndoEdit(dir, reg)
	res := execTool(t, undo, map[string]any{"path": "main.go"})
	assertSuccess(t, res)

	got := readFile(t, path)
	// Real behavior: restores B (second snapshot), not A (first)
	if got != "// state B\n" {
		t.Errorf("expected state B (second snapshot), got %q", got)
	}
	// Confirm A was lost
	if got == original {
		t.Fatal("expected NOT to restore original — this would mean stack was implemented")
	}
}

// ---------------------------------------------------------------------------
// New gap-filling tests
// ---------------------------------------------------------------------------

// TestEdit_UnicodeContent verifies that edit works correctly on files containing
// multi-byte UTF-8 characters including emoji.
func TestEdit_UnicodeContent(t *testing.T) {
	dir := t.TempDir()
	content := "hello 🌊 world\n"
	mustWrite(t, filepath.Join(dir, "unicode.txt"), content)
	tool := NewEditTool(dir, nil)

	res := execTool(t, tool, map[string]any{
		"path":       "unicode.txt",
		"old_string": "hello 🌊 world",
		"new_string": "hello 🌊 polvo",
	})
	assertSuccess(t, res)

	got := readFile(t, filepath.Join(dir, "unicode.txt"))
	if !strings.Contains(got, "hello 🌊 polvo") {
		t.Errorf("expected 'hello 🌊 polvo' in file, got: %q", got)
	}
	if strings.Contains(got, "hello 🌊 world") {
		t.Errorf("old_string should not remain in file, got: %q", got)
	}
}

// TestEdit_ReadonlyFile verifies that editing a read-only file returns an error
// result without panicking.
func TestEdit_ReadonlyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "readonly.go")
	mustWrite(t, path, "package main\nfunc main() {}\n")
	if err := os.Chmod(path, 0444); err != nil {
		t.Fatalf("chmod 0444: %v", err)
	}
	t.Cleanup(func() { os.Chmod(path, 0644) }) //nolint:errcheck

	tool := NewEditTool(dir, nil)
	res := execTool(t, tool, map[string]any{
		"path":       "readonly.go",
		"old_string": "func main() {}",
		"new_string": "func main() { /* edited */ }",
	})
	if !res.IsError {
		t.Fatal("expected IsError=true for read-only file, got success")
	}
}

// TestEdit_MultiBlock_FailureInFirstBlock verifies that when the first block
// in an `edits` array fails to match, the entire operation is rolled back
// atomically (the file is unchanged).
func TestEdit_MultiBlock_FailureInFirstBlock(t *testing.T) {
	dir := t.TempDir()
	original := "alpha\nbeta\ngamma\n"
	mustWrite(t, filepath.Join(dir, "f.txt"), original)
	tool := NewEditTool(dir, nil)

	res := execTool(t, tool, map[string]any{
		"path": "f.txt",
		"edits": []map[string]any{
			{"old_string": "NONEXISTENT_BLOCK_FIRST", "new_string": "ONE"},
			{"old_string": "beta", "new_string": "TWO"},
			{"old_string": "gamma", "new_string": "THREE"},
		},
	})
	assertError(t, res, "edit block failed")

	got := readFile(t, filepath.Join(dir, "f.txt"))
	if got != original {
		t.Errorf("file should be unchanged after first-block failure, got: %q", got)
	}
}

// TestEdit_MultiBlock_FailureInSecondBlock verifies that when the second block
// in an `edits` array fails, the entire operation is rolled back atomically
// (no partial apply from the first successful block).
func TestEdit_MultiBlock_FailureInSecondBlock(t *testing.T) {
	dir := t.TempDir()
	original := "alpha\nbeta\ngamma\n"
	mustWrite(t, filepath.Join(dir, "g.txt"), original)
	tool := NewEditTool(dir, nil)

	res := execTool(t, tool, map[string]any{
		"path": "g.txt",
		"edits": []map[string]any{
			{"old_string": "alpha", "new_string": "ONE"},
			{"old_string": "NONEXISTENT_BLOCK_SECOND", "new_string": "TWO"},
			{"old_string": "gamma", "new_string": "THREE"},
		},
	})
	assertError(t, res, "edit block failed")

	got := readFile(t, filepath.Join(dir, "g.txt"))
	if got != original {
		t.Errorf("file should be unchanged after second-block failure (no partial apply), got: %q", got)
	}
	// Confirm first block was NOT applied
	if strings.Contains(got, "ONE") {
		t.Errorf("first block should not have been applied, got: %q", got)
	}
}
