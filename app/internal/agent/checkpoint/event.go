package checkpoint

import "encoding/json"

// EventKind identifies the type of event recorded in the session log.
type EventKind string

const (
	EventUserMessage  EventKind = "user_message"
	EventAssistant    EventKind = "assistant_message"
	EventToolCall     EventKind = "tool_call"
	EventToolResult   EventKind = "tool_result"
	EventCondensation EventKind = "condensation"
	EventCheckpoint   EventKind = "checkpoint_marker"
	EventFileModified EventKind = "file_modified"
)

// Event is a single append-only entry in the session event log.
type Event struct {
	Index     int             `json:"index"`
	ID        string          `json:"id"`      // UUID v4 (hex)
	Kind      EventKind       `json:"kind"`
	Timestamp int64           `json:"ts_ns"`   // unix nanoseconds
	Payload   json.RawMessage `json:"payload"` // content typed by Kind
}

// MessagePayload is the payload for user/assistant message events.
type MessagePayload struct {
	Content string `json:"content"`
}

// ToolCallPayload is the payload for tool_call events.
type ToolCallPayload struct {
	ToolName string          `json:"tool_name"`
	Input    json.RawMessage `json:"input"`
}

// ToolResultPayload is the payload for tool_result events.
type ToolResultPayload struct {
	ToolName string `json:"tool_name"`
	Result   string `json:"result"`
	IsError  bool   `json:"is_error"`
}

// FileModifiedPayload is the payload for file_modified events.
type FileModifiedPayload struct {
	Path string `json:"path"`
}
