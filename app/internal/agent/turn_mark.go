package agent

// TurnMark classifies a user's annotation on a completed conversation turn.
type TurnMark int8

const (
	TurnMarkNone      TurnMark = 0 // default, no annotation
	TurnMarkUseful    TurnMark = 1 // explicitly marked as useful (visual emphasis)
	TurnMarkDismissed TurnMark = 2 // collapsed; content replaced with summary in context
)

// TurnMetadata holds the user-applied annotation for one completed turn.
type TurnMetadata struct {
	Index   int      // 0-based turn index
	Mark    TurnMark
	Summary string // generated on-demand when Mark == TurnMarkDismissed
}
