package agent

import (
	"testing"
)

func TestDetectConflicts_CommonFile(t *testing.T) {
	t.Parallel()

	intents := []FileIntent{
		{AgentName: "agent-a", Files: []string{"main.go", "util.go"}},
		{AgentName: "agent-b", Files: []string{"util.go", "config.go"}},
	}

	conflicts := DetectConflicts(intents)
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}

	c := conflicts[0]
	if c.Agent1 != "agent-a" || c.Agent2 != "agent-b" {
		t.Errorf("unexpected agent names: %q vs %q", c.Agent1, c.Agent2)
	}
	if len(c.SharedFiles) != 1 || c.SharedFiles[0] != "util.go" {
		t.Errorf("expected shared file [util.go], got %v", c.SharedFiles)
	}
}

func TestDetectConflicts_DisjointFiles(t *testing.T) {
	t.Parallel()

	intents := []FileIntent{
		{AgentName: "agent-x", Files: []string{"a.go", "b.go"}},
		{AgentName: "agent-y", Files: []string{"c.go", "d.go"}},
	}

	conflicts := DetectConflicts(intents)
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %d: %+v", len(conflicts), conflicts)
	}
}

func TestDetectConflicts_MultipleAgents(t *testing.T) {
	t.Parallel()

	intents := []FileIntent{
		{AgentName: "alpha", Files: []string{"shared.go", "alpha.go"}},
		{AgentName: "beta", Files: []string{"shared.go", "beta.go"}},
		{AgentName: "gamma", Files: []string{"shared.go", "gamma.go"}},
	}

	conflicts := DetectConflicts(intents)
	// alpha-beta, alpha-gamma, beta-gamma — all share "shared.go"
	if len(conflicts) != 3 {
		t.Errorf("expected 3 conflicts, got %d", len(conflicts))
	}
}

func TestDetectConflicts_EmptyIntents(t *testing.T) {
	t.Parallel()

	conflicts := DetectConflicts(nil)
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts for empty intents, got %d", len(conflicts))
	}
}

func TestDetectConflicts_SingleAgent(t *testing.T) {
	t.Parallel()

	intents := []FileIntent{
		{AgentName: "solo", Files: []string{"main.go"}},
	}

	conflicts := DetectConflicts(intents)
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts for single agent, got %d", len(conflicts))
	}
}

// ---- New gap-coverage tests -------------------------------------------------

// TestDetectConflicts_SameAgentTwice documents what happens when the same agent
// name appears twice in the intents slice. DetectConflicts uses a nested loop
// over index pairs (i, j) where i < j — it does not check for name equality, so
// two entries with the same name are treated as two different agents. If they
// share files a conflict is reported between "agent" and "agent".
func TestDetectConflicts_SameAgentTwice(t *testing.T) {
	t.Parallel()

	intents := []FileIntent{
		{AgentName: "agent", Files: []string{"shared.go", "only-a.go"}},
		{AgentName: "agent", Files: []string{"shared.go", "only-b.go"}},
	}

	conflicts := DetectConflicts(intents)

	// Current behavior: DetectConflicts finds the intersection ("shared.go") and
	// reports a conflict between the two entries even though they carry the same
	// agent name. The caller is responsible for de-duplicating agent names before
	// calling DetectConflicts.
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict (same name treated as two separate entries), got %d", len(conflicts))
	}
	c := conflicts[0]
	if c.Agent1 != "agent" || c.Agent2 != "agent" {
		t.Errorf("expected Agent1=Agent2='agent', got %q and %q", c.Agent1, c.Agent2)
	}
	if len(c.SharedFiles) != 1 || c.SharedFiles[0] != "shared.go" {
		t.Errorf("expected SharedFiles=[shared.go], got %v", c.SharedFiles)
	}
}

// TestDetectConflicts_EmptyFiles verifies that an agent with an empty Files slice
// never produces a conflict with any other agent, including one that has many
// files (the intersection of anything with an empty set is always empty).
func TestDetectConflicts_EmptyFiles(t *testing.T) {
	t.Parallel()

	intents := []FileIntent{
		{AgentName: "empty-agent", Files: []string{}},
		{AgentName: "busy-agent", Files: []string{"a.go", "b.go", "c.go"}},
	}

	conflicts := DetectConflicts(intents)
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts when one agent has empty Files, got %d: %+v", len(conflicts), conflicts)
	}
}

// TestDetectConflicts_CaseSensitive documents that file-path matching is
// case-sensitive: "pkg/Main.go" and "pkg/main.go" are treated as distinct paths.
// This matches the current sharedFiles implementation which uses a plain
// map[string]bool with exact string keys — no normalisation is applied.
func TestDetectConflicts_CaseSensitive(t *testing.T) {
	t.Parallel()

	intents := []FileIntent{
		{AgentName: "agent-upper", Files: []string{"pkg/Main.go"}},
		{AgentName: "agent-lower", Files: []string{"pkg/main.go"}},
	}

	conflicts := DetectConflicts(intents)

	// Current behavior: no conflict — the paths differ only in case and the
	// detector treats them as distinct files. On case-insensitive file systems
	// (e.g. macOS default HFS+) these paths would actually refer to the same
	// file, but DetectConflicts does not know about the underlying FS.
	if len(conflicts) != 0 {
		t.Errorf("expected 0 conflicts (case-sensitive comparison), got %d: %+v", len(conflicts), conflicts)
	}
}
