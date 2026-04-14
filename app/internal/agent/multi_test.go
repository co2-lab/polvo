package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// ---- mockAgent --------------------------------------------------------------

type mockAgent struct {
	name   string
	result *Result
	err    error
	delay  time.Duration
}

func (m *mockAgent) Name() string { return m.name }
func (m *mockAgent) Role() Role   { return RoleAuthor }
func (m *mockAgent) Execute(ctx context.Context, _ *Input) (*Result, error) {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return m.result, m.err
}

// ---- newTestExecutor --------------------------------------------------------

func newTestExecutor(agents map[string]Agent) *Executor {
	return &Executor{agents: agents}
}

// ---- TestRunParallel --------------------------------------------------------

func TestRunParallel(t *testing.T) {
	t.Run("lista vazia retorna slice vazio", func(t *testing.T) {
		exec := newTestExecutor(nil)
		results := RunParallel(context.Background(), exec, nil, 0)
		if len(results) != 0 {
			t.Errorf("expected empty results, got %d", len(results))
		}
	})

	t.Run("um agente com sucesso", func(t *testing.T) {
		agents := map[string]Agent{
			"agent-0": &mockAgent{
				name:   "agent-0",
				result: &Result{Summary: "done"},
			},
		}
		exec := newTestExecutor(agents)
		tasks := []AgentTask{{AgentName: "agent-0", Input: &Input{}}}
		results := RunParallel(context.Background(), exec, tasks, 0)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].Err != nil {
			t.Errorf("unexpected error: %v", results[0].Err)
		}
		if results[0].Result.Summary != "done" {
			t.Errorf("expected Summary='done', got %q", results[0].Result.Summary)
		}
	})

	t.Run("múltiplos agentes todos executam", func(t *testing.T) {
		agents := map[string]Agent{}
		var tasks []AgentTask
		for i := 0; i < 5; i++ {
			name := fmt.Sprintf("agent-%d", i)
			agents[name] = &mockAgent{name: name, result: &Result{Summary: name}}
			tasks = append(tasks, AgentTask{AgentName: name, Input: &Input{}})
		}
		exec := newTestExecutor(agents)
		results := RunParallel(context.Background(), exec, tasks, 0)
		if len(results) != 5 {
			t.Fatalf("expected 5 results, got %d", len(results))
		}
		for i, r := range results {
			if r.Err != nil {
				t.Errorf("result[%d] unexpected error: %v", i, r.Err)
			}
		}
	})

	t.Run("maxParallel=1 executa sem deadlock", func(t *testing.T) {
		agents := map[string]Agent{}
		var tasks []AgentTask
		for i := 0; i < 3; i++ {
			name := fmt.Sprintf("agent-%d", i)
			agents[name] = &mockAgent{name: name, result: &Result{Summary: name}}
			tasks = append(tasks, AgentTask{AgentName: name, Input: &Input{}})
		}
		exec := newTestExecutor(agents)
		results := RunParallel(context.Background(), exec, tasks, 1)
		if len(results) != 3 {
			t.Fatalf("expected 3 results, got %d", len(results))
		}
	})

	t.Run("maxParallel=0 usa len(tasks) como limite", func(t *testing.T) {
		agents := map[string]Agent{}
		var tasks []AgentTask
		for i := 0; i < 4; i++ {
			name := fmt.Sprintf("agent-%d", i)
			agents[name] = &mockAgent{name: name, result: &Result{Summary: name}}
			tasks = append(tasks, AgentTask{AgentName: name, Input: &Input{}})
		}
		exec := newTestExecutor(agents)
		results := RunParallel(context.Background(), exec, tasks, 0)
		if len(results) != 4 {
			t.Fatalf("expected 4 results, got %d", len(results))
		}
	})

	t.Run("agente retorna erro → results[i].Err != nil no índice correto", func(t *testing.T) {
		agents := map[string]Agent{
			"ok-agent":  &mockAgent{name: "ok-agent", result: &Result{Summary: "ok"}},
			"bad-agent": &mockAgent{name: "bad-agent", err: fmt.Errorf("something failed")},
		}
		exec := newTestExecutor(agents)
		tasks := []AgentTask{
			{AgentName: "ok-agent", Input: &Input{}},
			{AgentName: "bad-agent", Input: &Input{}},
		}
		results := RunParallel(context.Background(), exec, tasks, 0)
		if results[0].Err != nil {
			t.Errorf("results[0] should have no error, got: %v", results[0].Err)
		}
		if results[1].Err == nil {
			t.Error("results[1] should have error")
		}
	})
}

func TestRunParallelIndexStability(t *testing.T) {
	// 20 agentes, cada um retornando Summary: "agent-N" com delays variados
	// Agentes de índice alto terminam mais rápido (menor delay)
	agents := map[string]Agent{}
	var tasks []AgentTask
	for i := 0; i < 20; i++ {
		name := fmt.Sprintf("agent-%d", i)
		// Higher index = shorter delay (finishes first)
		delay := time.Duration(20-i) * time.Millisecond
		agents[name] = &mockAgent{
			name:   name,
			result: &Result{Summary: fmt.Sprintf("agent-%d", i)},
			delay:  delay,
		}
		tasks = append(tasks, AgentTask{AgentName: name, Input: &Input{}})
	}
	exec := newTestExecutor(agents)
	results := RunParallel(context.Background(), exec, tasks, 0)

	if len(results) != 20 {
		t.Fatalf("expected 20 results, got %d", len(results))
	}
	for i, r := range results {
		expected := fmt.Sprintf("agent-%d", i)
		if r.Err != nil {
			t.Errorf("results[%d] unexpected error: %v", i, r.Err)
			continue
		}
		if r.Result.Summary != expected {
			t.Errorf("results[%d].Summary = %q; want %q", i, r.Result.Summary, expected)
		}
	}
}

func TestRunParallelContextCancel(t *testing.T) {
	// Agente com delay 50ms + ctx cancelado após 10ms
	agents := map[string]Agent{
		"slow-agent": &mockAgent{
			name:   "slow-agent",
			result: &Result{Summary: "never"},
			delay:  50 * time.Millisecond,
		},
	}
	exec := newTestExecutor(agents)
	tasks := []AgentTask{{AgentName: "slow-agent", Input: &Input{}}}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	results := RunParallel(ctx, exec, tasks, 0)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err == nil {
		t.Error("expected context cancellation error")
	}
	if results[0].Err != context.Canceled {
		t.Errorf("expected context.Canceled, got: %v", results[0].Err)
	}

	// GAP: não implementado — subagentes em RunParallel não têm autonomy mode = plan
	// O draft (§ "Subagentes Read-only") especifica que subagentes de exploração são
	// read-only, mas RunParallel não propaga nem aplica FilterRegistryForMode.
	// Quando isso for implementado, adicionar TestRunParallel_PlanModeEnforced.
}

// ---- TestAggregateResults ---------------------------------------------------

func TestAggregateResults(t *testing.T) {
	t.Run("todos com sucesso → Summary concatenado", func(t *testing.T) {
		results := []AgentResult{
			{AgentName: "agent-a", Result: &Result{Summary: "found issues"}},
			{AgentName: "agent-b", Result: &Result{Summary: "looks good"}},
		}
		agg := AggregateResults(results)
		if !strings.Contains(agg.Summary, "[agent-a]") {
			t.Error("expected [agent-a] in summary")
		}
		if !strings.Contains(agg.Summary, "[agent-b]") {
			t.Error("expected [agent-b] in summary")
		}
		if !strings.Contains(agg.Summary, "found issues") {
			t.Error("expected 'found issues' in summary")
		}
		if !strings.Contains(agg.Summary, "looks good") {
			t.Error("expected 'looks good' in summary")
		}
	})

	t.Run("um com erro → erro incluído no Summary", func(t *testing.T) {
		results := []AgentResult{
			{AgentName: "agent-ok", Result: &Result{Summary: "success"}},
			{AgentName: "agent-bad", Err: fmt.Errorf("timeout")},
		}
		agg := AggregateResults(results)
		if !strings.Contains(agg.Summary, "[agent-bad] error:") {
			t.Error("expected error in summary")
		}
		if !strings.Contains(agg.Summary, "timeout") {
			t.Error("expected 'timeout' in summary")
		}
	})

	t.Run("findings concatenados em ordem do slice", func(t *testing.T) {
		f1 := Finding{File: "a.go", Message: "issue-a"}
		f2 := Finding{File: "b.go", Message: "issue-b"}
		results := []AgentResult{
			{AgentName: "agent-a", Result: &Result{Findings: []Finding{f1}}},
			{AgentName: "agent-b", Result: &Result{Findings: []Finding{f2}}},
		}
		agg := AggregateResults(results)
		if len(agg.Findings) != 2 {
			t.Fatalf("expected 2 findings, got %d", len(agg.Findings))
		}
		if agg.Findings[0].File != "a.go" {
			t.Errorf("expected first finding from a.go, got %q", agg.Findings[0].File)
		}
		if agg.Findings[1].File != "b.go" {
			t.Errorf("expected second finding from b.go, got %q", agg.Findings[1].File)
		}
	})

	t.Run("Decision: último non-empty vence", func(t *testing.T) {
		results := []AgentResult{
			{AgentName: "agent-a", Result: &Result{Decision: "REJECT"}},
			{AgentName: "agent-b", Result: &Result{Decision: "APPROVE"}},
			{AgentName: "agent-c", Result: &Result{Decision: ""}}, // empty — doesn't override
		}
		agg := AggregateResults(results)
		if agg.Decision != "APPROVE" {
			t.Errorf("expected Decision='APPROVE', got %q", agg.Decision)
		}
	})

	t.Run("todos sem Summary → Summary vazio", func(t *testing.T) {
		results := []AgentResult{
			{AgentName: "agent-a", Result: &Result{}},
			{AgentName: "agent-b", Result: &Result{}},
		}
		agg := AggregateResults(results)
		if agg.Summary != "" {
			t.Errorf("expected empty summary, got %q", agg.Summary)
		}
	})

	t.Run("Findings de subagentes propagados", func(t *testing.T) {
		f := Finding{File: "x.go", Message: "critical issue", Severity: "error"}
		results := []AgentResult{
			{AgentName: "agent-a", Result: &Result{Findings: []Finding{f}}},
		}
		agg := AggregateResults(results)
		if len(agg.Findings) == 0 {
			t.Fatal("expected findings to be propagated")
		}
		if agg.Findings[0].Message != "critical issue" {
			t.Errorf("expected finding message, got %q", agg.Findings[0].Message)
		}
	})
}
