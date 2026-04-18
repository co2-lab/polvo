package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// AgentTask is a named agent execution to run in parallel.
type AgentTask struct {
	AgentName string
	Input     *Input
}

// AgentResult holds the outcome of one parallel agent task.
type AgentResult struct {
	AgentName string
	Result    *Result
	Err       error
}

// RunParallel executes multiple agents concurrently and returns all results.
// maxParallel limits concurrency (0 = unlimited).
func RunParallel(ctx context.Context, exec *Executor, tasks []AgentTask, maxParallel int) []AgentResult {
	if maxParallel <= 0 {
		maxParallel = len(tasks)
	}

	sem := make(chan struct{}, maxParallel)
	results := make([]AgentResult, len(tasks))
	var wg sync.WaitGroup

	for i, task := range tasks {
		wg.Add(1)
		go func(idx int, t AgentTask) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			a, err := exec.GetAgent(t.AgentName, nil)
			if err != nil {
				results[idx] = AgentResult{AgentName: t.AgentName, Err: fmt.Errorf("getting agent: %w", err)}
				return
			}

			res, err := a.Execute(ctx, t.Input)
			results[idx] = AgentResult{AgentName: t.AgentName, Result: res, Err: err}
		}(i, task)
	}

	wg.Wait()
	return results
}

// AggregateResults merges parallel agent results into a single Result.
// Errors are collected into the summary; findings are merged via StateGraph
// (thread-safe append reducer).
func AggregateResults(results []AgentResult) *Result {
	var sb strings.Builder
	sg := NewStateGraph()
	anyDecision := ""

	for _, r := range results {
		if r.Err != nil {
			fmt.Fprintf(&sb, "[%s] error: %v\n", r.AgentName, r.Err)
			continue
		}
		if r.Result == nil {
			continue
		}
		if r.Result.Summary != "" {
			fmt.Fprintf(&sb, "[%s] %s\n", r.AgentName, r.Result.Summary)
		}
		if len(r.Result.Findings) > 0 {
			sg.Findings.Update(r.Result.Findings)
		}
		if r.Result.Decision != "" {
			anyDecision = r.Result.Decision
		}
	}

	return &Result{
		Decision: anyDecision,
		Summary:  strings.TrimRight(sb.String(), "\n"),
		Findings: sg.Findings.Get(),
	}
}
