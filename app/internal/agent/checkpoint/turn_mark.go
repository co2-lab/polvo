package checkpoint

// TurnMarkRecord is the serialisable form of a turn annotation.
// Stored as sessions/<sessionID>/turn_marks.json (overwritten atomically).
type TurnMarkRecord struct {
	TurnIndex int    `json:"turn_index"`
	Mark      int8   `json:"mark"`    // 0=none, 1=useful, 2=dismissed
	Summary   string `json:"summary,omitempty"`
}
