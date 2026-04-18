package agent

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
)

// TestDynamicFanoutThreeNodes verifies that a router generating 3 Sends returns 3 results.
func TestDynamicFanoutThreeNodes(t *testing.T) {
	sg := NewStateGraph()
	router := func(*StateGraph) ([]Send, error) {
		return []Send{
			{NodeName: "work", State: map[string]any{"n": 1}},
			{NodeName: "work", State: map[string]any{"n": 2}},
			{NodeName: "work", State: map[string]any{"n": 3}},
		}, nil
	}
	nodes := map[string]NodeFn{
		"work": func(_ context.Context, state map[string]any) (map[string]any, error) {
			return map[string]any{"done": state["n"]}, nil
		},
	}

	results, err := DynamicFanout(context.Background(), sg, router, nodes, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Err != nil {
			t.Errorf("unexpected result error: %v", r.Err)
		}
	}
}

// TestDynamicFanoutPartialFailure verifies that one node error does not cancel others.
func TestDynamicFanoutPartialFailure(t *testing.T) {
	sg := NewStateGraph()
	router := func(*StateGraph) ([]Send, error) {
		return []Send{
			{NodeName: "ok", State: nil},
			{NodeName: "bad", State: nil},
			{NodeName: "ok", State: nil},
		}, nil
	}
	nodes := map[string]NodeFn{
		"ok":  func(_ context.Context, _ map[string]any) (map[string]any, error) { return nil, nil },
		"bad": func(_ context.Context, _ map[string]any) (map[string]any, error) { return nil, fmt.Errorf("boom") },
	}

	results, err := DynamicFanout(context.Background(), sg, router, nodes, 0)
	if err != nil {
		t.Fatalf("DynamicFanout itself should not error: %v", err)
	}
	errorCount := 0
	for _, r := range results {
		if r.Err != nil {
			errorCount++
		}
	}
	if errorCount != 1 {
		t.Errorf("expected exactly 1 error result, got %d", errorCount)
	}
}

// TestDynamicFanoutMaxConcurrency verifies the semaphore limits simultaneous goroutines.
func TestDynamicFanoutMaxConcurrency(t *testing.T) {
	const total = 10
	const maxConc = 3
	var active int64
	var peak int64

	sg := NewStateGraph()
	router := func(*StateGraph) ([]Send, error) {
		sends := make([]Send, total)
		for i := range sends {
			sends[i] = Send{NodeName: "w", State: nil}
		}
		return sends, nil
	}
	nodes := map[string]NodeFn{
		"w": func(_ context.Context, _ map[string]any) (map[string]any, error) {
			cur := atomic.AddInt64(&active, 1)
			for {
				old := atomic.LoadInt64(&peak)
				if cur <= old || atomic.CompareAndSwapInt64(&peak, old, cur) {
					break
				}
			}
			atomic.AddInt64(&active, -1)
			return nil, nil
		},
	}

	if _, err := DynamicFanout(context.Background(), sg, router, nodes, maxConc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if peak > maxConc {
		t.Errorf("peak concurrency %d exceeded maxConcurrency %d", peak, maxConc)
	}
}

// TestMapReduceSumDoubled verifies map doubles each int, reduce sums them.
func TestMapReduceSumDoubled(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	result, err := MapReduce(
		context.Background(),
		items,
		func(_ context.Context, n int) (int, error) { return n * 2, nil },
		func(results []int) (int, error) {
			sum := 0
			for _, v := range results {
				sum += v
			}
			return sum, nil
		},
		0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := (1 + 2 + 3 + 4 + 5) * 2 // 30
	if result != expected {
		t.Errorf("expected %d, got %d", expected, result)
	}
}

// TestMapReduceAllErrorsReturnsError verifies that all-error inputs propagate.
func TestMapReduceAllErrorsReturnsError(t *testing.T) {
	items := []int{1, 2}
	_, err := MapReduce(
		context.Background(),
		items,
		func(_ context.Context, _ int) (int, error) { return 0, fmt.Errorf("fail") },
		func(results []int) (int, error) { return 0, nil },
		0,
	)
	if err == nil {
		t.Error("expected error when all map calls fail")
	}
}
