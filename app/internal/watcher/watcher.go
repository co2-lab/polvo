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

const maxFileSize = 1 << 20 // 1 MB — skip large files (binary, generated)

// Watcher monitors a directory for file changes and publishes WatchEvents to a channel.
type Watcher struct {
	name      string
	root      string
	includes  []string // patterns without "!" prefix
	excludes  []string // patterns from "!" prefix (stripped)
	ch        chan<- WatchEvent
	debounce  *Debouncer
	logger    *slog.Logger
	gitignore *ignore.GitIgnore
	done      chan struct{}
}

// New creates a Watcher for the named watcher config.
//
// patterns supports "!" negation: "!*.gen.go" excludes matching files.
// Events are published to ch; the caller owns ch and must drain it.
func New(name, root string, patterns []string, debounceMs int, ch chan<- WatchEvent, logger *slog.Logger) *Watcher {
	gi, _ := ignore.CompileIgnoreFile(filepath.Join(root, ".gitignore"))

	var includes, excludes []string
	for _, p := range patterns {
		if strings.HasPrefix(p, "!") {
			excludes = append(excludes, strings.TrimPrefix(p, "!"))
		} else {
			includes = append(includes, p)
		}
	}

	d := debounceMs
	if d == 0 {
		d = 300
	}

	return &Watcher{
		name:      name,
		root:      root,
		includes:  includes,
		excludes:  excludes,
		ch:        ch,
		debounce:  NewDebouncer(time.Duration(d) * time.Millisecond),
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

	_ = filepath.WalkDir(w.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			return nil
		}
		if skipDir(d.Name()) {
			return filepath.SkipDir
		}
		if w.isIgnored(path) {
			return filepath.SkipDir
		}
		if err := fsw.Add(path); err != nil {
			w.logger.Debug("skip watch dir", "watcher", w.name, "path", path, "error", err)
			return filepath.SkipDir
		}
		return nil
	})

	w.logger.Info("watcher started", "name", w.name, "root", w.root, "includes", w.includes, "excludes", w.excludes)

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
			w.logger.Error("watcher error", "name", w.name, "error", err)
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
	if !w.matches(event.Name) {
		return
	}
	if tooBig(event.Name) {
		return
	}

	var op Op
	switch {
	case event.Op.Has(fsnotify.Create):
		op = OpCreate
	case event.Op.Has(fsnotify.Write):
		op = OpModify
	case event.Op.Has(fsnotify.Remove):
		op = OpDelete
	default:
		return
	}

	w.debounce.Debounce(event.Name, func() {
		w.logger.Info("file changed", "watcher", w.name, "path", event.Name, "op", op)
		select {
		case w.ch <- WatchEvent{WatcherName: w.name, Path: event.Name, Op: op}:
		default:
			w.logger.Warn("watcher channel full, dropping event", "watcher", w.name, "path", event.Name)
		}
	})
}

// matches returns true if the path matches an include pattern and no exclude pattern.
func (w *Watcher) matches(path string) bool {
	rel, err := filepath.Rel(w.root, path)
	if err != nil {
		return false
	}
	base := filepath.Base(rel)

	// Check excludes first
	for _, ex := range w.excludes {
		if matchGlob(ex, rel) || matchGlob(ex, base) {
			return false
		}
	}

	// No includes = match everything not excluded
	if len(w.includes) == 0 {
		return true
	}

	for _, inc := range w.includes {
		if matchGlob(inc, rel) || matchGlob(inc, base) {
			return true
		}
	}
	return false
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

// matchGlob supports simple globs and basic ** patterns.
func matchGlob(pattern, name string) bool {
	if strings.Contains(pattern, "**") {
		// For **, match against any suffix of the path components.
		parts := strings.SplitN(pattern, "**/", 2)
		if len(parts) == 2 {
			suffix := parts[1]
			// Check each path component onwards
			components := strings.Split(name, string(filepath.Separator))
			for i := range components {
				candidate := strings.Join(components[i:], string(filepath.Separator))
				if m, _ := filepath.Match(suffix, candidate); m {
					return true
				}
			}
			return false
		}
	}
	m, _ := filepath.Match(pattern, name)
	return m
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

// tooBig returns true if the file is larger than maxFileSize.
func tooBig(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Size() > maxFileSize
}
