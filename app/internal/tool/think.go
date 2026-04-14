package tool

import (
	"context"
	"encoding/json"
)

type thinkInput struct {
	Thought string `json:"thought"`
}

type thinkTool struct{}

// NewThink creates the think tool — lets the agent reason without side-effects.
func NewThink() Tool { return &thinkTool{} }

func (t *thinkTool) Name() string { return "think" }

func (t *thinkTool) Description() string {
	return "Use this tool to think through a problem before acting. The thought is logged but has no side-effects. Use it to reason about which tool to call next, verify constraints, or plan a sequence of actions."
}

func (t *thinkTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"thought": {"type": "string", "description": "Your internal reasoning or plan"}
		},
		"required": ["thought"]
	}`)
}

func (t *thinkTool) Execute(_ context.Context, input json.RawMessage) (*Result, error) {
	var in thinkInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ErrorResult("invalid input: " + err.Error()), nil
	}
	if in.Thought == "" {
		return ErrorResult("thought is required"), nil
	}
	return &Result{Content: in.Thought}, nil
}
