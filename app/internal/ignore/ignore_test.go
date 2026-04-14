package ignore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeIgnore(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".polvoignore"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestIgnoreBasicPattern(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, "*.secret\n# comment\n.env\n")
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		path    string
		ignored bool
	}{
		{filepath.Join(dir, "api.secret"), true},
		{filepath.Join(dir, ".env"), true},
		{filepath.Join(dir, "main.go"), false},
		{filepath.Join(dir, "sub", "api.secret"), true},
	}
	for _, c := range cases {
		got := s.Ignored(c.path)
		if got != c.ignored {
			t.Errorf("Ignored(%q) = %v, want %v", c.path, got, c.ignored)
		}
	}
}

func TestIgnoreNegation(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, "*.log\n!important.log\n")
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Ignored(filepath.Join(dir, "debug.log")) {
		t.Error("debug.log should be ignored")
	}
	if s.Ignored(filepath.Join(dir, "important.log")) {
		t.Error("important.log should NOT be ignored (negated)")
	}
}

func TestIgnoreMissingFile(t *testing.T) {
	dir := t.TempDir()
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Ignored(filepath.Join(dir, "anything.go")) {
		t.Error("empty set should not ignore anything")
	}
}

func TestIgnoreOutsideRoot(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, "*.go\n")
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Path outside root is never ignored
	if s.Ignored("/tmp/outside.go") {
		t.Error("path outside root should not be ignored")
	}
}

// TestIgnoreDoublestar verifies that ** glob patterns work correctly.
func TestIgnoreDoublestar(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, "src/**/redis/**\n")
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		path    string
		ignored bool
	}{
		// deeply nested under src/.../redis/
		{filepath.Join(dir, "src", "pkg", "cache", "redis", "client.go"), true},
		{filepath.Join(dir, "src", "redis", "conn.go"), true},
		// not under redis
		{filepath.Join(dir, "src", "pkg", "cache", "memcache", "client.go"), false},
		// outside src
		{filepath.Join(dir, "lib", "redis", "client.go"), false},
	}
	for _, c := range cases {
		got := s.Ignored(c.path)
		if got != c.ignored {
			t.Errorf("Ignored(%q) = %v, want %v", c.path, got, c.ignored)
		}
	}
}

// TestIgnoreCharacterClass verifies that bracket/character-class patterns work.
func TestIgnoreCharacterClass(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, "[abc]*.go\n")
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		path    string
		ignored bool
	}{
		{filepath.Join(dir, "abc.go"), true},
		{filepath.Join(dir, "afile.go"), true},
		{filepath.Join(dir, "btest.go"), true},
		{filepath.Join(dir, "xyz.go"), false},
		{filepath.Join(dir, "main.go"), false},
	}
	for _, c := range cases {
		got := s.Ignored(c.path)
		if got != c.ignored {
			t.Errorf("Ignored(%q) = %v, want %v", c.path, got, c.ignored)
		}
	}
}

// TestWatchAndReload verifies that modifying .polvoignore causes the atomic
// pointer to be updated with the new patterns.
func TestWatchAndReload(t *testing.T) {
	dir := t.TempDir()

	// Start with a file that ignores *.secret
	writeIgnore(t, dir, "*.secret\n")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ptr, err := WatchAndReload(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	testFile := filepath.Join(dir, "notes.txt")

	// Initially notes.txt should NOT be ignored.
	if ptr.Load().Ignored(testFile) {
		t.Fatal("notes.txt should not be ignored initially")
	}

	// Rewrite .polvoignore to also ignore *.txt
	writeIgnore(t, dir, "*.secret\n*.txt\n")

	// Poll until the hot-reload picks up the change (up to 5 s).
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ptr.Load().Ignored(testFile) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !ptr.Load().Ignored(testFile) {
		t.Error("hot-reload: notes.txt should be ignored after .polvoignore update")
	}
}
