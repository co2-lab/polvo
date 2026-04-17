package checkpoint

// Saver is the storage backend interface for checkpoints and events.
// FSStore implements this interface; future backends (SQLiteSaver, PostgresSaver)
// can implement the same without modifying the core loop.
type Saver interface {
	AppendEvent(sessionID string, e Event) error
	LoadEvents(sessionID string, fromIndex int) ([]Event, error)
	SaveCheckpoint(c Checkpoint) error
	LoadCheckpoint(id string) (Checkpoint, error)
	ListCheckpoints(sessionID string) ([]Checkpoint, error)
	SaveBaseState(sessionID string, state BaseState) error
	LoadBaseState(sessionID string) (BaseState, error)
	// Pending writes (two-phase commit)
	PutPendingWrites(sessionID string, writes []PendingWrite) error
	LoadPendingWrites(sessionID string) ([]PendingWrite, error)
	ClearPendingWrites(sessionID string) error
	// Suspend point
	SaveSuspendPoint(sessionID string, sp SuspendPoint) error
	LoadSuspendPoint(sessionID string) (SuspendPoint, error)
	DeleteSuspendPoint(sessionID string) error
	// Turn marks (user annotations — mutable, overwritten atomically)
	SaveTurnMarks(sessionID string, marks []TurnMarkRecord) error
	LoadTurnMarks(sessionID string) ([]TurnMarkRecord, error)
}

// compile-time check: FSStore must implement Saver.
var _ Saver = (*FSStore)(nil)
