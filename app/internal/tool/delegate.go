package tool

import (
	"context"
	"encoding/json"
	"fmt"
)

// SubAgentRunner is the interface a delegate tool uses to invoke a sub-agent.
// The sub-agent always runs read-only (plan mode).
type SubAgentRunner interface {
	RunSubAgent(ctx context.Context, agentName, task string) (string, error)
}

type delegateInput struct {
	Agent string `json:"agent"`
	Task  string `json:"task"`
}

type delegateTool struct {
	runner SubAgentRunner
}

// NewDelegate creates a delegate tool backed by the given runner.
func NewDelegate(runner SubAgentRunner) Tool {
	return &delegateTool{runner: runner}
}

func (t *delegateTool) Name() string { return "delegate" }

func (t *delegateTool) Description() string {
	return "Delegate a research or analysis sub-task to another agent (read-only mode). " +
		"The sub-agent can read files and search the codebase but cannot make changes. " +
		"Returns the sub-agent's findings."
}

func (t *delegateTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"agent": {"type": "string", "description": "Name of the agent to delegate to"},
			"task":  {"type": "string", "description": "Task description for the sub-agent"}
		},
		"required": ["agent", "task"]
	}`)
}

func (t *delegateTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in delegateInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ErrorResult(fmt.Sprintf("invalid input: %v", err)), nil
	}
	if in.Agent == "" {
		return ErrorResult("agent name is required"), nil
	}
	if in.Task == "" {
		return ErrorResult("task is required"), nil
	}

	output, err := t.runner.RunSubAgent(ctx, in.Agent, in.Task)
	if err != nil {
		return ErrorResult(fmt.Sprintf("sub-agent %q failed: %v", in.Agent, err)), nil
	}
	return &Result{Content: fmt.Sprintf("[%s]: %s", in.Agent, output)}, nil
}
