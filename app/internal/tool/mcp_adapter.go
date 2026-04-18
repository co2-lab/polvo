package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/co2-lab/polvo/internal/tool/mcp"
)

// mcpToolAdapter wraps a single MCP tool definition so it satisfies the Tool interface.
type mcpToolAdapter struct {
	def mcp.ToolDefinition
	hub *mcp.MCPHub
}

func (a *mcpToolAdapter) Name() string        { return a.def.Name }
func (a *mcpToolAdapter) Description() string { return a.def.Description }
func (a *mcpToolAdapter) InputSchema() json.RawMessage {
	if a.def.InputSchema != nil {
		return a.def.InputSchema
	}
	return json.RawMessage(`{"type":"object","properties":{}}`)
}

func (a *mcpToolAdapter) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	raw, err := a.hub.Call(ctx, a.def.Name, input)
	if err != nil {
		return ErrorResult(fmt.Sprintf("mcp tool error: %v", err)), nil
	}
	// MCP results can be complex JSON — marshal back to string for the agent.
	var content string
	// Try to unwrap a simple string result first.
	var s string
	if json.Unmarshal(raw, &s) == nil {
		content = s
	} else {
		content = string(raw)
	}
	return &Result{Content: content}, nil
}
