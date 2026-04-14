package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/co2-lab/polvo/internal/memory"
)

type memoryWriteInput struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	File    string `json:"file"`
}

type memoryWriteTool struct {
	store     *memory.Store
	agentName string
	sessionID string
}

// NewMemoryWrite creates the memory_write tool.
func NewMemoryWrite(store *memory.Store, agentName, sessionID string) Tool {
	return &memoryWriteTool{store: store, agentName: agentName, sessionID: sessionID}
}

func (t *memoryWriteTool) Name() string { return "memory_write" }

func (t *memoryWriteTool) Description() string {
	return "Write an entry to the shared agent memory (.polvo/memory.db). Use to record observations, decisions, issues, or context that other agents may need."
}

func (t *memoryWriteTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"type":    {"type": "string", "enum": ["observation","decision","issue","context"], "description": "Type of memory entry"},
			"content": {"type": "string", "description": "Content to store"},
			"file":    {"type": "string", "description": "Optional file path this entry relates to"}
		},
		"required": ["type", "content"]
	}`)
}

func (t *memoryWriteTool) Execute(_ context.Context, input json.RawMessage) (*Result, error) {
	if t.store == nil {
		return ErrorResult("memory store not available"), nil
	}
	var in memoryWriteInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ErrorResult("invalid input: " + err.Error()), nil
	}
	if in.Type == "" || in.Content == "" {
		return ErrorResult("type and content are required"), nil
	}

	err := t.store.Write(memory.Entry{
		SessionID: t.sessionID,
		Agent:     t.agentName,
		File:      in.File,
		Type:      in.Type,
		Content:   in.Content,
	})
	if err != nil {
		return ErrorResult(fmt.Sprintf("writing memory: %v", err)), nil
	}
	return &Result{Content: fmt.Sprintf("stored %s entry in memory", in.Type)}, nil
}
