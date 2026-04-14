package checkpoint

// Checkpoint is an immutable snapshot of agent state at a specific event index.
// FilesSnapshot maps file path → base64-encoded file contents at the time of
// the checkpoint. This allows restoring file state without relying on git history.
type Checkpoint struct {
	ID              string            `json:"id"`               // UUID v4 (hex)
	ParentID        string            `json:"parent_id"`        // "" if root
	SessionID       string            `json:"session_id"`
	EventIndex      int               `json:"event_index"`      // last event index included
	Timestamp       int64             `json:"ts_ns"`            // unix nanoseconds
	Description     string            `json:"description"`
	FilesSnapshot   map[string]string `json:"files_snapshot"`   // path → base64(content)
	ConversationLen int               `json:"conversation_len"` // number of messages at checkpoint
}
