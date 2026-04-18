package agent

import (
	"testing"
)

func TestPhase_BudgetApplied(t *testing.T) {
	cfg := LoopConfig{
		Model:     "base-model",
		MaxTurns:  50,
		MaxTokens: 8192,
		PhaseBudgets: map[Phase]PhaseBudget{
			PhaseBuild: {MaxTokens: 4096, MaxTurns: 10, Model: "fast-model"},
			PhaseVerify: {MaxTokens: 2048},
		},
		CurrentPhase: PhaseBuild,
	}
	l := NewLoop(cfg)
	if l.cfg.MaxTokens != 4096 {
		t.Errorf("MaxTokens: want 4096, got %d", l.cfg.MaxTokens)
	}
	if l.cfg.MaxTurns != 10 {
		t.Errorf("MaxTurns: want 10, got %d", l.cfg.MaxTurns)
	}
	if l.cfg.Model != "fast-model" {
		t.Errorf("Model: want 'fast-model', got %q", l.cfg.Model)
	}
}

func TestPhase_PartialBudget(t *testing.T) {
	// Only MaxTokens overridden; MaxTurns and Model keep base values.
	cfg := LoopConfig{
		Model:        "base",
		MaxTurns:     50,
		MaxTokens:    8192,
		PhaseBudgets: map[Phase]PhaseBudget{PhaseVerify: {MaxTokens: 2048}},
		CurrentPhase: PhaseVerify,
	}
	l := NewLoop(cfg)
	if l.cfg.MaxTokens != 2048 {
		t.Errorf("MaxTokens: want 2048, got %d", l.cfg.MaxTokens)
	}
	if l.cfg.MaxTurns != 50 {
		t.Errorf("MaxTurns should stay 50, got %d", l.cfg.MaxTurns)
	}
	if l.cfg.Model != "base" {
		t.Errorf("Model should stay 'base', got %q", l.cfg.Model)
	}
}

func TestPhase_NoBudget_NoChange(t *testing.T) {
	cfg := LoopConfig{
		Model:        "base",
		MaxTurns:     30,
		MaxTokens:    1000,
		CurrentPhase: PhaseBuild, // no matching budget
	}
	l := NewLoop(cfg)
	if l.cfg.MaxTokens != 1000 {
		t.Errorf("MaxTokens unchanged: want 1000, got %d", l.cfg.MaxTokens)
	}
	if l.cfg.MaxTurns != 30 {
		t.Errorf("MaxTurns unchanged: want 30, got %d", l.cfg.MaxTurns)
	}
}

func TestPhase_NoCurrentPhase(t *testing.T) {
	cfg := LoopConfig{
		MaxTokens:    512,
		MaxTurns:     5,
		PhaseBudgets: map[Phase]PhaseBudget{PhaseBuild: {MaxTokens: 9999, MaxTurns: 99}},
		// CurrentPhase not set
	}
	l := NewLoop(cfg)
	if l.cfg.MaxTokens != 512 {
		t.Errorf("MaxTokens should be 512, got %d", l.cfg.MaxTokens)
	}
}

func TestPhase_Constants(t *testing.T) {
	phases := []Phase{PhaseContext, PhasePlan, PhaseBuild, PhaseVerify, PhaseCommit}
	seen := map[Phase]bool{}
	for _, p := range phases {
		if p == "" {
			t.Errorf("phase constant is empty string")
		}
		if seen[p] {
			t.Errorf("duplicate phase: %q", p)
		}
		seen[p] = true
	}
}
