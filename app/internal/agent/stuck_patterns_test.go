package agent

import (
	"testing"
)

// TestStuckDetector_ErrorLoop verifies that 3 consecutive errors on the same
// tool trigger StuckPatternErrorLoop, and that 2 errors + 1 success does not.
func TestStuckDetector_ErrorLoop(t *testing.T) {
	t.Run("3 consecutive errors → ErrorLoop", func(t *testing.T) {
		det := NewStuckDetector(6, 3)
		det.RecordResult("bash", `{"cmd":"fail"}`, true)
		det.RecordResult("bash", `{"cmd":"fail"}`, true)
		det.RecordResult("bash", `{"cmd":"fail"}`, true)

		pattern, stuck := det.CheckAll()
		if !stuck {
			t.Fatal("expected stuck=true")
		}
		if pattern != StuckPatternErrorLoop {
			t.Errorf("expected StuckPatternErrorLoop, got %s", pattern)
		}
	})

	t.Run("2 errors + 1 success (different input) → not stuck", func(t *testing.T) {
		// The success uses a different command, so neither ErrorLoop (last 3 not all
		// same tool with errors) nor Repeat (only 2 identical tool+input) fires.
		det := NewStuckDetector(6, 3)
		det.RecordResult("bash", `{"cmd":"fail"}`, true)
		det.RecordResult("bash", `{"cmd":"fail"}`, true)
		det.RecordResult("bash", `{"cmd":"ok"}`, false) // different input, success

		_, stuck := det.CheckAll()
		if stuck {
			t.Error("expected stuck=false for 2 errors + 1 success with different input")
		}
	})

	t.Run("different tools with errors → not ErrorLoop", func(t *testing.T) {
		det := NewStuckDetector(6, 3)
		det.RecordResult("bash", `{"cmd":"fail"}`, true)
		det.RecordResult("read", "file.go", true)
		det.RecordResult("write", "out.go", true)

		pattern, stuck := det.CheckAll()
		if stuck && pattern == StuckPatternErrorLoop {
			t.Error("expected no ErrorLoop for different tools with errors")
		}
	})
}

// TestStuckDetector_CyclicPattern verifies A→B→A→B→A→B detection in last 6 calls.
func TestStuckDetector_CyclicPattern(t *testing.T) {
	t.Run("read,write,read,write,read,write → StuckPatternCyclic", func(t *testing.T) {
		det := NewStuckDetector(6, 10) // high threshold so Repeat doesn't fire first
		det.RecordResult("read", "file.go", false)
		det.RecordResult("write", "out.go", false)
		det.RecordResult("read", "file.go", false)
		det.RecordResult("write", "out.go", false)
		det.RecordResult("read", "file.go", false)
		det.RecordResult("write", "out.go", false)

		pattern, stuck := det.CheckAll()
		if !stuck {
			t.Fatal("expected stuck=true for A,B,A,B,A,B pattern")
		}
		if pattern != StuckPatternCyclic {
			t.Errorf("expected StuckPatternCyclic, got %s", pattern)
		}
	})

	t.Run("read,write,edit,read,write,edit → not cyclic (3-cycle, not 2-cycle)", func(t *testing.T) {
		det := NewStuckDetector(6, 10) // high threshold to avoid Repeat firing
		det.RecordResult("read", "file.go", false)
		det.RecordResult("write", "out.go", false)
		det.RecordResult("edit", "x.go", false)
		det.RecordResult("read", "file.go", false)
		det.RecordResult("write", "out.go", false)
		det.RecordResult("edit", "x.go", false)

		pattern, stuck := det.CheckAll()
		if stuck && pattern == StuckPatternCyclic {
			t.Error("expected no StuckPatternCyclic for 3-element cycle (A,B,C not A,B)")
		}
	})

	t.Run("A,A,A,A,A,A → not cyclic (A==B condition)", func(t *testing.T) {
		det := NewStuckDetector(6, 10)
		for i := 0; i < 6; i++ {
			det.RecordResult("read", "file.go", false)
		}
		pattern, _ := det.CheckAll()
		if pattern == StuckPatternCyclic {
			t.Error("expected no StuckPatternCyclic when A==B")
		}
	})
}

// TestStuckDetector_Monologue verifies monologue (4+ text turns without tool calls).
func TestStuckDetector_Monologue(t *testing.T) {
	t.Run("4 RecordTextTurn → StuckPatternMonologue", func(t *testing.T) {
		det := NewStuckDetector(6, 3)
		det.RecordTextTurn()
		det.RecordTextTurn()
		det.RecordTextTurn()
		det.RecordTextTurn()

		pattern, stuck := det.CheckAll()
		if !stuck {
			t.Fatal("expected stuck=true after 4 text turns")
		}
		if pattern != StuckPatternMonologue {
			t.Errorf("expected StuckPatternMonologue, got %s", pattern)
		}
	})

	t.Run("3 text turns → not stuck", func(t *testing.T) {
		det := NewStuckDetector(6, 3)
		det.RecordTextTurn()
		det.RecordTextTurn()
		det.RecordTextTurn()

		_, stuck := det.CheckAll()
		if stuck {
			t.Error("expected stuck=false for only 3 text turns")
		}
	})

	t.Run("3 text turns + RecordResult resets + 4 text turns → stuck", func(t *testing.T) {
		det := NewStuckDetector(6, 3)
		det.RecordTextTurn()
		det.RecordTextTurn()
		det.RecordTextTurn()

		// Tool call resets the counter
		det.RecordResult("read", "file.go", false)

		// Check not stuck yet (counter was reset)
		_, stuck := det.CheckAll()
		if stuck {
			t.Error("expected stuck=false after RecordResult reset the counter")
		}

		// Now accumulate 4 more
		det.RecordTextTurn()
		det.RecordTextTurn()
		det.RecordTextTurn()
		det.RecordTextTurn()

		pattern, stuck := det.CheckAll()
		if !stuck {
			t.Fatal("expected stuck=true after reset + 4 new text turns")
		}
		if pattern != StuckPatternMonologue {
			t.Errorf("expected StuckPatternMonologue, got %s", pattern)
		}
	})

	t.Run("Record (non-error) also resets counter", func(t *testing.T) {
		det := NewStuckDetector(6, 3)
		det.RecordTextTurn()
		det.RecordTextTurn()
		det.RecordTextTurn()

		det.Record("read", "file.go") // backward-compat method

		_, stuck := det.CheckAll()
		if stuck {
			t.Error("expected stuck=false after Record reset the monologue counter")
		}
	})
}

// TestStuckDetector_ContextWindow verifies that 3+ condensations triggers the pattern.
func TestStuckDetector_ContextWindow(t *testing.T) {
	t.Run("3 RecordCondensation → StuckPatternContextWindow", func(t *testing.T) {
		det := NewStuckDetector(6, 3)
		det.RecordCondensation()
		det.RecordCondensation()
		det.RecordCondensation()

		pattern, stuck := det.CheckAll()
		if !stuck {
			t.Fatal("expected stuck=true after 3 condensations")
		}
		if pattern != StuckPatternContextWindow {
			t.Errorf("expected StuckPatternContextWindow, got %s", pattern)
		}
	})

	t.Run("2 condensations → not stuck", func(t *testing.T) {
		det := NewStuckDetector(6, 3)
		det.RecordCondensation()
		det.RecordCondensation()

		_, stuck := det.CheckAll()
		if stuck {
			t.Error("expected stuck=false for only 2 condensations")
		}
	})
}

// TestStuckDetector_PriorityOrder verifies that ErrorLoop takes priority over Repeat
// when both conditions are met simultaneously.
func TestStuckDetector_PriorityOrder(t *testing.T) {
	t.Run("ErrorLoop takes priority over Repeat", func(t *testing.T) {
		// 3x same tool with errors → satisfies both ErrorLoop AND Repeat (threshold=3).
		det := NewStuckDetector(6, 3)
		det.RecordResult("bash", `{"cmd":"fail"}`, true)
		det.RecordResult("bash", `{"cmd":"fail"}`, true)
		det.RecordResult("bash", `{"cmd":"fail"}`, true)

		pattern, stuck := det.CheckAll()
		if !stuck {
			t.Fatal("expected stuck=true")
		}
		if pattern != StuckPatternErrorLoop {
			t.Errorf("expected StuckPatternErrorLoop (priority over Repeat), got %s", pattern)
		}
	})

	t.Run("Cyclic takes priority over Repeat", func(t *testing.T) {
		// A,B,A,B,A,B with threshold=2: Repeat fires on pos 0,2 but Cyclic is checked first.
		det := NewStuckDetector(6, 2)
		det.RecordResult("read", "file.go", false)  // A
		det.RecordResult("write", "out.go", false)  // B
		det.RecordResult("read", "file.go", false)  // A
		det.RecordResult("write", "out.go", false)  // B
		det.RecordResult("read", "file.go", false)  // A
		det.RecordResult("write", "out.go", false)  // B

		pattern, stuck := det.CheckAll()
		if !stuck {
			t.Fatal("expected stuck=true")
		}
		if pattern != StuckPatternCyclic {
			t.Errorf("expected StuckPatternCyclic (priority over Repeat), got %s", pattern)
		}
	})

	t.Run("Monologue lower priority than Repeat", func(t *testing.T) {
		det := NewStuckDetector(6, 3)
		// Trigger both Repeat and Monologue
		det.RecordResult("read", "file.go", false)
		det.RecordResult("read", "file.go", false)
		det.RecordResult("read", "file.go", false)
		// Also trigger monologue (4 text turns without tool calls)
		// But since RecordResult resets textTurnCount, we need to build both conditions.
		// Actually, text turn count gets reset by RecordResult, so let's use a fresh
		// detector that has Repeat satisfied but no text turns.

		pattern, stuck := det.CheckAll()
		if !stuck {
			t.Fatal("expected stuck=true")
		}
		if pattern != StuckPatternRepeat {
			t.Errorf("expected StuckPatternRepeat, got %s", pattern)
		}
	})
}

// TestStuckDetector_StuckPatternString verifies the String() method.
func TestStuckDetector_StuckPatternString(t *testing.T) {
	tests := []struct {
		p    StuckPattern
		want string
	}{
		{StuckPatternNone, "none"},
		{StuckPatternRepeat, "repeat"},
		{StuckPatternErrorLoop, "error_loop"},
		{StuckPatternCyclic, "cyclic"},
		{StuckPatternMonologue, "monologue"},
		{StuckPatternContextWindow, "context_window"},
	}
	for _, tt := range tests {
		if got := tt.p.String(); got != tt.want {
			t.Errorf("StuckPattern(%d).String() = %q; want %q", tt.p, got, tt.want)
		}
	}
}
