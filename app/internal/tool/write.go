package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type writeInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type writeTool struct {
	workdir string
}

func NewWrite(workdir string) Tool { return &writeTool{workdir: workdir} }

func (t *writeTool) Name() string { return "write" }

func (t *writeTool) Description() string {
	return "Create or overwrite a file with the given content."
}

func (t *writeTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path":    {"type": "string", "description": "File path relative to working directory"},
			"content": {"type": "string", "description": "Content to write to the file"}
		},
		"required": ["path", "content"]
	}`)
}

func (t *writeTool) Execute(_ context.Context, input json.RawMessage) (*Result, error) {
	var in writeInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ErrorResult(fmt.Sprintf("invalid input: %v", err)), nil
	}

	path, err := resolvePath(t.workdir, in.Path)
	if err != nil {
		return ErrorResult(err.Error()), nil
	}

	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return ErrorResult(fmt.Sprintf("creating directories: %v", err)), nil
	}

	if err := os.WriteFile(path, []byte(in.Content), 0644); err != nil {
		return ErrorResult(fmt.Sprintf("writing file: %v", err)), nil
	}

	return &Result{Content: fmt.Sprintf("Wrote %d bytes to %s", len(in.Content), in.Path)}, nil
}
