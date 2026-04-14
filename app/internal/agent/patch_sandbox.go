package agent

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

// PatchSandbox buffers file writes in memory before committing to disk.
// Used in supervised mode: agent edits accumulate here; user approves → Apply().
type PatchSandbox struct {
	mu      sync.Mutex
	pending []sandboxEntry
}

type sandboxEntry struct {
	path     string
	original []byte // nil if file didn't exist
	content  []byte // new content
}

// Record saves a pending file write without touching disk.
func (s *PatchSandbox) Record(path string, content []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	original, _ := os.ReadFile(path) // nil if not exists
	s.pending = append(s.pending, sandboxEntry{path, original, content})
	return nil
}

// Preview returns a simple diff-style summary of all pending changes.
func (s *PatchSandbox) Preview() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pending) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, e := range s.pending {
		sb.WriteString(fmt.Sprintf("--- %s\n+++ %s\n", e.path, e.path))
		oldLines := strings.Split(string(e.original), "\n")
		newLines := strings.Split(string(e.content), "\n")
		// Simple line-based diff: show removed and added lines.
		oldSet := make(map[string]bool, len(oldLines))
		for _, l := range oldLines {
			oldSet[l] = true
		}
		newSet := make(map[string]bool, len(newLines))
		for _, l := range newLines {
			newSet[l] = true
		}
		for _, l := range oldLines {
			if !newSet[l] {
				sb.WriteString("-" + l + "\n")
			}
		}
		for _, l := range newLines {
			if !oldSet[l] {
				sb.WriteString("+" + l + "\n")
			}
		}
	}
	return sb.String()
}

// Apply writes all pending changes to disk atomically (best-effort: stops on first error).
func (s *PatchSandbox) Apply() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.pending {
		if err := os.WriteFile(e.path, e.content, 0644); err != nil {
			return fmt.Errorf("applying %s: %w", e.path, err)
		}
	}
	s.pending = nil
	return nil
}

// Discard clears all pending changes without writing.
func (s *PatchSandbox) Discard() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = nil
}

// Len returns the number of pending file changes.
func (s *PatchSandbox) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.pending)
}
