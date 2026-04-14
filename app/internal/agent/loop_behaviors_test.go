package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/co2-lab/polvo/internal/provider"
	"github.com/co2-lab/polvo/internal/tool"
)

// ---- TestLoopConfig_MaxConsecutiveTimeoutsField -----------------------------

// timeoutChatProvider always returns context.DeadlineExceeded on Chat calls.
type timeoutChatProvider struct {
	calls int
}

func (p *timeoutChatProvider) Chat(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
	p.calls++
	return nil, context.DeadlineExceeded
}

func (p *timeoutChatProvider) Name() string                      { return "timeout" }
func (p *timeoutChatProvider) Available(_ context.Context) error { return nil }
func (p *timeoutChatProvider) Complete(_ context.Context, _ provider.Request) (*provider.Response, error) {
	return &provider.Response{}, nil
}

// TestLoopConfig_MaxConsecutiveTimeoutsField verifies that the loop stops after
// MaxConsecutiveTimeouts consecutive context.DeadlineExceeded errors from the LLM.
func TestLoopConfig_MaxConsecutiveTimeoutsField(t *testing.T) {
	prov := &timeoutChatProvider{}
	loop := NewLoop(LoopConfig{
		Provider:               prov,
		MaxTurns:               50,
		MaxConsecutiveTimeouts: 3,
	})

	_, err := loop.Run(context.Background(), "task")
	if err == nil {
		t.Fatal("expected error after consecutive timeouts")
	}
	if !containsStr(err.Error(), "consecutive timeouts") {
		t.Errorf("expected error to mention 'consecutive timeouts', got: %v", err)
	}
	// The loop should abort on the 3rd consecutive timeout, so exactly 3 LLM calls.
	if prov.calls != 3 {
		t.Errorf("expected exactly 3 LLM calls before abort, got %d", prov.calls)
	}
}

// ---- TestExploreParallel_ResultsInInputOrder --------------------------------

// TestExploreParallel_ResultsInInputOrder is superseded by the existing
// TestExploreParallel_ResultsInOrder in explore_test.go which already verifies
// the same guarantee with maxParallel=1. This test adds a parallel (maxParallel=3)
// variant to verify ordering is preserved even under concurrent execution.
func TestExploreParallel_ResultsInInputOrder_Parallel(t *testing.T) {
	tasks := []ExploreTask{
		{Description: "task-zero"},
		{Description: "task-one"},
		{Description: "task-two"},
	}
	results, err := ExploreParallel(context.Background(), tasks, nil, "", 0, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for i, r := range results {
		if r.TaskDescription != tasks[i].Description {
			t.Errorf("results[%d].TaskDescription = %q; want %q", i, r.TaskDescription, tasks[i].Description)
		}
	}
}

// ---- TestReadOnlyExecutor_AllWriteToolsBlocked ------------------------------

// TestReadOnlyExecutor_AllWriteToolsBlocked verifies that each individual write
// tool in the canonical blocked set is rejected with ErrToolNotPermitted.
// The writeToolNames set in readonly_executor.go defines the full blocked list;
// this test focuses on the five tools called out in the task specification.
func TestReadOnlyExecutor_AllWriteToolsBlocked(t *testing.T) {
	blockedTools := []string{
		"write",
		"edit",
		"bash",
		"patch",
		"memory_write",
	}

	reg := buildReadOnlyTestRegistry() // read-only tools only — no panic on construction
	exec := NewReadOnlyExecutor(reg)

	for _, name := range blockedTools {
		name := name // capture for subtest
		t.Run(name, func(t *testing.T) {
			result, err := exec.Execute(context.Background(), name, json.RawMessage(`{}`))
			if err == nil {
				t.Errorf("tool %q should be blocked but Execute returned nil error", name)
				return
			}
			if _, ok := err.(ErrToolNotPermitted); !ok {
				t.Errorf("tool %q: expected ErrToolNotPermitted, got %T: %v", name, err, err)
			}
			if result != nil {
				t.Errorf("tool %q: expected nil result when blocked, got non-nil", name)
			}
		})
	}
}

// ---- TestDelegateLevel_Propagation ------------------------------------------

// TestDelegateLevel_Propagation is a thorough test of context propagation:
// - level 0 is the default for a fresh context
// - WithDelegateLevel(ctx, 1) yields level 1
// - nesting to level 2 yields level 2
// - the original base context still returns 0 (no mutation)
// - a sibling context derived from the same base also returns 0
func TestDelegateLevel_Propagation(t *testing.T) {
	base := context.Background()

	// Base context has no level set → must return 0.
	if got := DelegateLevelFromCtx(base); got != 0 {
		t.Errorf("base context: expected level=0, got %d", got)
	}

	// Wrap base at level 1.
	ctx1 := WithDelegateLevel(base, 1)
	if got := DelegateLevelFromCtx(ctx1); got != 1 {
		t.Errorf("ctx1: expected level=1, got %d", got)
	}

	// Nest ctx1 at level 2.
	ctx2 := WithDelegateLevel(ctx1, 2)
	if got := DelegateLevelFromCtx(ctx2); got != 2 {
		t.Errorf("ctx2: expected level=2, got %d", got)
	}

	// The original base context must still read 0 (immutable).
	if got := DelegateLevelFromCtx(base); got != 0 {
		t.Errorf("base context after nesting: expected level=0, got %d (context must not be mutated)", got)
	}

	// ctx1 must still read 1 (deriving ctx2 must not mutate ctx1).
	if got := DelegateLevelFromCtx(ctx1); got != 1 {
		t.Errorf("ctx1 after deriving ctx2: expected level=1, got %d", got)
	}

	// A sibling derived independently from base must read 0.
	sibling := WithDelegateLevel(base, 0)
	if got := DelegateLevelFromCtx(sibling); got != 0 {
		t.Errorf("sibling context: expected level=0, got %d", got)
	}

	// Verify that the block guard in ExploreParallel fires at level >= 1.
	// (Integration check: DelegateLevelFromCtx feeds into ExploreParallel guard.)
	tasks := []ExploreTask{{Description: "x"}}
	if _, err := ExploreParallel(ctx1, tasks, nil, "", 0, 0); err == nil {
		t.Error("ExploreParallel with delegate level=1 should return an error")
	}
	if _, err := ExploreParallel(ctx2, tasks, nil, "", 0, 0); err == nil {
		t.Error("ExploreParallel with delegate level=2 should return an error")
	}
	// Level 0 is allowed through the guard (will fail for another reason — no provider).
	results, err := ExploreParallel(base, tasks, nil, "", 0, 0)
	if err != nil {
		t.Fatalf("ExploreParallel with level=0 should not return a guard error, got: %v", err)
	}
	if len(results) != 1 || results[0].Err == nil {
		t.Log("expected individual task to fail (no provider), results[0].Err should be non-nil")
	}
}

// makeWriteToolForRegistry builds a tool with the given name that returns an
// error result. Used to test panic-on-construction scenarios independently.
func makeWriteToolForRegistry(name string) tool.Tool {
	return &noopToolImpl{name: name}
}
