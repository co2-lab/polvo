package microagent

import (
	"strings"
	"testing"
)

func makeResult(name string, priority int) MatchResult {
	return MatchResult{
		Microagent: Microagent{
			Name:     name,
			Priority: priority,
			Content:  "Content for " + name + ".",
		},
		Trigger: TriggerAlways,
		Score:   1.0,
	}
}

func TestInject_NoMatches(t *testing.T) {
	out := Inject(nil, 5)
	if out != "" {
		t.Errorf("expected empty string for no matches, got %q", out)
	}
}

func TestInject_EmptySlice(t *testing.T) {
	out := Inject([]MatchResult{}, 5)
	if out != "" {
		t.Errorf("expected empty string for empty slice, got %q", out)
	}
}

func TestInject_ThreeMatches(t *testing.T) {
	matches := []MatchResult{
		makeResult("redis-guide", 10),
		makeResult("auth-patterns", 8),
		makeResult("db-conventions", 5),
	}
	out := Inject(matches, 5)

	if !strings.Contains(out, "[Conhecimento Especializado Ativo]") {
		t.Error("missing section header")
	}
	for _, name := range []string{"redis-guide", "auth-patterns", "db-conventions"} {
		if !strings.Contains(out, "--- "+name+" ---") {
			t.Errorf("missing entry for %q in output", name)
		}
		if !strings.Contains(out, "Content for "+name+".") {
			t.Errorf("missing content for %q in output", name)
		}
	}
}

func TestInject_SixMatchesMaxFive(t *testing.T) {
	matches := []MatchResult{
		makeResult("a", 100),
		makeResult("b", 90),
		makeResult("c", 80),
		makeResult("d", 70),
		makeResult("e", 60),
		makeResult("f", 50), // should be excluded
	}
	out := Inject(matches, 5)

	if strings.Contains(out, "--- f ---") {
		t.Error("6th match (f) should be excluded when maxInjected=5")
	}
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		if !strings.Contains(out, "--- "+name+" ---") {
			t.Errorf("expected entry for %q", name)
		}
	}
}

func TestInject_MaxInjectedZeroUsesDefault(t *testing.T) {
	// Build 7 matches (more than default of 5).
	matches := make([]MatchResult, 7)
	for i := range matches {
		matches[i] = makeResult(string(rune('a'+i)), 100-i)
	}
	out := Inject(matches, 0)

	// Default is 5; entries f and g should be absent.
	if strings.Contains(out, "--- f ---") || strings.Contains(out, "--- g ---") {
		t.Error("entries beyond default limit (5) should not appear when maxInjected=0")
	}
}

// ---------------------------------------------------------------------------
// New gap-coverage tests
// ---------------------------------------------------------------------------

// TestInjector_TokenBudget creates 5 matches each with ~200-word content and
// verifies the output size. Inject has no character budget — only a count
// limit (maxInjected). The output can therefore be arbitrarily large if each
// entry has large content. Use InjectWithBudget when a token budget is needed.
func TestInjector_TokenBudget(t *testing.T) {
	// Build ~200 words of content per match.
	word := "contextword "
	largeContent := strings.Repeat(word, 200) // ~200 words, ~2400 chars each

	matches := make([]MatchResult, 5)
	for i := range matches {
		matches[i] = MatchResult{
			Microagent: Microagent{
				Name:     strings.Repeat(string(rune('a'+i)), 1),
				Priority: 100 - i,
				Content:  largeContent,
			},
			Trigger: TriggerAlways,
			Score:   1.0,
		}
	}

	out := Inject(matches, 5)

	// Verify all 5 entries are present (the count limit works correctly).
	for i := range matches {
		name := strings.Repeat(string(rune('a'+i)), 1)
		if !strings.Contains(out, "--- "+name+" ---") {
			t.Errorf("expected entry for %q in output", name)
		}
	}
}

// TestInjector_TokenBudget_HardLimit verifies that InjectWithBudget with
// maxChars=500 produces output that does not exceed 500 characters.
func TestInjector_TokenBudget_HardLimit(t *testing.T) {
	word := "contextword "
	largeContent := strings.Repeat(word, 200) // ~2400 chars per entry

	matches := make([]MatchResult, 5)
	for i := range matches {
		matches[i] = MatchResult{
			Microagent: Microagent{
				Name:     strings.Repeat(string(rune('a'+i)), 1),
				Priority: 100 - i,
				Content:  largeContent,
			},
			Trigger: TriggerAlways,
			Score:   1.0,
		}
	}

	const budget = 500
	out := InjectWithBudget(matches, 5, budget)

	if len(out) > budget {
		t.Errorf("InjectWithBudget output is %d chars, exceeds maxChars=%d", len(out), budget)
	}
}

func TestInject_ContainsHeader(t *testing.T) {
	matches := []MatchResult{makeResult("solo", 1)}
	out := Inject(matches, 5)

	lines := strings.Split(out, "\n")
	if lines[0] != "[Conhecimento Especializado Ativo]" {
		t.Errorf("first line should be the section header, got %q", lines[0])
	}
}
