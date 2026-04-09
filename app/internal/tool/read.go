package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

type readInput struct {
	Path   string `json:"path"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

type readTool struct {
	workdir string
}

func NewRead(workdir string) Tool { return &readTool{workdir: workdir} }

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

func (t *readTool) Execute(_ context.Context, input json.RawMessage) (*Result, error) {
	var in readInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ErrorResult(fmt.Sprintf("invalid input: %v", err)), nil
	}

	path, err := resolvePath(t.workdir, in.Path)
	if err != nil {
		return ErrorResult(err.Error()), nil
	}

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

	return &Result{Content: sb.String()}, nil
}
