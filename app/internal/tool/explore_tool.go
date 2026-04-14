package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ExploreRunner is the interface used by the explore tool to launch parallel
// read-only subagents. It is implemented by agent.Executor.
type ExploreRunner interface {
	// RunExplore executes parallel read-only exploration tasks.
	// Returns a markdown-formatted summary of all findings.
	RunExplore(ctx context.Context, tasks []ExploreTaskInput, tokenBudgetPerTask int) (string, error)
}

// ExploreTaskInput is the per-task input passed to ExploreRunner.
type ExploreTaskInput struct {
	Description string   `json:"description"`
	Focus       []string `json:"focus,omitempty"`
}

// exploreTool is the tool.Tool implementation for "explore".
type exploreTool struct {
	runner ExploreRunner
}

type exploreInput struct {
	Tasks              []ExploreTaskInput `json:"tasks"`
	TokenBudgetPerTask int                `json:"token_budget_per_task,omitempty"`
}

// NewExploreTool creates the explore tool backed by the given runner.
// This tool should only be registered when DelegateLevel == 0.
func NewExploreTool(runner ExploreRunner) Tool {
	return &exploreTool{runner: runner}
}

func (t *exploreTool) Name() string { return "explore" }

func (t *exploreTool) Description() string {
	return "Launch read-only subagents in parallel to explore the codebase. " +
		"Use before generating code when you need to understand parts of the project " +
		"that are not in the current context. Subagents can only read files — they " +
		"cannot write, edit, or execute anything."
}

func (t *exploreTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"tasks": {
				"type": "array",
				"description": "List of exploration subtasks to run in parallel",
				"items": {
					"type": "object",
					"properties": {
						"description": {"type": "string", "description": "What this subagent should explore"},
						"focus":       {"type": "array", "items": {"type": "string"}, "description": "File paths or patterns to focus on"}
					},
					"required": ["description"]
				}
			},
			"token_budget_per_task": {
				"type": "integer",
				"description": "Token budget per subagent task (default: 8000)"
			}
		},
		"required": ["tasks"]
	}`)
}

func (t *exploreTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	// DelegateLevel check is done by the runner (ExploreRunner.RunExplore)
	var in exploreInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ErrorResult(fmt.Sprintf("invalid input: %v", err)), nil
	}
	if len(in.Tasks) == 0 {
		return ErrorResult("at least one task is required"), nil
	}

	output, err := t.runner.RunExplore(ctx, in.Tasks, in.TokenBudgetPerTask)
	if err != nil {
		return ErrorResult(fmt.Sprintf("explore failed: %v", err)), nil
	}
	return &Result{Content: output}, nil
}

// FormatExploreResults formats a list of explore result summaries into markdown.
// This is a helper used by the runner implementation.
func FormatExploreResults(taskDescriptions []string, summaries []string, errors []error) string {
	var sb strings.Builder
	sb.WriteString("# Exploration Results\n\n")

	for i, desc := range taskDescriptions {
		sb.WriteString(fmt.Sprintf("## Task %d: %s\n\n", i+1, desc))
		if i < len(errors) && errors[i] != nil {
			sb.WriteString(fmt.Sprintf("**Error:** %v\n\n", errors[i]))
			continue
		}
		if i < len(summaries) && summaries[i] != "" {
			sb.WriteString(summaries[i])
		} else {
			sb.WriteString("_No findings._")
		}
		sb.WriteString("\n\n")
	}

	return sb.String()
}
