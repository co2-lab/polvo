package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/co2-lab/polvo/internal/provider"
	"github.com/co2-lab/polvo/internal/tool"
)

// ExploreTask is a single exploration subtask for a read-only subagent.
type ExploreTask struct {
	Description string
	Focus       []string // file paths/patterns to focus on
	TokenBudget int      // 0 = use default
}

// ExploreFinding is a structured discovery from a read-only exploration.
// Named ExploreFinding to avoid conflict with the existing Finding type in agent.go.
type ExploreFinding struct {
	Type    string // "definition", "usage", "pattern", "issue"
	File    string
	Line    int
	Content string
	Note    string
}

// ExploreResult is the aggregated output from a read-only subagent.
type ExploreResult struct {
	TaskDescription string
	Findings        []ExploreFinding
	FilesRead       []string
	Summary         string
	TokensUsed      int
	TurnsUsed       int
	Err             error
}

const defaultExploreTokenBudget = 8000
const defaultExploreMaxTurns = 15
const defaultExploreMaxParallel = 5

// ExploreParallel launches N read-only subagents in parallel.
// Each runs with ReadOnlyToolset only (2-layer enforcement).
// globalTokenBudget: sum of all individual budgets must not exceed this.
// maxParallel: max concurrent goroutines (default 5).
// Returns results in the same order as input tasks.
func ExploreParallel(
	ctx context.Context,
	tasks []ExploreTask,
	exec *Executor,
	model string,
	globalTokenBudget int,
	maxParallel int,
) ([]ExploreResult, error) {
	if len(tasks) == 0 {
		return nil, nil
	}

	// Prevent subagents from calling ExploreParallel (no recursion)
	if DelegateLevelFromCtx(ctx) >= 1 {
		return nil, fmt.Errorf("explore not available in subagents: delegate level is %d", DelegateLevelFromCtx(ctx))
	}

	if maxParallel <= 0 {
		maxParallel = defaultExploreMaxParallel
	}

	// Distribute token budget across tasks
	perTaskBudget := defaultExploreTokenBudget
	if globalTokenBudget > 0 && len(tasks) > 0 {
		perTaskBudget = globalTokenBudget / len(tasks)
		if perTaskBudget <= 0 {
			perTaskBudget = defaultExploreTokenBudget
		}
	}

	results := make([]ExploreResult, len(tasks))
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup

	for i, task := range tasks {
		wg.Add(1)
		go func(idx int, t ExploreTask) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			budget := t.TokenBudget
			if budget <= 0 {
				budget = perTaskBudget
			}

			result := runExploreTask(ctx, t, exec, model, budget)
			results[idx] = result
		}(i, task)
	}

	wg.Wait()
	return results, nil
}

// runExploreTask runs a single read-only explore task using a Loop directly.
func runExploreTask(ctx context.Context, task ExploreTask, exec *Executor, model string, tokenBudget int) ExploreResult {
	result := ExploreResult{
		TaskDescription: task.Description,
	}

	// Build a subagent context with delegate level 1
	subCtx := WithDelegateLevel(ctx, 1)

	// Build a read-only tool registry from the executor's config
	// We need a provider to run the loop — get one from the executor if possible
	var chatProvider provider.ChatProvider
	if exec != nil && exec.registry != nil {
		p, err := exec.registry.Default()
		if err == nil {
			if cp, ok := p.(provider.ChatProvider); ok {
				chatProvider = cp
			}
		}
	}

	if chatProvider == nil {
		result.Err = fmt.Errorf("no chat provider available for explore task")
		return result
	}

	// Build a read-only registry with only the allowed tools
	readOnlyReg := buildReadOnlyRegistry(exec)

	// Build the task prompt
	var sb strings.Builder
	sb.WriteString(task.Description)
	if len(task.Focus) > 0 {
		sb.WriteString("\n\nFocus on the following paths/patterns:\n")
		for _, f := range task.Focus {
			sb.WriteString("- ")
			sb.WriteString(f)
			sb.WriteString("\n")
		}
	}

	// Track tool calls to extract files read
	var filesRead []string
	var mu sync.Mutex

	maxTokens := tokenBudget
	if maxTokens <= 0 {
		maxTokens = defaultExploreTokenBudget
	}

	useModel := model
	if useModel == "" && exec != nil && exec.cfg != nil {
		// Try to get a summary model from config
	}

	loop := NewLoop(LoopConfig{
		Provider:  chatProvider,
		Tools:     readOnlyReg,
		System:    "You are a read-only code exploration subagent. Your task is to explore the codebase and report findings. You CANNOT write, edit, or execute any files. Only use read, glob, grep, ls, think, and memory_read tools.",
		Model:     useModel,
		MaxTurns:  defaultExploreMaxTurns,
		MaxTokens: maxTokens,
		OnToolCall: func(tc provider.ToolCall) {
			mu.Lock()
			defer mu.Unlock()
			// Track files read from read, glob, and grep tool calls
			if tc.Name == "read" || tc.Name == "glob" || tc.Name == "grep" {
				filesRead = append(filesRead, string(tc.Input))
			}
		},
	})

	loopResult, err := loop.Run(subCtx, sb.String())
	if err != nil {
		result.Err = err
		return result
	}

	mu.Lock()
	result.FilesRead = filesRead
	mu.Unlock()

	result.Summary = loopResult.FinalText
	result.TurnsUsed = loopResult.TurnCount
	result.TokensUsed = loopResult.TokensUsed.TotalTokens

	return result
}

// buildReadOnlyRegistry constructs a tool.Registry containing only read-only tools.
// It uses executor's existing tools if available, otherwise creates minimal registry.
func buildReadOnlyRegistry(exec *Executor) *tool.Registry {
	readOnlyReg := tool.NewRegistry()

	if exec == nil {
		return readOnlyReg
	}

	// Try to get tools from an existing agent in the executor cache
	// We iterate the agents map to find a ToolLLMAgent with a registry
	for _, a := range exec.agents {
		if tla, ok := a.(*ToolLLMAgent); ok {
			// Filter to only read-only tools
			allowedSet := make(map[string]bool)
			for _, name := range ReadOnlyToolset {
				allowedSet[name] = true
			}
			for _, t := range tla.tools.All() {
				if allowedSet[t.Name()] {
					readOnlyReg.Register(t)
				}
			}
			return readOnlyReg
		}
	}

	return readOnlyReg
}
