package agent

import (
	"context"
	"testing"
)

// TestReflectionPhases_LintThenTest verifies that when lint passes and test
// fails, RunPhases returns test feedback (not lint feedback).
func TestReflectionPhases_LintThenTest(t *testing.T) {
	cfg := ReflectionConfig{
		Enabled: true,
		Phases: []ReflectionPhase{
			{Name: "lint", Commands: []string{"true"}, MaxRetries: 1},   // always passes
			{Name: "test", Commands: []string{"false"}, MaxRetries: 1},  // always fails
		},
	}
	r := NewReflector(cfg, t.TempDir())
	if r == nil {
		t.Fatal("expected non-nil reflector")
	}

	feedback, results, allPassed := r.RunPhases(context.Background())

	if allPassed {
		t.Fatal("expected allPassed=false when test phase fails")
	}
	if feedback == "" {
		t.Error("expected non-empty feedback from failing test phase")
	}

	// Should have results for both phases (lint ran and passed, test ran and failed).
	if len(results) < 2 {
		t.Fatalf("expected at least 2 phase results, got %d", len(results))
	}
	if !results[0].Passed {
		t.Errorf("lint phase should have passed, but Passed=%v", results[0].Passed)
	}
	if results[0].PhaseName != "lint" {
		t.Errorf("expected first phase name 'lint', got %q", results[0].PhaseName)
	}
	if results[1].Passed {
		t.Errorf("test phase should have failed, but Passed=%v", results[1].Passed)
	}
	if results[1].PhaseName != "test" {
		t.Errorf("expected second phase name 'test', got %q", results[1].PhaseName)
	}
}

// TestReflectionPhases_StopsOnFirstPass verifies StopOnFirstPass=true:
// when lint passes, RunPhases stops and does not run test.
func TestReflectionPhases_StopsOnFirstPass(t *testing.T) {
	cfg := ReflectionConfig{
		Enabled:         true,
		StopOnFirstPass: true,
		Phases: []ReflectionPhase{
			{Name: "lint", Commands: []string{"true"}, MaxRetries: 1},  // always passes
			{Name: "test", Commands: []string{"false"}, MaxRetries: 1}, // would fail
		},
	}
	r := NewReflector(cfg, t.TempDir())
	if r == nil {
		t.Fatal("expected non-nil reflector")
	}

	_, results, allPassed := r.RunPhases(context.Background())

	if !allPassed {
		t.Fatal("expected allPassed=true when StopOnFirstPass and lint passes")
	}
	// Only lint should have run; test should not have been executed.
	if len(results) != 1 {
		t.Errorf("expected exactly 1 phase result (stopped after lint), got %d", len(results))
	}
	if len(results) > 0 && results[0].PhaseName != "lint" {
		t.Errorf("expected phase 'lint', got %q", results[0].PhaseName)
	}
}

// TestReflectionPhases_MaxRetriesPerPhase verifies that when lint fails MaxRetries
// times, RunPhases returns lint feedback and does NOT run the test phase.
func TestReflectionPhases_MaxRetriesPerPhase(t *testing.T) {
	cfg := ReflectionConfig{
		Enabled: true,
		Phases: []ReflectionPhase{
			{Name: "lint", Commands: []string{"false"}, MaxRetries: 2}, // always fails, 2 retries
			{Name: "test", Commands: []string{"true"}, MaxRetries: 1},  // would pass
		},
	}
	r := NewReflector(cfg, t.TempDir())
	if r == nil {
		t.Fatal("expected non-nil reflector")
	}

	feedback, results, allPassed := r.RunPhases(context.Background())

	if allPassed {
		t.Fatal("expected allPassed=false when lint fails")
	}
	if feedback == "" {
		t.Error("expected non-empty feedback from failing lint phase")
	}

	// Only lint phase result should be present (test was not run).
	if len(results) != 1 {
		t.Errorf("expected exactly 1 phase result (lint failed, test skipped), got %d", len(results))
	}
	if len(results) > 0 {
		if results[0].PhaseName != "lint" {
			t.Errorf("expected phase 'lint', got %q", results[0].PhaseName)
		}
		if results[0].Passed {
			t.Error("lint phase should have failed")
		}
	}
}

// TestReflectionConfig_BackwardCompat verifies that legacy flat Commands/MaxRetries
// still works via effectivePhases().
func TestReflectionConfig_BackwardCompat(t *testing.T) {
	t.Run("flat Commands synthesizes default phase", func(t *testing.T) {
		cfg := ReflectionConfig{
			Enabled:    true,
			Commands:   []string{"true"},
			MaxRetries: 5,
		}
		phases := cfg.effectivePhases()
		if len(phases) != 1 {
			t.Fatalf("expected 1 synthesized phase, got %d", len(phases))
		}
		if phases[0].Name != "default" {
			t.Errorf("expected phase name 'default', got %q", phases[0].Name)
		}
		if phases[0].MaxRetries != 5 {
			t.Errorf("expected MaxRetries=5, got %d", phases[0].MaxRetries)
		}
	})

	t.Run("no Commands and no Phases → nil", func(t *testing.T) {
		cfg := ReflectionConfig{Enabled: true}
		phases := cfg.effectivePhases()
		if len(phases) != 0 {
			t.Errorf("expected nil/empty phases, got %d", len(phases))
		}
	})

	t.Run("Phases takes precedence over Commands", func(t *testing.T) {
		cfg := ReflectionConfig{
			Enabled:  true,
			Commands: []string{"true"},
			Phases: []ReflectionPhase{
				{Name: "custom", Commands: []string{"true"}},
			},
		}
		phases := cfg.effectivePhases()
		if len(phases) != 1 {
			t.Fatalf("expected 1 phase, got %d", len(phases))
		}
		if phases[0].Name != "custom" {
			t.Errorf("expected phase name 'custom', got %q", phases[0].Name)
		}
	})

	t.Run("RunAndReflect still works (legacy)", func(t *testing.T) {
		cfg := ReflectionConfig{
			Enabled:    true,
			Commands:   []string{"true"},
			MaxRetries: 2,
		}
		r := NewReflector(cfg, t.TempDir())
		if r == nil {
			t.Fatal("expected non-nil reflector")
		}
		feedback, passed := r.RunAndReflect(context.Background())
		if !passed {
			t.Errorf("expected passed=true for 'true' command, got feedback: %q", feedback)
		}
	})

	t.Run("RunAndReflect reports failure (legacy)", func(t *testing.T) {
		cfg := ReflectionConfig{
			Enabled:    true,
			Commands:   []string{"false"},
			MaxRetries: 1,
		}
		r := NewReflector(cfg, t.TempDir())
		if r == nil {
			t.Fatal("expected non-nil reflector")
		}
		feedback, passed := r.RunAndReflect(context.Background())
		if passed {
			t.Error("expected passed=false for 'false' command")
		}
		if feedback == "" {
			t.Error("expected non-empty feedback on failure")
		}
	})
}

// TestNewReflector_NilWhenDisabled verifies NewReflector returns nil when
// reflection is disabled or no commands/phases are provided.
func TestNewReflector_NilWhenDisabled(t *testing.T) {
	t.Run("disabled=false → nil", func(t *testing.T) {
		cfg := ReflectionConfig{Enabled: false, Commands: []string{"true"}}
		if r := NewReflector(cfg, t.TempDir()); r != nil {
			t.Error("expected nil when Enabled=false")
		}
	})

	t.Run("enabled but no commands/phases → nil", func(t *testing.T) {
		cfg := ReflectionConfig{Enabled: true}
		if r := NewReflector(cfg, t.TempDir()); r != nil {
			t.Error("expected nil when no commands and no phases")
		}
	})

	t.Run("enabled with Phases → non-nil", func(t *testing.T) {
		cfg := ReflectionConfig{
			Enabled: true,
			Phases:  []ReflectionPhase{{Name: "check", Commands: []string{"true"}}},
		}
		if r := NewReflector(cfg, t.TempDir()); r == nil {
			t.Error("expected non-nil when Enabled=true and Phases set")
		}
	})
}

// TestRunPhases_NilReflector verifies RunPhases is safe on nil receiver.
func TestRunPhases_NilReflector(t *testing.T) {
	var r *Reflector
	feedback, results, allPassed := r.RunPhases(context.Background())
	if !allPassed {
		t.Error("nil Reflector: expected allPassed=true")
	}
	if feedback != "" {
		t.Errorf("nil Reflector: expected empty feedback, got %q", feedback)
	}
	if len(results) != 0 {
		t.Errorf("nil Reflector: expected no results, got %d", len(results))
	}
}
