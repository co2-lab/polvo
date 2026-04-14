package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

type undoEditInput struct {
	Path string `json:"path"`
}

// UndoRegistry stores pre-edit file contents for undo_edit.
// It is created once per agent run and passed to UndoEditTool.
type UndoRegistry struct {
	mu      sync.Mutex
	history map[string][]byte // path → previous content
}

// NewUndoRegistry creates a fresh undo registry.
func NewUndoRegistry() *UndoRegistry {
	return &UndoRegistry{history: make(map[string][]byte)}
}

// Snapshot saves the current content of path before an edit.
// Call this before every write/edit that modifies path.
func (u *UndoRegistry) Snapshot(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			data = nil // new file — undo will delete it
		} else {
			return err
		}
	}
	u.mu.Lock()
	u.history[path] = data
	u.mu.Unlock()
	return nil
}

type undoEditTool struct {
	workdir string
	undo    *UndoRegistry
}

// NewUndoEdit creates the undo_edit tool. reg must be the same UndoRegistry
// used to snapshot files before edits.
func NewUndoEdit(workdir string, reg *UndoRegistry) Tool {
	return &undoEditTool{workdir: workdir, undo: reg}
}

func (t *undoEditTool) Name() string { return "undo_edit" }

func (t *undoEditTool) Description() string {
	return "Undo the last edit to a file, restoring its previous content. Only works for files edited in the current agent run."
}

func (t *undoEditTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "File path to restore"}
		},
		"required": ["path"]
	}`)
}

func (t *undoEditTool) Execute(_ context.Context, input json.RawMessage) (*Result, error) {
	var in undoEditInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ErrorResult("invalid input: " + err.Error()), nil
	}

	path, err := resolvePath(t.workdir, in.Path)
	if err != nil {
		return ErrorResult(err.Error()), nil
	}

	t.undo.mu.Lock()
	prev, ok := t.undo.history[path]
	if ok {
		delete(t.undo.history, path) // consume — only one undo per path
	} else {
		// Fallback: try the lexical path (covers macOS /tmp → /private/tmp symlink divergence).
		if lex, err2 := resolveLexical(t.workdir, in.Path); err2 == nil && lex != path {
			prev, ok = t.undo.history[lex]
			if ok {
				delete(t.undo.history, lex)
			}
		}
	}
	t.undo.mu.Unlock()

	if !ok {
		return ErrorResult(fmt.Sprintf("no edit history found for %s in this session", in.Path)), nil
	}

	if prev == nil {
		// File was created in this session — delete it
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return ErrorResult(fmt.Sprintf("removing new file: %v", err)), nil
		}
		return &Result{Content: fmt.Sprintf("deleted %s (was created in this session)", in.Path)}, nil
	}

	if err := os.WriteFile(path, prev, 0644); err != nil {
		return ErrorResult(fmt.Sprintf("restoring file: %v", err)), nil
	}
	return &Result{Content: fmt.Sprintf("restored %s to previous content", in.Path)}, nil
}
