// Package provider defines the LLM provider abstraction.
package provider

import (
	"context"
	"encoding/json"
)

// Request represents a completion request to an LLM.
type Request struct {
	Model       string
	System      string
	Prompt      string
	MaxTokens   int
	Temperature float64
}

// Response represents a completion response from an LLM.
type Response struct {
	Content    string
	TokensUsed TokenUsage
}

// TokenUsage tracks token consumption.
type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CacheReadTokens  int // tokens served from cache (read hit)
	CacheWriteTokens int // tokens written to cache (cache creation)
}

// LLMProvider is the interface for AI model providers.
type LLMProvider interface {
	// Name returns the provider identifier.
	Name() string

	// Complete sends a prompt and returns the completion.
	Complete(ctx context.Context, req Request) (*Response, error)

	// Available checks if the provider is reachable and configured.
	Available(ctx context.Context) error
}

// ChatProvider extends LLMProvider with multi-turn chat and tool use.
type ChatProvider interface {
	LLMProvider
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
}

// Message represents a single message in a conversation.
type Message struct {
	Role       string       // "user", "assistant", "tool"
	Content    string       // Text content
	ToolCalls  []ToolCall   // Tool invocations (assistant messages)
	ToolResult *ToolResult  // Tool execution result (tool messages)
}

// ToolCall represents a tool invocation by the LLM.
type ToolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

// ToolResult represents the result of a tool execution.
type ToolResult struct {
	ToolCallID string
	Content    string
	IsError    bool
}

// ToolDef defines a tool available to the LLM.
type ToolDef struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

// ChatRequest is a multi-turn chat request with optional tools.
type ChatRequest struct {
	Model       string
	System      string
	Messages    []Message
	Tools       []ToolDef
	MaxTokens   int
	Temperature float64
}

// ChatResponse is the result of a chat request.
type ChatResponse struct {
	Message    Message
	StopReason string // "end_turn", "tool_use", "max_tokens"
	TokensUsed TokenUsage
}

// StreamEvent represents a streaming event from the LLM.
type StreamEvent struct {
	Type      string // "text_delta", "tool_use_start", "tool_use_done", "done"
	TextDelta string
	ToolCall  *ToolCall
}

// StreamProvider extends ChatProvider with streaming support.
type StreamProvider interface {
	ChatProvider
	ChatStream(ctx context.Context, req ChatRequest, handler func(StreamEvent)) (*ChatResponse, error)
}
