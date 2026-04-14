package tool

import (
	"context"
	"encoding/json"

	"github.com/co2-lab/polvo/internal/git"
)

type diffInput struct {
	Staged bool     `json:"staged"`
	Paths  []string `json:"paths,omitempty"`
}

type diffTool struct {
	client git.Client
}

// NewDiff creates a tool that shows the unified diff of uncommitted changes.
func NewDiff(client git.Client) Tool {
	return &diffTool{client: client}
}

func (t *diffTool) Name() string { return "diff" }

func (t *diffTool) Description() string {
	return "Show unified diff of uncommitted changes. Useful in reflection to see what was actually modified."
}

func (t *diffTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"staged": {
				"type": "boolean",
				"description": "If true, show staged (cached) diff. If false (default), show unstaged diff against HEAD."
			},
			"paths": {
				"type": "array",
				"items": {"type": "string"},
				"description": "Optional list of file paths to restrict the diff to."
			}
		},
		"required": []
	}`)
}

func (t *diffTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in diffInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ErrorResult("invalid input: " + err.Error()), nil
	}

	diff, err := t.client.DiffFiles(ctx, in.Paths, in.Staged)
	if err != nil {
		return ErrorResult("diff failed: " + err.Error()), nil
	}

	if diff == "" {
		return &Result{Content: "no changes"}, nil
	}
	return &Result{Content: diff}, nil
}
