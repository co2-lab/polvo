package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/co2-lab/polvo/internal/filelock"
)

type readInput struct {
	Path   string `json:"path"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

type readTool struct {
	workdir string
	ignore  Ignorer
	cache   *ToolCache
}

// NewRead creates a read tool without caching.
func NewRead(workdir string, ig Ignorer) Tool { return NewReadWithCache(workdir, ig, nil) }

// NewReadWithCache creates a read tool with an optional result cache.
func NewReadWithCache(workdir string, ig Ignorer, cache *ToolCache) Tool {
	return &readTool{workdir: workdir, ignore: ig, cache: cache}
}

func (t *readTool) Name() string { return "read" }

func (t *readTool) Description() string {
	return "Read a file's contents. Supports offset and limit for large files."
}

func (t *readTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path":   {"type": "string", "description": "File path relative to working directory"},
			"offset": {"type": "integer", "description": "Line number to start reading from (0-based)", "default": 0},
			"limit":  {"type": "integer", "description": "Maximum number of lines to read", "default": 2000}
		},
		"required": ["path"]
	}`)
}

func (t *readTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in readInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ErrorResult(fmt.Sprintf("invalid input: %v", err)), nil
	}

	// Compute lexical path for ignore check (before symlink resolution).
	lexPath, err := resolveLexical(t.workdir, in.Path)
	if err != nil {
		return ErrorResult(err.Error()), nil
	}
	if err := checkIgnored(t.ignore, lexPath); err != nil {
		return ErrorResult(err.Error()), nil
	}
	path, err := resolvePath(t.workdir, in.Path)
	if err != nil {
		return ErrorResult(err.Error()), nil
	}

	// Check cache before acquiring the lock and reading the file.
	if t.cache != nil {
		key := CacheKey(t.Name(), input, path)
		if cached, ok := t.cache.Get(key); ok {
			return cached, nil
		}
	}

	// Acquire shared read lock with a 30-second timeout.
	lockCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	unlock, err := filelock.Global.LockRead(lockCtx, path)
	if err != nil {
		return ErrorResult(fmt.Sprintf("acquiring read lock: %v", err)), nil
	}
	defer unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return ErrorResult(fmt.Sprintf("reading file: %v", err)), nil
	}

	// Detect binary
	if !utf8.Valid(data) {
		return ErrorResult("file appears to be binary"), nil
	}

	limit := in.Limit
	if limit <= 0 {
		limit = 2000
	}

	lines := strings.Split(string(data), "\n")
	offset := in.Offset
	if offset < 0 {
		offset = 0
	}
	if offset >= len(lines) {
		return &Result{Content: ""}, nil
	}

	end := offset + limit
	if end > len(lines) {
		end = len(lines)
	}

	// Format with line numbers
	var sb strings.Builder
	for i := offset; i < end; i++ {
		fmt.Fprintf(&sb, "%4d\t%s\n", i+1, lines[i])
	}

	res := &Result{Content: sb.String()}
	if t.cache != nil {
		key := CacheKey(t.Name(), input, path)
		t.cache.Set(key, path, res)
	}
	return res, nil
}
