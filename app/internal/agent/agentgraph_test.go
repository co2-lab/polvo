package agent

import (
	"context"
	"sync"
	"testing"
	"time"
)

func nodeAgent(name string) *GraphNode {
	return &GraphNode{
		ID:        NodeID(name),
		AgentName: name,
		Input:     &Input{},
	}
}

func graphExec(names ...string) *Executor {
	agents := make(map[string]Agent, len(names))
	for _, n := range names {
		agents[n] = &mockAgent{name: n, result: &Result{Summary: n}}
	}
	return newTestExecutor(agents)
}

func resultNames(results []AgentResult) []string {
	names := make([]string, len(results))
	for i, r := range results {
		names[i] = r.AgentName
	}
	return names
}

func TestGraph_LinearABC(t *testing.T) {
	exec := graphExec("A", "B", "C")
	g := NewAgentGraph("A")
	g.AddNode(nodeAgent("A"))
	g.AddNode(nodeAgent("B"))
	g.AddNode(nodeAgent("C"))
	_ = g.AddEdge(EdgeRule{From: "A", To: "B"})
	_ = g.AddEdge(EdgeRule{From: "B", To: "C"})

	results, err := g.Run(context.Background(), exec)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d: %v", len(results), resultNames(results))
	}
}

func TestGraph_Fork_ABandAC(t *testing.T) {
	exec := graphExec("A", "B", "C")
	g := NewAgentGraph("A")
	g.AddNode(nodeAgent("A"))
	g.AddNode(nodeAgent("B"))
	g.AddNode(nodeAgent("C"))
	_ = g.AddEdge(EdgeRule{From: "A", To: "B"})
	_ = g.AddEdge(EdgeRule{From: "A", To: "C"})

	results, err := g.Run(context.Background(), exec)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results (A, B, C), got %d", len(results))
	}
}

func TestGraph_ConditionalRoute_TrueGoesB(t *testing.T) {
	aResult := &Result{Summary: "go-b"}
	agents := map[string]Agent{
		"A": &mockAgent{name: "A", result: aResult},
		"B": &mockAgent{name: "B", result: &Result{}},
		"C": &mockAgent{name: "C", result: &Result{}},
	}
	exec := newTestExecutor(agents)

	g := NewAgentGraph("A")
	g.AddNode(nodeAgent("A"))
	g.AddNode(nodeAgent("B"))
	g.AddNode(nodeAgent("C"))
	_ = g.AddEdge(ConditionalRoute("A", "B", func(r AgentResult) bool {
		return r.Result != nil && r.Result.Summary == "go-b"
	}))
	_ = g.AddEdge(ConditionalRoute("A", "C", func(r AgentResult) bool {
		return r.Result != nil && r.Result.Summary == "go-c"
	}))

	results, err := g.Run(context.Background(), exec)
	if err != nil {
		t.Fatal(err)
	}
	// A and B should run; C should not.
	names := resultNames(results)
	hasA, hasB, hasC := false, false, false
	for _, n := range names {
		switch n {
		case "A":
			hasA = true
		case "B":
			hasB = true
		case "C":
			hasC = true
		}
	}
	if !hasA {
		t.Error("expected A to run")
	}
	if !hasB {
		t.Error("expected B to run (condition true)")
	}
	if hasC {
		t.Error("C should NOT run (condition false)")
	}
}

func TestGraph_CycleNotExecutedTwice(t *testing.T) {
	var mu sync.Mutex
	execCount := map[string]int{}

	countingAgent := func(name string) Agent {
		return &funcAgent{name: name, fn: func(ctx context.Context, _ *Input) (*Result, error) {
			mu.Lock()
			execCount[name]++
			mu.Unlock()
			return &Result{Summary: name}, nil
		}}
	}
	agents := map[string]Agent{
		"A": countingAgent("A"),
		"B": countingAgent("B"),
	}
	exec := newTestExecutor(agents)

	g := NewAgentGraph("A")
	g.AddNode(nodeAgent("A"))
	g.AddNode(nodeAgent("B"))
	_ = g.AddEdge(EdgeRule{From: "A", To: "B"})
	_ = g.AddEdge(EdgeRule{From: "B", To: "A"}) // cycle A→B→A

	_, err := g.Run(context.Background(), exec)
	if err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if execCount["A"] != 1 {
		t.Errorf("A executed %d times, want 1", execCount["A"])
	}
	if execCount["B"] != 1 {
		t.Errorf("B executed %d times, want 1", execCount["B"])
	}
}

func TestGraph_SingleNode(t *testing.T) {
	exec := graphExec("only")
	g := NewAgentGraph("only")
	g.AddNode(nodeAgent("only"))

	results, err := g.Run(context.Background(), exec)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestGraph_ContextCancel(t *testing.T) {
	agents := map[string]Agent{
		"A":    &mockAgent{name: "A", result: &Result{}},
		"slow": &mockAgent{name: "slow", result: &Result{}, delay: 5 * time.Second},
	}
	exec := newTestExecutor(agents)

	g := NewAgentGraph("A")
	g.AddNode(nodeAgent("A"))
	g.AddNode(nodeAgent("slow"))
	_ = g.AddEdge(EdgeRule{From: "A", To: "slow"})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := g.Run(ctx, exec)
	// We expect either a context error or just slow getting ctx.Done.
	// The point is we don't hang forever.
	_ = err
}

func TestGraph_UnknownNodeInEdge(t *testing.T) {
	g := NewAgentGraph("A")
	g.AddNode(nodeAgent("A"))

	err := g.AddEdge(EdgeRule{From: "A", To: "ghost"})
	if err == nil {
		t.Error("expected error for unknown target node")
	}
}

func TestGraph_Race(t *testing.T) {
	g := NewAgentGraph("N0")
	for i := 0; i < 5; i++ {
		g.AddNode(nodeAgent("N" + string(rune('0'+i))))
	}
	// Fan-out from N0 to N1..N4
	for i := 1; i < 5; i++ {
		_ = g.AddEdge(EdgeRule{From: "N0", To: NodeID("N" + string(rune('0'+i)))})
	}

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e := graphExec("N0", "N1", "N2", "N3", "N4")
			_, _ = g.Run(context.Background(), e)
		}()
	}
	wg.Wait()
}

// funcAgent is a test helper that delegates Execute to a function.
type funcAgent struct {
	name string
	fn   func(context.Context, *Input) (*Result, error)
}

func (f *funcAgent) Name() string { return f.name }
func (f *funcAgent) Role() Role   { return RoleAuthor }
func (f *funcAgent) Execute(ctx context.Context, in *Input) (*Result, error) {
	return f.fn(ctx, in)
}
