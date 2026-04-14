package checkpoint

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// BaseState is the mutable metadata for a session. It is overwritten on each
// status change (the only file that is not append-only).
type BaseState struct {
	SessionID string `json:"session_id"`
	AgentName string `json:"agent_name"`
	StartedAt int64  `json:"started_at_ns"`
	UpdatedAt int64  `json:"updated_at_ns"`
	Status    string `json:"status"` // "running" | "completed" | "failed"
}

// FSStore persists events and checkpoints as JSON files on disk.
//
// Directory layout:
//
//	baseDir/
//	  sessions/
//	    <sessionID>/
//	      base_state.json
//	      events/
//	        0001-<uuid>.json
//	        0002-<uuid>.json
//	      checkpoints/
//	        <checkpointID>.json
type FSStore struct {
	baseDir string
}

// NewFSStore creates an FSStore rooted at baseDir.
func NewFSStore(baseDir string) *FSStore {
	return &FSStore{baseDir: baseDir}
}

// newUUID generates a random hex UUID (32 hex characters, no dashes).
func newUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating uuid: %w", err)
	}
	return fmt.Sprintf("%x", b), nil
}

// sessionDir returns the root directory for a session.
func (s *FSStore) sessionDir(sessionID string) string {
	return filepath.Join(s.baseDir, "sessions", sessionID)
}

// eventsDir returns the events sub-directory for a session.
func (s *FSStore) eventsDir(sessionID string) string {
	return filepath.Join(s.sessionDir(sessionID), "events")
}

// checkpointsDir returns the checkpoints sub-directory for a session.
func (s *FSStore) checkpointsDir(sessionID string) string {
	return filepath.Join(s.sessionDir(sessionID), "checkpoints")
}

// ensureDir creates a directory (and parents) if it does not exist.
func ensureDir(dir string) error {
	return os.MkdirAll(dir, 0o755)
}

// writeJSON marshals v and atomically writes it to path.
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling json for %s: %w", path, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	return os.Rename(tmp, path)
}

// readJSON reads path and unmarshals into v.
func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	return json.Unmarshal(data, v)
}

// AppendEvent writes the event to events/<index+1>-<uuid>.json.
// If e.ID is empty, a new UUID is generated. If e.Index is 0 (default),
// it is set to the next sequential index based on existing files.
func (s *FSStore) AppendEvent(sessionID string, e Event) error {
	dir := s.eventsDir(sessionID)
	if err := ensureDir(dir); err != nil {
		return err
	}

	// Determine next index from existing files.
	existing, err := s.listEventFiles(dir)
	if err != nil {
		return err
	}
	e.Index = len(existing) // 0-based

	// Generate ID if not set.
	if e.ID == "" {
		id, err := newUUID()
		if err != nil {
			return err
		}
		e.ID = id
	}

	filename := fmt.Sprintf("%04d-%s.json", e.Index+1, e.ID)
	return writeJSON(filepath.Join(dir, filename), e)
}

// listEventFiles returns event filenames in sorted order.
func (s *FSStore) listEventFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading events dir %s: %w", dir, err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	return files, nil
}

// LoadEvents reads all event files in order for a session, returning only
// those with Index >= fromIndex.
func (s *FSStore) LoadEvents(sessionID string, fromIndex int) ([]Event, error) {
	dir := s.eventsDir(sessionID)
	files, err := s.listEventFiles(dir)
	if err != nil {
		return nil, err
	}

	var events []Event
	for _, name := range files {
		// Parse the sequence number from the filename prefix (e.g. "0001-<uuid>.json").
		dashIdx := strings.Index(name, "-")
		if dashIdx < 0 {
			continue
		}
		seqStr := name[:dashIdx]
		seq, err := strconv.Atoi(seqStr)
		if err != nil {
			continue
		}
		// seq is 1-based; event Index is 0-based
		if seq-1 < fromIndex {
			continue
		}

		var e Event
		if err := readJSON(filepath.Join(dir, name), &e); err != nil {
			return nil, fmt.Errorf("loading event %s: %w", name, err)
		}
		events = append(events, e)
	}
	return events, nil
}

// SaveCheckpoint writes the checkpoint to checkpoints/<id>.json.
func (s *FSStore) SaveCheckpoint(c Checkpoint) error {
	dir := s.checkpointsDir(c.SessionID)
	if err := ensureDir(dir); err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, c.ID+".json"), c)
}

// LoadCheckpoint reads a checkpoint by ID.
// It searches all session directories because a checkpoint ID is globally unique.
func (s *FSStore) LoadCheckpoint(id string) (Checkpoint, error) {
	sessionsDir := filepath.Join(s.baseDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return Checkpoint{}, fmt.Errorf("reading sessions dir: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(sessionsDir, entry.Name(), "checkpoints", id+".json")
		if _, err := os.Stat(path); err == nil {
			var c Checkpoint
			if err := readJSON(path, &c); err != nil {
				return Checkpoint{}, err
			}
			return c, nil
		}
	}
	return Checkpoint{}, fmt.Errorf("checkpoint %q not found", id)
}

// ListCheckpoints returns all checkpoints for a session in chronological order
// (sorted by Timestamp ascending).
func (s *FSStore) ListCheckpoints(sessionID string) ([]Checkpoint, error) {
	dir := s.checkpointsDir(sessionID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading checkpoints dir: %w", err)
	}

	var checkpoints []Checkpoint
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var c Checkpoint
		if err := readJSON(filepath.Join(dir, entry.Name()), &c); err != nil {
			return nil, fmt.Errorf("loading checkpoint %s: %w", entry.Name(), err)
		}
		checkpoints = append(checkpoints, c)
	}

	sort.Slice(checkpoints, func(i, j int) bool {
		return checkpoints[i].Timestamp < checkpoints[j].Timestamp
	})
	return checkpoints, nil
}

// SaveBaseState writes base_state.json for a session (overwrites on each update).
func (s *FSStore) SaveBaseState(sessionID string, state BaseState) error {
	dir := s.sessionDir(sessionID)
	if err := ensureDir(dir); err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, "base_state.json"), state)
}

// LoadBaseState reads base_state.json for a session.
func (s *FSStore) LoadBaseState(sessionID string) (BaseState, error) {
	path := filepath.Join(s.sessionDir(sessionID), "base_state.json")
	var state BaseState
	if err := readJSON(path, &state); err != nil {
		return BaseState{}, err
	}
	return state, nil
}
