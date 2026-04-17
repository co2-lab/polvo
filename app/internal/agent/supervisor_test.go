package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func makeExec(agents map[string]Agent) *Executor {
	return newTestExecutor(agents)
}

func TestSupervisor_AllSucceed_WaitAll(t *testing.T) {
	agents := map[string]Agent{
		"a": &mockAgent{name: "a", result: &Result{Summary: "ok-a"}},
		"b": &mockAgent{name: "b", result: &Result{Summary: "ok-b"}},
		"c": &mockAgent{name: "c", result: &Result{Summary: "ok-c"}},
	}
	sv := NewSupervisorAgent(makeExec(agents), nil, BestEffort)
	ctx := context.Background()
	for _, name := range []string{"a", "b", "c"} {
		sv.Assign(ctx, AgentTask{AgentName: name, Input: &Input{}})
	}
	results, err := sv.WaitAll(5 * time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Err != nil {
			t.Errorf("agent %s had error: %v", r.AgentName, r.Err)
		}
	}
}

func TestSupervisor_AllOrNothing_OnFirstError(t *testing.T) {
	agents := map[string]Agent{
		"ok":  &mockAgent{name: "ok", result: &Result{Summary: "fine"}, delay: 200 * time.Millisecond},
		"bad": &mockAgent{name: "bad", err: errors.New("boom")},
	}
	sv := NewSupervisorAgent(makeExec(agents), nil, AllOrNothing)
	ctx := context.Background()
	sv.Assign(ctx, AgentTask{AgentName: "ok", Input: &Input{}})
	sv.Assign(ctx, AgentTask{AgentName: "bad", Input: &Input{}})

	_, err := sv.WaitAll(5 * time.Second)
	if err == nil {
		t.Error("expected error from AllOrNothing when agent fails")
	}
}

func TestSupervisor_BestEffort_AllReturn(t *testing.T) {
	agents := map[string]Agent{
		"ok1": &mockAgent{name: "ok1", result: &Result{}},
		"ok2": &mockAgent{name: "ok2", result: &Result{}},
		"bad": &mockAgent{name: "bad", err: errors.New("err")},
	}
	sv := NewSupervisorAgent(makeExec(agents), nil, BestEffort)
	ctx := context.Background()
	for _, n := range []string{"ok1", "ok2", "bad"} {
		sv.Assign(ctx, AgentTask{AgentName: n, Input: &Input{}})
	}
	results, err := sv.WaitAll(5 * time.Second)
	if err != nil {
		t.Fatalf("BestEffort should not return top-level error: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
	errCount := 0
	for _, r := range results {
		if r.Err != nil {
			errCount++
		}
	}
	if errCount != 1 {
		t.Errorf("expected 1 result with Err, got %d", errCount)
	}
}

func TestSupervisor_WaitAny_ReturnsFirst(t *testing.T) {
	agents := map[string]Agent{
		"fast": &mockAgent{name: "fast", result: &Result{Summary: "fast"}},
		"slow": &mockAgent{name: "slow", result: &Result{Summary: "slow"}, delay: 5 * time.Second},
	}
	sv := NewSupervisorAgent(makeExec(agents), nil, BestEffort)
	ctx := context.Background()
	sv.Assign(ctx, AgentTask{AgentName: "fast", Input: &Input{}})
	sv.Assign(ctx, AgentTask{AgentName: "slow", Input: &Input{}})

	r, err := sv.WaitAny(2 * time.Second)
	if err != nil {
		t.Fatalf("WaitAny error: %v", err)
	}
	if r.AgentName != "fast" {
		t.Errorf("expected fast, got %s", r.AgentName)
	}
}

func TestSupervisor_Timeout_WaitAll(t *testing.T) {
	agents := map[string]Agent{
		"slow": &mockAgent{name: "slow", result: &Result{}, delay: 5 * time.Second},
	}
	sv := NewSupervisorAgent(makeExec(agents), nil, BestEffort)
	sv.Assign(context.Background(), AgentTask{AgentName: "slow", Input: &Input{}})

	_, err := sv.WaitAll(50 * time.Millisecond)
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestSupervisor_ResultPublishedOnBus(t *testing.T) {
	bus := NewAgentBus()
	defer bus.Close()

	agents := map[string]Agent{
		"worker": &mockAgent{name: "worker", result: &Result{Summary: "done"}},
	}
	sv := NewSupervisorAgent(makeExec(agents), bus, BestEffort)
	superCh := bus.Subscribe("supervisor")

	sv.Assign(context.Background(), AgentTask{AgentName: "worker", Input: &Input{}})
	sv.WaitAll(5 * time.Second) //nolint:errcheck

	select {
	case msg := <-superCh:
		if msg.Type != MessageResult {
			t.Errorf("expected MessageResult, got %v", msg.Type)
		}
	case <-time.After(time.Second):
		t.Error("supervisor did not receive result message on bus")
	}
}

func TestSupervisor_Race(t *testing.T) {
	agents := map[string]Agent{
		"r1": &mockAgent{name: "r1", result: &Result{}},
		"r2": &mockAgent{name: "r2", result: &Result{}},
		"r3": &mockAgent{name: "r3", result: &Result{}},
	}
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sv := NewSupervisorAgent(makeExec(agents), nil, BestEffort)
			ctx := context.Background()
			for _, n := range []string{"r1", "r2", "r3"} {
				sv.Assign(ctx, AgentTask{AgentName: n, Input: &Input{}})
			}
			sv.WaitAll(5 * time.Second) //nolint:errcheck
		}()
	}
	wg.Wait()
}
