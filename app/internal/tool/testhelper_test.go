package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testSetup creates a standard fixture directory with known files.
func testSetup(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")
	mustWrite(t, filepath.Join(dir, "README.md"), "# Project\n")
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	mustWrite(t, filepath.Join(dir, "sub", "util.go"), "package sub\n")
	return dir
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("mustWrite %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readFile %s: %v", path, err)
	}
	return string(data)
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mustJSON: %v", err)
	}
	return b
}

func execTool(t *testing.T, tool Tool, input any) *Result {
	t.Helper()
	res, err := tool.Execute(context.Background(), mustJSON(t, input))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	return res
}

func assertError(t *testing.T, res *Result, wantContains string) {
	t.Helper()
	if !res.IsError {
		t.Fatalf("expected error result, got success: %s", res.Content)
	}
	if !strings.Contains(res.Content, wantContains) {
		t.Errorf("error content %q does not contain %q", res.Content, wantContains)
	}
}

func assertSuccess(t *testing.T, res *Result) {
	t.Helper()
	if res.IsError {
		t.Fatalf("expected success result, got error: %s", res.Content)
	}
}

// mockIgnorer implements Ignorer for tests without depending on the ignore package.
type mockIgnorer struct{ ignored map[string]bool }

func (m *mockIgnorer) Ignored(path string) bool { return m.ignored[path] }
