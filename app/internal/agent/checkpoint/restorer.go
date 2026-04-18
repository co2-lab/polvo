package checkpoint

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
)

// RestoreMode controls which dimensions are restored.
type RestoreMode int

const (
	RestoreCodeOnly         RestoreMode = 1 // revert files, keep conversation
	RestoreConversationOnly RestoreMode = 2 // revert conversation, keep files
	RestoreFull             RestoreMode = 3 // revert both files and conversation
)

// Restorer performs checkpoint restore operations.
type Restorer struct {
	store Saver
}

// NewRestorer creates a Restorer backed by the given store.
func NewRestorer(store Saver) *Restorer {
	return &Restorer{store: store}
}

// RestoreFiles restores files to their state at the given checkpoint.
// FilesSnapshot maps path → base64-encoded content. Each file is decoded and
// written back to workdir/<path>. Existing files not in the snapshot are left
// untouched (only files captured at checkpoint time are restored).
func (r *Restorer) RestoreFiles(checkpointID string, workdir string) error {
	c, err := r.store.LoadCheckpoint(checkpointID)
	if err != nil {
		return fmt.Errorf("loading checkpoint %s: %w", checkpointID, err)
	}

	for relPath, encoded := range c.FilesSnapshot {
		content, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return fmt.Errorf("decoding file %s: %w", relPath, err)
		}

		abs := filepath.Join(workdir, relPath)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return fmt.Errorf("creating parent dirs for %s: %w", abs, err)
		}
		if err := os.WriteFile(abs, content, 0o644); err != nil {
			return fmt.Errorf("writing file %s: %w", abs, err)
		}
	}
	return nil
}

// LoadConversationHistory returns all events recorded up to and including the
// checkpoint's EventIndex (inclusive). Events are in chronological order.
func (r *Restorer) LoadConversationHistory(checkpointID string) ([]Event, error) {
	c, err := r.store.LoadCheckpoint(checkpointID)
	if err != nil {
		return nil, fmt.Errorf("loading checkpoint %s: %w", checkpointID, err)
	}

	all, err := r.store.LoadEvents(c.SessionID, 0)
	if err != nil {
		return nil, fmt.Errorf("loading events for session %s: %w", c.SessionID, err)
	}

	var result []Event
	for _, e := range all {
		if e.Index <= c.EventIndex {
			result = append(result, e)
		}
	}
	return result, nil
}
