package checkpoint

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Recorder is a helper used by the agent loop to record events and create
// checkpoints. It is created once per agent run.
type Recorder struct {
	store      Saver
	sessionID  string
	mu         sync.Mutex
	eventIdx   int            // next event index (0-based, incremented after each append)
	pending    []PendingWrite // two-phase write buffer
	pendingSeq int
}

// NewRecorder initialises a Recorder for the given session and writes the
// initial base_state.json with status "running".
func NewRecorder(store Saver, sessionID, agentName string) (*Recorder, error) {
	now := time.Now().UnixNano()
	state := BaseState{
		SessionID: sessionID,
		AgentName: agentName,
		StartedAt: now,
		UpdatedAt: now,
		Status:    "running",
	}
	if err := store.SaveBaseState(sessionID, state); err != nil {
		return nil, fmt.Errorf("saving base state: %w", err)
	}
	return &Recorder{
		store:     store,
		sessionID: sessionID,
	}, nil
}

// appendEvent builds and appends an event with the given kind and payload.
// Must be called with r.mu held.
func (r *Recorder) appendEvent(kind EventKind, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling payload: %w", err)
	}
	e := Event{
		Kind:      kind,
		Timestamp: time.Now().UnixNano(),
		Payload:   json.RawMessage(raw),
	}
	if err := r.store.AppendEvent(r.sessionID, e); err != nil {
		return err
	}
	r.eventIdx++
	return nil
}

// RecordMessage appends a user or assistant message event.
func (r *Recorder) RecordMessage(kind EventKind, content string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.appendEvent(kind, MessagePayload{Content: content})
}

// RecordToolCall appends a tool_call event.
func (r *Recorder) RecordToolCall(toolName string, input json.RawMessage) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.appendEvent(EventToolCall, ToolCallPayload{
		ToolName: toolName,
		Input:    input,
	})
}

// RecordToolResult appends a tool_result event.
func (r *Recorder) RecordToolResult(toolName, result string, isError bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.appendEvent(EventToolResult, ToolResultPayload{
		ToolName: toolName,
		Result:   result,
		IsError:  isError,
	})
}

// RecordFileModified appends a file_modified event.
func (r *Recorder) RecordFileModified(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.appendEvent(EventFileModified, FileModifiedPayload{Path: path})
}

// CreateCheckpoint saves the current state as a checkpoint and returns its ID.
// description is a human-readable label. filesSnapshot maps path →
// base64-encoded file contents for files modified during the session.
func (r *Recorder) CreateCheckpoint(description string, filesSnapshot map[string]string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id, err := newUUID()
	if err != nil {
		return "", fmt.Errorf("generating checkpoint id: %w", err)
	}

	// The checkpoint captures the state up to the last recorded event.
	// eventIdx is the count of events appended so far; the last index is eventIdx-1.
	lastEventIndex := r.eventIdx - 1
	if lastEventIndex < 0 {
		lastEventIndex = 0
	}

	c := Checkpoint{
		ID:              id,
		SessionID:       r.sessionID,
		EventIndex:      lastEventIndex,
		Timestamp:       time.Now().UnixNano(),
		Description:     description,
		FilesSnapshot:   filesSnapshot,
		ConversationLen: r.eventIdx,
	}
	if err := r.store.SaveCheckpoint(c); err != nil {
		return "", fmt.Errorf("saving checkpoint: %w", err)
	}

	// Also record a checkpoint_marker event.
	_ = r.appendEvent(EventCheckpoint, map[string]string{"checkpoint_id": id})

	return id, nil
}

// RecordSuspend appends a suspend event to the log.
func (r *Recorder) RecordSuspend(reason SuspendReason) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.appendEvent(EventSuspend, map[string]string{"reason": string(reason)})
}

// RecordResume appends a resume event with the human's input to the log.
func (r *Recorder) RecordResume(humanInput string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.appendEvent(EventResume, map[string]string{"input": humanInput})
}

// BeginPendingWrite adds an event to the in-memory buffer without persisting.
// Call FlushPending once the SuspendPoint is durably saved.
func (r *Recorder) BeginPendingWrite(kind EventKind, payload any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	r.pending = append(r.pending, PendingWrite{
		Kind:    kind,
		Payload: json.RawMessage(raw),
		Seq:     r.pendingSeq,
	})
	r.pendingSeq++
}

// FlushPending persists buffered writes in order, then clears the buffer.
func (r *Recorder) FlushPending() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.pending) == 0 {
		return nil
	}
	if err := r.store.PutPendingWrites(r.sessionID, r.pending); err != nil {
		return err
	}
	for _, pw := range r.pending {
		e := Event{
			Kind:      pw.Kind,
			Timestamp: time.Now().UnixNano(),
			Payload:   pw.Payload,
		}
		if err := r.store.AppendEvent(r.sessionID, e); err != nil {
			return err
		}
		r.eventIdx++
	}
	r.pending = r.pending[:0]
	_ = r.store.ClearPendingWrites(r.sessionID)
	return nil
}

// DiscardPending drops buffered writes without persisting them.
func (r *Recorder) DiscardPending() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pending = r.pending[:0]
	_ = r.store.ClearPendingWrites(r.sessionID)
}

// Finish updates base_state.json with the final status ("completed" or "failed").
func (r *Recorder) Finish(status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	state, err := r.store.LoadBaseState(r.sessionID)
	if err != nil {
		// If we can't load existing state, build a minimal one.
		state = BaseState{SessionID: r.sessionID}
	}
	state.Status = status
	state.UpdatedAt = time.Now().UnixNano()
	return r.store.SaveBaseState(r.sessionID, state)
}
