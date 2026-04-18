package pipeline

// Step defines a single step in the reaction chain.
type Step struct {
	Trigger EventType
	Agent   string
	Role    string // author or reviewer
	Gates   []string
	Next    EventType // event to emit after success
}

// DefaultChain returns the built-in reaction chain for the MVP.
func DefaultChain() []Step {
	return []Step{
		{
			Trigger: EventSpecChanged,
			Agent:   "spec",
			Role:    "author",
			Next:    EventInterfaceMerged, // after review + merge
		},
		{
			Trigger: EventInterfaceMerged,
			Agent:   "features",
			Role:    "author",
			Next:    EventFeaturesMerged,
		},
		{
			Trigger: EventFeaturesMerged,
			Agent:   "tests",
			Role:    "author",
			Next:    EventTestsMerged,
		},
		{
			Trigger: EventTestsMerged,
			Agent:   "docs",
			Role:    "author",
			Next:    EventDocsMerged,
		},
	}
}

// ReviewChain returns the review steps that apply to every author PR.
func ReviewChain() []string {
	return []string{"lint", "best-practices", "review"}
}

// SpecialistRegistry returns a map[agentName]Step for use with Supervisor.
func SpecialistRegistry() map[string]Step {
	reg := make(map[string]Step)
	for _, step := range DefaultChain() {
		reg[step.Agent] = step
	}
	return reg
}
