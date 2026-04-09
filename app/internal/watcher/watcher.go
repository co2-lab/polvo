// Package watcher monitors the filesystem for changes.
package watcher

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	ignore "github.com/sabhiram/go-gitignore"
)

// Event represents a detected file change.
type Event struct {
	Path string
	Op   string // "create", "modify", "delete"
}

// Handler is called when a file change is detected.
type Handler func(Event)

// Watcher monitors directories for file changes.
type Watcher struct {
	root      string
	patterns  []string
	handler   Handler
	debouncer *Debouncer
	logger    *slog.Logger
	gitignore *ignore.GitIgnore
	done      chan struct{}
}

// New creates a new file watcher.
func New(root string, patterns []string, debounceMs int, handler Handler, logger *slog.Logger) *Watcher {
	// Load .gitignore if it exists
	gi, _ := ignore.CompileIgnoreFile(filepath.Join(root, ".gitignore"))

	return &Watcher{
		root:      root,
		patterns:  patterns,
		handler:   handler,
		debouncer: NewDebouncer(time.Duration(debounceMs) * time.Millisecond),
		logger:    logger,
		gitignore: gi,
		done:      make(chan struct{}),
	}
}

// Start begins watching for file changes. Blocks until Stop is called.
func (w *Watcher) Start() error {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("creating fsnotify watcher: %w", err)
	}
	defer fsw.Close()

	// Walk directory tree and add all directories
	_ = filepath.WalkDir(w.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// Always skip on error — never abort the walk
			return filepath.SkipDir
		}
		if !d.IsDir() {
			return nil
		}
		name := d.Name()
		if skipDir(name) {
			return filepath.SkipDir
		}
		if w.isIgnored(path) {
			return filepath.SkipDir
		}
		if err := fsw.Add(path); err != nil {
			w.logger.Debug("skip watch dir", "path", path, "error", err)
			return filepath.SkipDir
		}
		return nil
	})

	w.logger.Info("watching for changes", "root", w.root, "patterns", w.patterns)

	for {
		select {
		case event, ok := <-fsw.Events:
			if !ok {
				return nil
			}
			w.handleFSEvent(event)
		case err, ok := <-fsw.Errors:
			if !ok {
				return nil
			}
			w.logger.Error("watcher error", "error", err)
		case <-w.done:
			return nil
		}
	}
}

// Stop signals the watcher to stop.
func (w *Watcher) Stop() {
	close(w.done)
}

func (w *Watcher) handleFSEvent(event fsnotify.Event) {
	if w.isIgnored(event.Name) {
		return
	}
	if !w.matchesPattern(event.Name) {
		return
	}

	var op string
	switch {
	case event.Op.Has(fsnotify.Create):
		op = "create"
	case event.Op.Has(fsnotify.Write):
		op = "modify"
	case event.Op.Has(fsnotify.Remove):
		op = "delete"
	default:
		return
	}

	w.debouncer.Debounce(event.Name, func() {
		w.logger.Info("file changed", "path", event.Name, "op", op)
		w.handler(Event{Path: event.Name, Op: op})
	})
}

// isIgnored returns true if the path matches .gitignore rules.
func (w *Watcher) isIgnored(path string) bool {
	if w.gitignore == nil {
		return false
	}
	rel, err := filepath.Rel(w.root, path)
	if err != nil {
		return false
	}
	return w.gitignore.MatchesPath(rel)
}

// skipDir returns true for directories that should never be watched.
func skipDir(name string) bool {
	if strings.HasPrefix(name, ".") && name != "." {
		return true
	}
	switch name {
	case "node_modules", "vendor", "__pycache__", "dist", "build",
		"target", "out", "bin", ".polvo":
		return true
	}
	return false
}

func (w *Watcher) matchesPattern(path string) bool {
	rel, err := filepath.Rel(w.root, path)
	if err != nil {
		return false
	}

	for _, pattern := range w.patterns {
		matched, _ := filepath.Match(pattern, rel)
		if matched {
			return true
		}
		// Also try matching just the filename
		matched, _ = filepath.Match(pattern, filepath.Base(rel))
		if matched {
			return true
		}
	}
	return false
}
