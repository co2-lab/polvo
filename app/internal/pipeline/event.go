// Package pipeline implements the reaction chain orchestration.
package pipeline

// EventType identifies what triggered the pipeline.
type EventType string

const (
	EventSpecChanged      EventType = "spec_changed"
	EventInterfaceChanged EventType = "interface_changed"
	EventInterfaceMerged  EventType = "interface_merged"
	EventFeaturesMerged   EventType = "features_merged"
	EventTestsMerged      EventType = "tests_merged"
	EventDocsMerged       EventType = "docs_merged"
	EventPRApproved       EventType = "pr_approved"
	EventPRRejected       EventType = "pr_rejected"
)

// FileEvent represents a change detected in the repository.
type FileEvent struct {
	Type        EventType
	File        string
	Content     string
	Diff        string
	Owner       string // repo owner
	Repo        string // repo name
	Branch      string // source branch
	PRNumber    int    // associated PR number (if any)
	RetryCount  int    // number of retries for this step
}
