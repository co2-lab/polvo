package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type lsInput struct {
	Path string `json:"path"`
}

type lsTool struct {
	workdir string
	cache   *ToolCache
}

// NewLS creates an ls tool without caching.
func NewLS(workdir string) Tool { return NewLSWithCache(workdir, nil) }

// NewLSWithCache creates an ls tool with an optional result cache.
func NewLSWithCache(workdir string, cache *ToolCache) Tool {
	return &lsTool{workdir: workdir, cache: cache}
}

func (t *lsTool) Name() string { return "ls" }

func (t *lsTool) Description() string {
	return "List directory contents with file sizes and types."
}

func (t *lsTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Directory path (default: working directory)"}
		}
	}`)
}

func (t *lsTool) Execute(_ context.Context, input json.RawMessage) (*Result, error) {
	var in lsInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ErrorResult(fmt.Sprintf("invalid input: %v", err)), nil
	}

	// Check cache before reading the directory.
	if t.cache != nil {
		key := CacheKey(t.Name(), input, "")
		if cached, ok := t.cache.Get(key); ok {
			return cached, nil
		}
	}

	dir := t.workdir
	if in.Path != "" {
		resolved, err := resolvePath(t.workdir, in.Path)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		dir = resolved
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return ErrorResult(fmt.Sprintf("reading directory: %v", err)), nil
	}

	var lines []string
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}

		relPath, _ := filepath.Rel(t.workdir, filepath.Join(dir, e.Name()))
		if e.IsDir() {
			lines = append(lines, fmt.Sprintf("  %s/", relPath))
		} else {
			lines = append(lines, fmt.Sprintf("  %-40s %s", relPath, formatSize(info.Size())))
		}
	}

	var res *Result
	if len(lines) == 0 {
		res = &Result{Content: "(empty directory)"}
	} else {
		res = &Result{Content: strings.Join(lines, "\n")}
	}
	if t.cache != nil {
		key := CacheKey(t.Name(), input, "")
		t.cache.Set(key, "", res)
	}
	return res, nil
}

func formatSize(bytes int64) string {
	switch {
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(bytes)/float64(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(bytes)/float64(1<<10))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}
