package agent

import (
	"context"
	"fmt"
	"sync"
)

// Send encapsulates the destination and initial state for a dynamic parallel execution.
// Equivalent to the Send API in LangGraph.
type Send struct {
	// NodeName is the identifier of the node (agent type) to execute.
	NodeName string
	// State is the initial state for this specific execution.
	State map[string]any
}

// FanoutResult is the outcome of a single Send execution.
type FanoutResult struct {
	NodeName string
	State    map[string]any // state produced by the node
	Err      error
}

// RouterFn examines the current StateGraph and dynamically generates a list of
// Sends for parallel execution.
type RouterFn func(state *StateGraph) ([]Send, error)

// NodeFn is the function executed for each Send. Receives the Send's state and
// returns a map of state updates.
type NodeFn func(ctx context.Context, state map[string]any) (map[string]any, error)

// DynamicFanout runs the router to generate Sends, then executes each NodeFn in
// parallel with the corresponding state.
// maxConcurrency <= 0 means unbounded concurrency.
// A node error is recorded in FanoutResult.Err; other nodes continue executing.
func DynamicFanout(
	ctx context.Context,
	sg *StateGraph,
	router RouterFn,
	nodes map[string]NodeFn,
	maxConcurrency int,
) ([]FanoutResult, error) {
	sends, err := router(sg)
	if err != nil {
		return nil, fmt.Errorf("router: %w", err)
	}
	if len(sends) == 0 {
		return nil, nil
	}

	if maxConcurrency <= 0 {
		maxConcurrency = len(sends)
	}

	sem := make(chan struct{}, maxConcurrency)
	results := make([]FanoutResult, len(sends))
	var wg sync.WaitGroup

	for i, s := range sends {
		wg.Add(1)
		go func(idx int, snd Send) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			fn, ok := nodes[snd.NodeName]
			if !ok {
				results[idx] = FanoutResult{NodeName: snd.NodeName, Err: fmt.Errorf("unknown node %q", snd.NodeName)}
				return
			}

			out, err := fn(ctx, snd.State)
			results[idx] = FanoutResult{NodeName: snd.NodeName, State: out, Err: err}
		}(i, s)
	}

	wg.Wait()
	return results, nil
}

// MapReduce is a high-level helper for the split→map→reduce pattern.
// splitFn is implicit: items is the pre-split input list.
// mapFn processes each item concurrently; reduceFn aggregates all outputs.
// maxConcurrency <= 0 means unbounded.
func MapReduce[I, O, R any](
	ctx context.Context,
	items []I,
	mapFn func(ctx context.Context, item I) (O, error),
	reduceFn func(results []O) (R, error),
	maxConcurrency int,
) (R, error) {
	var zero R
	if len(items) == 0 {
		return reduceFn(nil)
	}

	if maxConcurrency <= 0 {
		maxConcurrency = len(items)
	}

	sem := make(chan struct{}, maxConcurrency)
	outputs := make([]O, len(items))
	errs := make([]error, len(items))
	var wg sync.WaitGroup

	for i, item := range items {
		wg.Add(1)
		go func(idx int, it I) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			out, err := mapFn(ctx, it)
			outputs[idx] = out
			errs[idx] = err
		}(i, item)
	}

	wg.Wait()

	// Collect successful outputs; surface first error if all failed.
	var successOutputs []O
	var firstErr error
	for i, out := range outputs {
		if errs[i] != nil {
			if firstErr == nil {
				firstErr = errs[i]
			}
		} else {
			successOutputs = append(successOutputs, out)
		}
	}

	if len(successOutputs) == 0 && firstErr != nil {
		return zero, firstErr
	}

	return reduceFn(successOutputs)
}
