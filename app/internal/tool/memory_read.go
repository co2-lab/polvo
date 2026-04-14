package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/co2-lab/polvo/internal/memory"
)

type memoryReadInput struct {
	Agent     string `json:"agent"`
	File      string `json:"file"`
	Type      string `json:"type"`
	SessionID string `json:"session_id"`
	Limit     int    `json:"limit"`
}

type memoryReadTool struct {
	store *memory.Store
}

// NewMemoryRead creates the memory_read tool.
func NewMemoryRead(store *memory.Store) Tool {
	return &memoryReadTool{store: store}
}

func (t *memoryReadTool) Name() string { return "memory_read" }

func (t *memoryReadTool) Description() string {
	return "Read entries from the shared agent memory (.polvo/memory.db). Filter by agent, file, type (observation|decision|issue|context), or session_id."
}

func (t *memoryReadTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"agent":      {"type": "string", "description": "Filter by agent name"},
			"file":       {"type": "string", "description": "Filter by file path"},
			"type":       {"type": "string", "enum": ["observation","decision","issue","context"], "description": "Filter by entry type"},
			"session_id": {"type": "string", "description": "Filter by session ID"},
			"limit":      {"type": "integer", "description": "Max results to return (default 20)", "default": 20}
		}
	}`)
}

func (t *memoryReadTool) Execute(_ context.Context, input json.RawMessage) (*Result, error) {
	if t.store == nil {
		return ErrorResult("memory store not available"), nil
	}
	var in memoryReadInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ErrorResult("invalid input: " + err.Error()), nil
	}
	if in.Limit == 0 {
		in.Limit = 20
	}

	entries, err := t.store.Read(memory.Filter{
		Agent:     in.Agent,
		File:      in.File,
		Type:      in.Type,
		SessionID: in.SessionID,
		Limit:     in.Limit,
	})
	if err != nil {
		return ErrorResult(fmt.Sprintf("reading memory: %v", err)), nil
	}
	if len(entries) == 0 {
		return &Result{Content: "no entries found"}, nil
	}

	var sb strings.Builder
	for _, e := range entries {
		sb.WriteString(fmt.Sprintf("[%s] %s", e.Type, e.Content))
		if e.Agent != "" {
			sb.WriteString(fmt.Sprintf(" (agent: %s)", e.Agent))
		}
		if e.File != "" {
			sb.WriteString(fmt.Sprintf(" (file: %s)", e.File))
		}
		sb.WriteString("\n")
	}
	return &Result{Content: sb.String()}, nil
}
