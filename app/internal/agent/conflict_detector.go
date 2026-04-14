package agent

// FileIntent describes what files an agent intends to modify.
type FileIntent struct {
	AgentName string
	Files     []string
}

// Conflict represents two agents that share at least one target file.
type Conflict struct {
	Agent1      string
	Agent2      string
	SharedFiles []string
}

// DetectConflicts finds all pairs of agents with overlapping file intentions.
// Used by a dispatcher before launching parallel agents to surface potential
// write-write conflicts.
func DetectConflicts(intents []FileIntent) []Conflict {
	var conflicts []Conflict

	for i := 0; i < len(intents); i++ {
		for j := i + 1; j < len(intents); j++ {
			shared := sharedFiles(intents[i].Files, intents[j].Files)
			if len(shared) > 0 {
				conflicts = append(conflicts, Conflict{
					Agent1:      intents[i].AgentName,
					Agent2:      intents[j].AgentName,
					SharedFiles: shared,
				})
			}
		}
	}

	return conflicts
}

// sharedFiles returns the intersection of two file slices.
func sharedFiles(a, b []string) []string {
	set := make(map[string]bool, len(a))
	for _, f := range a {
		set[f] = true
	}

	var shared []string
	seen := make(map[string]bool)
	for _, f := range b {
		if set[f] && !seen[f] {
			shared = append(shared, f)
			seen[f] = true
		}
	}
	return shared
}
