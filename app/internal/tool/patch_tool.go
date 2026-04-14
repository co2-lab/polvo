package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/co2-lab/polvo/internal/patch"
)

type patchInput struct {
	Files []string `json:"files"`
}

type patchTool struct {
	writer *patch.Writer
}

// NewPatchTool creates the patch tool — saves a unified diff of modified files.
func NewPatchTool(workdir string) Tool {
	return &patchTool{writer: patch.New(workdir)}
}

func (t *patchTool) Name() string { return "patch" }

func (t *patchTool) Description() string {
	return "Save a unified diff of the files modified in this execution to .polvo/patches/. Call this at the end of your task to record what changed."
}

func (t *patchTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"files": {
				"type": "array",
				"items": {"type": "string"},
				"description": "List of file paths that were modified in this execution"
			}
		},
		"required": ["files"]
	}`)
}

func (t *patchTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in patchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ErrorResult("invalid input: " + err.Error()), nil
	}
	if len(in.Files) == 0 {
		return ErrorResult("files list is required"), nil
	}

	patchPath, err := t.writer.Write(ctx, "agent", in.Files)
	if err != nil {
		return ErrorResult(fmt.Sprintf("writing patch: %v", err)), nil
	}
	if patchPath == "" {
		return &Result{Content: "no changes to record"}, nil
	}
	return &Result{Content: fmt.Sprintf("patch saved to %s", patchPath)}, nil
}
