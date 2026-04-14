package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/co2-lab/polvo/internal/filelock"
)

type writeInput struct {
	Path         string `json:"path"`
	Content      string `json:"content"`
	SecurityRisk string `json:"security_risk"` // low | medium | high | critical
}

type writeTool struct {
	workdir string
	ignore  Ignorer
	cache   *ToolCache
}

// NewWrite creates a write tool without cache invalidation.
func NewWrite(workdir string, ig Ignorer) Tool { return NewWriteWithCache(workdir, ig, nil) }

// NewWriteWithCache creates a write tool that invalidates the cache on successful writes.
func NewWriteWithCache(workdir string, ig Ignorer, cache *ToolCache) Tool {
	return &writeTool{workdir: workdir, ignore: ig, cache: cache}
}

func (t *writeTool) Name() string { return "write" }

func (t *writeTool) Description() string {
	return "Create or overwrite a file with the given content."
}

func (t *writeTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path":          {"type": "string", "description": "File path relative to working directory"},
			"content":       {"type": "string", "description": "Content to write to the file"},
			"security_risk": {"type": "string", "enum": ["low","medium","high","critical"], "description": "Assessed risk level of this write operation", "default": "low"}
		},
		"required": ["path", "content", "security_risk"]
	}`)
}

func (t *writeTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in writeInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ErrorResult(fmt.Sprintf("invalid input: %v", err)), nil
	}

	path, err := resolvePath(t.workdir, in.Path)
	if err != nil {
		return ErrorResult(err.Error()), nil
	}
	if err := checkIgnored(t.ignore, path); err != nil {
		return ErrorResult(err.Error()), nil
	}

	// Acquire exclusive write lock with a 30-second timeout.
	lockCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	unlock, err := filelock.Global.LockWrite(lockCtx, path)
	if err != nil {
		return ErrorResult(fmt.Sprintf("acquiring write lock: %v", err)), nil
	}
	defer unlock()

	// Log security risk for high/critical operations.
	if in.SecurityRisk == "high" || in.SecurityRisk == "critical" {
		slog.Warn("file write", "path", in.Path, "risk", in.SecurityRisk)
	}

	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return ErrorResult(fmt.Sprintf("creating directories: %v", err)), nil
	}

	if err := os.WriteFile(path, []byte(in.Content), 0644); err != nil {
		return ErrorResult(fmt.Sprintf("writing file: %v", err)), nil
	}

	if t.cache != nil {
		t.cache.Invalidate(path)
	}

	return &Result{Content: fmt.Sprintf("Wrote %d bytes to %s", len(in.Content), in.Path)}, nil
}
