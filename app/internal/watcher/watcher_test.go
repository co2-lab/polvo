package watcher

import (
	"log/slog"
	"path/filepath"
	"testing"
)

// TestMatchGlob tests the matchGlob function with all documented cases.
func TestMatchGlob(t *testing.T) {
	cases := []struct {
		pattern string
		name    string
		want    bool
		note    string
	}{
		{"*.go", "main.go", true, "simple glob match"},
		{"*.go", "main.ts", false, "simple glob no match"},
		{"**/*.go", "internal/a.go", true, "double-star glob match subdir"},
		{"**/*.go", "a/b/c.go", true, "double-star glob match deep path"},
		{"**/*.go", "main.ts", false, "double-star glob no match wrong ext"},
		{"*.go", "sub/main.go", false, "no double-star, no match in subdir"},
		// BUG: "foo/**" deveria casar com "foo/bar/baz.go" mas splitN("foo/**", "**/", 2)
		// retorna suffix="" e filepath.Match("", candidate) nunca casa — documentar como gap.
		{"foo/**", "foo/bar/baz.go", false, "BUG: foo/** não casa com subdirs na impl atual"},
		{"*", "anything", true, "single star matches anything"},
		// BUG: "**" sem barra não faz match de caminhos com separadores.
		// SplitN("**", "**/", 2) retorna len=1, cai para filepath.Match("**", ...) = false.
		{"**", "any/path/here", false, "BUG: bare ** does not match paths with separators in current impl"},
		{"exact.go", "exact.go", true, "exact match"},
		{"exact.go", "other.go", false, "exact no match"},
	}

	for _, tc := range cases {
		t.Run(tc.pattern+"/"+tc.name, func(t *testing.T) {
			got := matchGlob(tc.pattern, tc.name)
			if got != tc.want {
				t.Errorf("matchGlob(%q, %q) = %v, want %v (%s)", tc.pattern, tc.name, got, tc.want, tc.note)
			}
		})
	}
}

// TestSkipDir tests the skipDir function with all documented cases.
func TestSkipDir(t *testing.T) {
	cases := []struct {
		name string
		want bool
		note string
	}{
		{"node_modules", true, "switch case"},
		{"vendor", true, "switch case"},
		{"__pycache__", true, "switch case"},
		{".git", true, "HasPrefix dot"},
		{"dist", true, "switch case"},
		{"build", true, "switch case"},
		{"target", true, "switch case"},
		{"out", true, "switch case"},
		{"bin", true, "switch case"},
		{".polvo", true, "switch case and HasPrefix dot"},
		{".hidden", true, "HasPrefix dot"},
		{".", false, "explicit exception name != '.'"},
		{"src", false, "not in list"},
		{"internal", false, "not in list"},
		{"cmd", false, "not in list"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := skipDir(tc.name)
			if got != tc.want {
				t.Errorf("skipDir(%q) = %v, want %v (%s)", tc.name, got, tc.want, tc.note)
			}
		})
	}
}

// TestWatcher_Matches tests the Watcher.matches method with includes and excludes.
func TestWatcher_Matches(t *testing.T) {
	t.Run("includes and excludes", func(t *testing.T) {
		root := t.TempDir()
		w := New("test", root, []string{"*.go", "!*_test.go"}, 0, make(chan WatchEvent, 1), slog.Default())

		cases := []struct {
			file string
			want bool
		}{
			{"main.go", true},
			{"main_test.go", false},
			{"main.ts", false},
		}

		for _, tc := range cases {
			got := w.matches(filepath.Join(root, tc.file))
			if got != tc.want {
				t.Errorf("matches(%q) = %v, want %v", tc.file, got, tc.want)
			}
		}
	})

	t.Run("only excludes", func(t *testing.T) {
		root := t.TempDir()
		w := New("test", root, []string{"!vendor/**"}, 0, make(chan WatchEvent, 1), slog.Default())

		cases := []struct {
			file string
			want bool
		}{
			{"main.go", true},         // sem includes → tudo incluso por default
			{"vendor/dep.go", false},   // excluído
		}

		for _, tc := range cases {
			got := w.matches(filepath.Join(root, tc.file))
			if got != tc.want {
				t.Errorf("matches(%q) = %v, want %v", tc.file, got, tc.want)
			}
		}
	})

	t.Run("no patterns", func(t *testing.T) {
		root := t.TempDir()
		w := New("test", root, nil, 0, make(chan WatchEvent, 1), slog.Default())

		files := []string{"anything.go", "readme.md", "Makefile"}
		for _, f := range files {
			got := w.matches(filepath.Join(root, f))
			if !got {
				t.Errorf("matches(%q) = false, want true (no patterns → match everything)", f)
			}
		}
	})
}
