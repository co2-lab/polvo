// Package ignore loads and evaluates .polvoignore files, similar to .gitignore.
package ignore

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/fsnotify/fsnotify"
)

// Set holds compiled ignore patterns for a workspace root.
type Set struct {
	root     string
	patterns []pattern
}

type pattern struct {
	raw     string
	negate  bool
	dirOnly bool // pattern ends with /
}

// Load reads .polvoignore from root (if present) and returns a Set.
// Missing file is not an error — returns an empty Set.
func Load(root string) (*Set, error) {
	s := &Set{root: filepath.Clean(root)}
	p := filepath.Join(root, ".polvoignore")
	f, err := os.Open(p)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		// Strip inline comments
		if i := strings.Index(line, " #"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimRight(line, " \t")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		s.patterns = append(s.patterns, compile(line))
	}
	return s, scanner.Err()
}

// compile parses a single ignore pattern line.
func compile(line string) pattern {
	p := pattern{raw: line}
	if strings.HasPrefix(line, "!") {
		p.negate = true
		line = line[1:]
	}
	if strings.HasSuffix(line, "/") {
		p.dirOnly = true
		line = strings.TrimSuffix(line, "/")
	}
	p.raw = line
	return p
}

// Ignored returns true if the given absolute path should be ignored.
// Paths outside root are never ignored.
func (s *Set) Ignored(absPath string) bool {
	rel, err := filepath.Rel(s.root, filepath.Clean(absPath))
	if err != nil || strings.HasPrefix(rel, "..") {
		return false // outside root — never ignore
	}
	// Normalise to forward slashes for doublestar matching.
	relSlash := filepath.ToSlash(rel)

	ignored := false
	for _, p := range s.patterns {
		if matches(relSlash, p) {
			ignored = !p.negate
		}
	}
	return ignored
}

// matches checks whether rel (forward-slash path) matches pattern p.
func matches(rel string, p pattern) bool {
	pat := p.raw

	// If pattern contains no /, match against basename and also full path.
	if !strings.Contains(pat, "/") {
		// Match against any path component using the appropriate matcher.
		parts := strings.Split(rel, "/")
		for _, part := range parts {
			if matchSingle(pat, part) {
				return true
			}
		}
		// Also try matching the whole rel path (e.g. a bare ** pattern).
		return matchSingle(pat, rel)
	}

	// Pattern with / — match against full relative path (try both with and
	// without a leading slash stripped).
	pat = strings.TrimPrefix(pat, "/")
	return matchSingle(pat, rel)
}

// matchSingle matches pat against name using doublestar when the pattern
// contains "**", otherwise falls back to filepath.Match semantics via
// doublestar.Match (which is a superset of filepath.Match).
func matchSingle(pat, name string) bool {
	ok, _ := doublestar.Match(pat, name)
	return ok
}

// Empty returns true if there are no patterns.
func (s *Set) Empty() bool { return len(s.patterns) == 0 }

// WatchAndReload watches the .polvoignore file at root for changes.
// Returns the initial Set and starts a background goroutine.
// The returned *atomic.Pointer[Set] always holds the current Set.
func WatchAndReload(ctx context.Context, root string) (*atomic.Pointer[Set], error) {
	initial, err := Load(root)
	if err != nil {
		return nil, err
	}

	ptr := &atomic.Pointer[Set]{}
	ptr.Store(initial)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	ignoreFile := filepath.Join(root, ".polvoignore")

	// Watch the directory so we catch creation of .polvoignore too.
	if err := watcher.Add(filepath.Clean(root)); err != nil {
		watcher.Close()
		return nil, err
	}

	go func() {
		defer watcher.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				// React to any write/create/rename/remove on .polvoignore.
				if filepath.Clean(event.Name) != filepath.Clean(ignoreFile) {
					continue
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) ||
					event.Has(fsnotify.Rename) || event.Has(fsnotify.Remove) {
					if fresh, loadErr := Load(root); loadErr == nil {
						ptr.Store(fresh)
					}
				}
			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
			}
		}
	}()

	return ptr, nil
}
