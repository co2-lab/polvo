package checkpoint

import "encoding/json"

// SuspendReason identifies why the loop was suspended.
type SuspendReason string

const (
	SuspendReasonApproval  SuspendReason = "approval"
	SuspendReasonHuman     SuspendReason = "human_input"
	SuspendReasonError     SuspendReason = "error_review"
	SuspendReasonUserPause SuspendReason = "user_pause"
)

// SuspendPoint is the full serialisable state at the moment of suspension.
// Stored as sessions/<sessionID>/suspend_point.json (overwritten each suspend,
// deleted on resume).
type SuspendPoint struct {
	CheckpointID string            `json:"checkpoint_id"`
	Reason       SuspendReason     `json:"reason"`
	Prompt       string            `json:"prompt"`
	TurnCount    int               `json:"turn_count"`
	PendingTool  *PendingToolState `json:"pending_tool,omitempty"`
	SuspendedAt  int64             `json:"suspended_at_ns"`
}

// PendingToolState captures the tool call that triggered the suspension.
type PendingToolState struct {
	ToolCallID string          `json:"tool_call_id"`
	ToolName   string          `json:"tool_name"`
	Input      json.RawMessage `json:"input"`
}

// PendingWrite is an event buffered but not yet flushed to the Saver.
// Used for two-phase commit during suspension windows.
type PendingWrite struct {
	Kind    EventKind       `json:"kind"`
	Payload json.RawMessage `json:"payload"`
	Seq     int             `json:"seq"`
}

// EventKinds for suspend/resume audit trail.
const (
	EventSuspend EventKind = "suspend"
	EventResume  EventKind = "resume"
)
