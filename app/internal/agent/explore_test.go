package agent

import (
	"context"
	"testing"
)

// ---- TestDelegateLevelFromCtx -----------------------------------------------

func TestDelegateLevelFromCtx_DefaultIsZero(t *testing.T) {
	level := DelegateLevelFromCtx(context.Background())
	if level != 0 {
		t.Errorf("expected delegate level 0 for fresh context, got %d", level)
	}
}

func TestWithDelegateLevel(t *testing.T) {
	ctx := WithDelegateLevel(context.Background(), 1)
	level := DelegateLevelFromCtx(ctx)
	if level != 1 {
		t.Errorf("expected delegate level 1, got %d", level)
	}
}

func TestWithDelegateLevel_Nested(t *testing.T) {
	ctx := WithDelegateLevel(context.Background(), 1)
	ctx = WithDelegateLevel(ctx, 2)
	level := DelegateLevelFromCtx(ctx)
	if level != 2 {
		t.Errorf("expected delegate level 2, got %d", level)
	}
}

// ---- TestExploreParallel_EmptyTasks -----------------------------------------

func TestExploreParallel_EmptyTasks(t *testing.T) {
	results, err := ExploreParallel(context.Background(), nil, nil, "", 0, 0)
	if err != nil {
		t.Fatalf("expected no error for empty tasks, got: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results for empty tasks, got %v", results)
	}
}

// ---- TestExploreParallel_BlockedInSubagent ----------------------------------

func TestExploreParallel_BlockedInSubagent(t *testing.T) {
	// When called from a subagent context (DelegateLevel >= 1), must return error
	ctx := WithDelegateLevel(context.Background(), 1)
	tasks := []ExploreTask{
		{Description: "explore something"},
	}
	_, err := ExploreParallel(ctx, tasks, nil, "", 0, 0)
	if err == nil {
		t.Fatal("expected error when called from subagent context, got nil")
	}
	if !containsStr(err.Error(), "explore not available in subagents") {
		t.Errorf("expected 'explore not available in subagents' in error, got: %v", err)
	}
}

// ---- TestExploreParallel_NilExecReturnsError --------------------------------

func TestExploreParallel_NilExecReturnsError(t *testing.T) {
	// With a nil executor, each task should fail with an error
	// (no provider available), but the function should not panic
	tasks := []ExploreTask{
		{Description: "task 1"},
		{Description: "task 2"},
	}
	results, err := ExploreParallel(context.Background(), tasks, nil, "", 0, 0)
	if err != nil {
		t.Fatalf("ExploreParallel itself should not error, got: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Each result should have an error since there's no provider
	for i, r := range results {
		if r.Err == nil {
			t.Errorf("results[%d]: expected error (no provider), got nil", i)
		}
	}
}

// ---- TestExploreParallel_ResultsInOrder -------------------------------------

func TestExploreParallel_ResultsInOrder(t *testing.T) {
	// Verify results are returned in the same order as input tasks
	tasks := []ExploreTask{
		{Description: "task A"},
		{Description: "task B"},
		{Description: "task C"},
	}
	results, err := ExploreParallel(context.Background(), tasks, nil, "", 0, 1) // maxParallel=1 to force sequential
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

// ---- TestExploreParallel_ContextCancel --------------------------------------

func TestExploreParallel_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	tasks := []ExploreTask{
		{Description: "task 1"},
	}
	results, err := ExploreParallel(ctx, tasks, nil, "", 0, 0)
	// Should not panic; err might be nil (ExploreParallel itself succeeds)
	// but individual results should have errors due to cancelled context
	if err != nil {
		t.Logf("ExploreParallel returned error (acceptable): %v", err)
		return
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// The result should fail since the context was cancelled
	// (either context cancelled or no provider)
	if results[0].Err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

// ---- TestExploreParallel_DefaultMaxParallel ---------------------------------

func TestExploreParallel_DefaultMaxParallel(t *testing.T) {
	// maxParallel=0 should use defaultExploreMaxParallel (5), not deadlock
	tasks := make([]ExploreTask, 3)
	for i := range tasks {
		tasks[i] = ExploreTask{Description: "task"}
	}
	results, err := ExploreParallel(context.Background(), tasks, nil, "", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
}

// ---- TestExploreTask_TokenBudgetDistribution --------------------------------

func TestExploreTask_TokenBudgetDistribution(t *testing.T) {
	// Individual task budget overrides the distributed budget
	tasks := []ExploreTask{
		{Description: "task 1", TokenBudget: 5000},
		{Description: "task 2"}, // uses distributed budget
	}
	// globalTokenBudget=10000, 2 tasks → perTaskBudget=5000
	results, err := ExploreParallel(context.Background(), tasks, nil, "", 10000, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Both tasks should have populated TaskDescription
	if results[0].TaskDescription != "task 1" {
		t.Errorf("expected task 1, got %q", results[0].TaskDescription)
	}
	if results[1].TaskDescription != "task 2" {
		t.Errorf("expected task 2, got %q", results[1].TaskDescription)
	}
}
