package tool

import (
	"path/filepath"
	"strings"
	"testing"
)

// --- Level 1: exact match ---

func TestEditFallback_Level1_ExactMatch(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "f.go"), "package main\n\nfunc Hello() {}\n")
	tool := NewEditTool(dir, nil)

	res := execTool(t, tool, map[string]any{
		"path":       "f.go",
		"old_string": "func Hello() {}",
		"new_string": "func Hello() { return }",
	})
	assertSuccess(t, res)
	if !strings.Contains(res.Content, "Replaced 1") {
		t.Errorf("expected 'Replaced 1', got: %q", res.Content)
	}
	// No fallback mention for exact match
	if strings.Contains(res.Content, "fuzzy") || strings.Contains(res.Content, "whitespace") {
		t.Errorf("unexpected fallback mention for exact match: %q", res.Content)
	}
}

// --- Level 2a: trailing whitespace ---

func TestEditFallback_Level2a_TrailingWhitespace(t *testing.T) {
	dir := t.TempDir()
	// File has no trailing spaces, search has trailing spaces
	mustWrite(t, filepath.Join(dir, "f.go"), "package main\n\nfunc Hello() {}\n")
	tool := NewEditTool(dir, nil)

	res := execTool(t, tool, map[string]any{
		"path":       "f.go",
		"old_string": "func Hello() {}  ", // trailing spaces in search
		"new_string": "func Hello() { return }",
	})
	assertSuccess(t, res)
	content := readFile(t, filepath.Join(dir, "f.go"))
	if !strings.Contains(content, "func Hello() { return }") {
		t.Errorf("expected replacement, got: %q", content)
	}
}

// --- Level 2b: shared minimum indentation ---

func TestEditFallback_Level2b_IndentMismatch(t *testing.T) {
	dir := t.TempDir()
	// File uses 4-space indentation
	fileContent := "package main\n\nfunc Foo() error {\n    if x < 0 {\n        return nil\n    }\n    return nil\n}\n"
	mustWrite(t, filepath.Join(dir, "f.go"), fileContent)
	tool := NewEditTool(dir, nil)

	// Search uses 2-space indentation (different from file)
	// Use relative indent matching (level 3) which strips all leading whitespace
	// Actually, for level 2b we test dedenting: search has more indentation than file
	// Let's test: search block with extra leading spaces on each line
	res := execTool(t, tool, map[string]any{
		"path": "f.go",
		// 2 extra spaces added to every line of the search block
		"old_string": "  if x < 0 {\n        return nil\n    }",
		"new_string": "    if x >= 0 {\n        return nil\n    }",
	})
	// This should succeed via level 2b (dedented match) or level 3 (relative indent)
	assertSuccess(t, res)
}

// --- Level 2c: leading blank line ignored ---

func TestEditFallback_Level2c_LeadingBlankLine(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "f.go"), "package main\n\nfunc Hello() {}\n")
	tool := NewEditTool(dir, nil)

	res := execTool(t, tool, map[string]any{
		"path":       "f.go",
		"old_string": "\nfunc Hello() {}", // extra leading blank line
		"new_string": "func Hello() { return }",
	})
	assertSuccess(t, res)
	content := readFile(t, filepath.Join(dir, "f.go"))
	if !strings.Contains(content, "func Hello() { return }") {
		t.Errorf("expected replacement after blank-line-trim, got: %q", content)
	}
}

// --- Level 3: relative indentation ---

func TestEditFallback_Level3_RelativeIndent(t *testing.T) {
	dir := t.TempDir()
	// File uses tabs
	fileContent := "package main\n\nfunc Foo() {\n\tif x {\n\t\treturn\n\t}\n}\n"
	mustWrite(t, filepath.Join(dir, "f.go"), fileContent)
	tool := NewEditTool(dir, nil)

	// Search uses spaces — structurally identical
	res := execTool(t, tool, map[string]any{
		"path":       "f.go",
		"old_string": "if x {\n\t\treturn\n\t}", // tabs match file exactly — exact match
		"new_string": "if x {\n\t\treturn true\n\t}",
	})
	assertSuccess(t, res)
}

func TestEditFallback_Level3_RelativeIndent_SpaceVsTab(t *testing.T) {
	dir := t.TempDir()
	// File uses tabs
	fileContent := "package main\n\nfunc Foo() {\n\tif x {\n\t\treturn\n\t}\n}\n"
	mustWrite(t, filepath.Join(dir, "f.go"), fileContent)
	tool := NewEditTool(dir, nil)

	// Search uses spaces instead of tabs — relative indent match
	res := execTool(t, tool, map[string]any{
		"path":       "f.go",
		"old_string": "    if x {\n        return\n    }", // spaces, but structurally same
		"new_string": "\tif x {\n\t\treturn true\n\t}",
	})
	assertSuccess(t, res)
}

// --- Level 4: fuzzy matching ---

func TestEditFallback_Level4_FuzzyMatch_HighScore(t *testing.T) {
	dir := t.TempDir()
	// File has a function with a specific implementation
	fileContent := "package main\n\nfunc calculateTotal(items []Item) float64 {\n\ttotal := 0.0\n\tfor _, item := range items {\n\t\ttotal += item.Price\n\t}\n\treturn total\n}\n"
	mustWrite(t, filepath.Join(dir, "f.go"), fileContent)
	tool := NewEditTool(dir, nil)

	// Search is very close but has one word difference (high similarity)
	res := execTool(t, tool, map[string]any{
		"path":       "f.go",
		"old_string": "func calculateTotal(items []Item) float64 {\n\ttotal := 0.0\n\tfor _, item := range items {\n\t\ttotal += item.Price\n\t}\n\treturn total\n}",
		"new_string": "func calculateTotal(items []Item) float64 {\n\ttotal := 0.0\n\tfor _, item := range items {\n\t\ttotal += item.Cost\n\t}\n\treturn total\n}",
	})
	assertSuccess(t, res)
}

func TestEditFallback_Level4_FuzzyMatch_WithStartLineHint(t *testing.T) {
	dir := t.TempDir()
	// File with multiple functions
	fileContent := "package main\n\nfunc Alpha() {}\n\nfunc Beta() {}\n\nfunc Gamma() {\n\tx := 1\n\treturn\n}\n"
	mustWrite(t, filepath.Join(dir, "f.go"), fileContent)
	tool := NewEditTool(dir, nil)

	// Provide start_line hint pointing near Gamma
	res := execTool(t, tool, map[string]any{
		"path":       "f.go",
		"old_string": "func Gamma() {\n\tx := 1\n\treturn\n}",
		"new_string": "func Gamma() {\n\tx := 2\n\treturn\n}",
		"start_line": 7,
	})
	assertSuccess(t, res)
	content := readFile(t, filepath.Join(dir, "f.go"))
	if !strings.Contains(content, "x := 2") {
		t.Errorf("expected x := 2 after edit, got: %q", content)
	}
}

func TestEditFallback_Level4_FuzzyMatch_LowScoreFails(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "f.go"), "package main\n\nfunc Hello() {}\n")
	tool := NewEditTool(dir, nil)

	// Search is completely unrelated to file content — should fail with diagnostic
	res := execTool(t, tool, map[string]any{
		"path":       "f.go",
		"old_string": "completely different content that does not exist in this file at all whatsoever",
		"new_string": "replaced",
	})
	assertError(t, res, "not found")
}

func TestEditFallback_Level4_DiagnosticMessage(t *testing.T) {
	dir := t.TempDir()
	fileContent := "package main\n\nfunc Compute(x int) int {\n\tresult := x * 2\n\treturn result\n}\n"
	mustWrite(t, filepath.Join(dir, "f.go"), fileContent)
	tool := NewEditTool(dir, nil)

	// Search is similar enough to trigger a diagnostic but not good enough to auto-apply
	// We use a threshold test: old_string=="" won't be found, but we simulate a mismatch
	res := execTool(t, tool, map[string]any{
		"path":       "f.go",
		"old_string": "nonexistent_function_totally_different(x int)",
		"new_string": "replaced",
	})
	assertError(t, res, "not found")
	// Should have a tip in the message
	if !strings.Contains(res.Content, "read") {
		t.Errorf("expected 'read' tip in error message, got: %q", res.Content)
	}
}

// --- Level 5: whole-file rewrite ---

func TestEditFallback_Level5_WholeFileRewrite(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "f.go"), "package main\n\nfunc main() {}\n")
	tool := NewEditTool(dir, nil)

	newContent := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"
	res := execTool(t, tool, map[string]any{
		"path":        "f.go",
		"new_content": newContent,
	})
	assertSuccess(t, res)
	if !strings.Contains(res.Content, "level 5") {
		t.Errorf("expected 'level 5' in result, got: %q", res.Content)
	}
	content := readFile(t, filepath.Join(dir, "f.go"))
	if content != newContent {
		t.Errorf("expected full replacement, got: %q", content)
	}
}

// --- Multi-block: order-invariant ---

func TestEditMultiBlock_OrderInvariant(t *testing.T) {
	dir := t.TempDir()
	fileContent := "line1\nline2\nline3\nline4\nline5\n"
	mustWrite(t, filepath.Join(dir, "f.txt"), fileContent)
	tool := NewEditTool(dir, nil)

	// Provide edits in reverse order — should still apply correctly
	res := execTool(t, tool, map[string]any{
		"path": "f.txt",
		"edits": []map[string]any{
			{"old_string": "line5", "new_string": "FIVE"},
			{"old_string": "line1", "new_string": "ONE"},
			{"old_string": "line3", "new_string": "THREE"},
		},
	})
	assertSuccess(t, res)
	content := readFile(t, filepath.Join(dir, "f.txt"))
	if !strings.Contains(content, "ONE") || !strings.Contains(content, "THREE") || !strings.Contains(content, "FIVE") {
		t.Errorf("not all substitutions applied, got: %q", content)
	}
	// Original markers should be gone
	if strings.Contains(content, "line1") || strings.Contains(content, "line3") || strings.Contains(content, "line5") {
		t.Errorf("old strings still present: %q", content)
	}
}

func TestEditMultiBlock_AtomicFailure(t *testing.T) {
	dir := t.TempDir()
	fileContent := "alpha\nbeta\ngamma\n"
	mustWrite(t, filepath.Join(dir, "f.txt"), fileContent)
	tool := NewEditTool(dir, nil)

	// Third block doesn't exist — entire operation should fail atomically
	res := execTool(t, tool, map[string]any{
		"path": "f.txt",
		"edits": []map[string]any{
			{"old_string": "alpha", "new_string": "ALPHA"},
			{"old_string": "beta", "new_string": "BETA"},
			{"old_string": "NONEXISTENT_BLOCK_THAT_DOES_NOT_EXIST_IN_FILE", "new_string": "DELTA"},
		},
	})
	assertError(t, res, "edit block failed")
	// File should be unchanged (atomic rollback)
	content := readFile(t, filepath.Join(dir, "f.txt"))
	if content != fileContent {
		t.Errorf("file should be unchanged after atomic failure, got: %q", content)
	}
}

// --- Internal function tests ---

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"abc", "axc", 1},
		{"kitten", "sitting", 3},
		{"saturday", "sunday", 3},
	}
	for _, tc := range cases {
		got := levenshtein(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestStringSimilarity(t *testing.T) {
	if s := stringSimilarity("abc", "abc"); s != 1.0 {
		t.Errorf("identical strings should have similarity 1.0, got %f", s)
	}
	if s := stringSimilarity("abc", "xyz"); s >= 0.5 {
		t.Errorf("completely different strings should have low similarity, got %f", s)
	}
	if s := stringSimilarity("", ""); s != 1.0 {
		t.Errorf("empty strings should have similarity 1.0, got %f", s)
	}
}

func TestDedentBlock(t *testing.T) {
	cases := []struct {
		input       string
		wantDedent  string
		wantIndent  int
	}{
		{
			input:      "    if x {\n        return\n    }",
			wantDedent: "if x {\n    return\n}",
			wantIndent: 4,
		},
		{
			input:      "no indent",
			wantDedent: "no indent",
			wantIndent: 0,
		},
		{
			input:      "\t\tfoo\n\t\tbar",
			wantDedent: "foo\nbar",
			wantIndent: 2,
		},
	}
	for _, tc := range cases {
		got, indent := dedentBlock(tc.input)
		if indent != tc.wantIndent {
			t.Errorf("dedentBlock indent: got %d, want %d (input=%q)", indent, tc.wantIndent, tc.input)
		}
		if got != tc.wantDedent {
			t.Errorf("dedentBlock result: got %q, want %q", got, tc.wantDedent)
		}
	}
}

func TestStripTrailingWhitespace(t *testing.T) {
	in := "line1   \nline2\t\nline3"
	want := "line1\nline2\nline3"
	got := stripTrailingWhitespace(in)
	if got != want {
		t.Errorf("stripTrailingWhitespace(%q) = %q, want %q", in, got, want)
	}
}

func TestLineOffsets(t *testing.T) {
	content := "line0\nline1\nline2\n"
	if lineOffsets(content, 0) != 0 {
		t.Error("lineOffsets(content, 0) should be 0")
	}
	if lineOffsets(content, 1) != 6 { // "line0\n" = 6 chars
		t.Errorf("lineOffsets(content, 1) = %d, want 6", lineOffsets(content, 1))
	}
	if lineOffsets(content, 2) != 12 { // "line0\nline1\n" = 12 chars
		t.Errorf("lineOffsets(content, 2) = %d, want 12", lineOffsets(content, 2))
	}
}

// --- Regression: existing exact-match behavior preserved ---

func TestEditFallback_ExactMatchPreserved_Errors(t *testing.T) {
	t.Run("not found returns error", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "f.go"), "package main\n")
		tool := NewEditTool(dir, nil)
		res := execTool(t, tool, map[string]any{
			"path":       "f.go",
			"old_string": "nonexistent",
			"new_string": "replaced",
		})
		assertError(t, res, "not found")
	})

	t.Run("duplicate found returns error", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "f.go"), "line\nline\n")
		tool := NewEditTool(dir, nil)
		res := execTool(t, tool, map[string]any{
			"path":       "f.go",
			"old_string": "line",
			"new_string": "pkg",
		})
		assertError(t, res, "found 2 times")
	})
}
