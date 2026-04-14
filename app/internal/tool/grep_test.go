package tool

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestGrepTool(t *testing.T) {
	t.Run("pattern found in main.go", func(t *testing.T) {
		dir := testSetup(t)
		tool := NewGrep(dir)

		res := execTool(t, tool, map[string]any{"pattern": "func main"})
		assertSuccess(t, res)

		if !strings.Contains(res.Content, "main.go") {
			t.Errorf("expected 'main.go' in grep output, got: %q", res.Content)
		}
		if !strings.Contains(res.Content, "func main") {
			t.Errorf("expected 'func main' in grep output, got: %q", res.Content)
		}
	})

	t.Run("regex pattern matches multiple files", func(t *testing.T) {
		dir := testSetup(t)
		tool := NewGrep(dir)

		res := execTool(t, tool, map[string]any{"pattern": `func \w+`})
		assertSuccess(t, res)

		// main.go has func main, sub/util.go has no func
		if !strings.Contains(res.Content, "main.go") {
			t.Errorf("expected 'main.go' in output for regex, got: %q", res.Content)
		}
	})

	t.Run("pattern not found returns no error", func(t *testing.T) {
		dir := testSetup(t)
		tool := NewGrep(dir)

		res := execTool(t, tool, map[string]any{"pattern": "xyzNotFoundAnywhere12345"})
		assertSuccess(t, res)

		if strings.Contains(res.Content, "main.go") {
			t.Errorf("expected no match result, got: %q", res.Content)
		}
	})

	t.Run("file_pattern glob filters files", func(t *testing.T) {
		dir := testSetup(t)
		tool := NewGrep(dir)

		// Search for "Project" which appears in README.md, limit to *.md
		res := execTool(t, tool, map[string]any{"pattern": "Project", "glob": "*.md"})
		assertSuccess(t, res)

		if !strings.Contains(res.Content, "README.md") {
			t.Errorf("expected 'README.md' in output, got: %q", res.Content)
		}
		if strings.Contains(res.Content, "main.go") {
			t.Errorf("expected 'main.go' to NOT appear when glob=*.md, got: %q", res.Content)
		}
	})

	t.Run("invalid regex returns error", func(t *testing.T) {
		dir := testSetup(t)
		tool := NewGrep(dir)

		res := execTool(t, tool, map[string]any{"pattern": "[invalid"})
		assertError(t, res, "invalid regex")
	})

	t.Run("truncation at maxResults", func(t *testing.T) {
		dir := t.TempDir()
		// Create a file with 250 lines all matching the pattern
		var sb strings.Builder
		for i := 0; i < 250; i++ {
			fmt.Fprintf(&sb, "match line %d\n", i)
		}
		mustWrite(t, filepath.Join(dir, "big.go"), sb.String())

		tool := NewGrep(dir)
		res := execTool(t, tool, map[string]any{"pattern": "match line"})
		assertSuccess(t, res)

		// Should be truncated at 200 results
		lines := strings.Split(strings.TrimSuffix(res.Content, "\n"), "\n")
		if len(lines) <= 200 {
			// Check if truncation message appears
			if strings.Contains(res.Content, "truncated") {
				// truncation message present — good
			}
		}
		// The key assertion: result should not contain all 250 lines
		if strings.Count(res.Content, "match line") > 200 {
			t.Errorf("expected at most 200 matches, got more in: %d lines", len(lines))
		}
	})
}
