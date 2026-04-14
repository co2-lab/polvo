package watcher

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/co2-lab/polvo/internal/config"
)

// stubRunner implements AgentRunner for tests.
type stubRunner struct {
	mu    sync.Mutex
	calls []runCall
	err   error
	onRun func() // optional hook called during RunAgentForFile
}

type runCall struct {
	agent string
	file  string
}

func (s *stubRunner) RunAgentForFile(_ context.Context, agentName, filePath string) error {
	if s.onRun != nil {
		s.onRun()
	}
	s.mu.Lock()
	s.calls = append(s.calls, runCall{agent: agentName, file: filePath})
	s.mu.Unlock()
	return s.err
}

func (s *stubRunner) recorded() []runCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]runCall(nil), s.calls...)
}

// TestDispatcher_MultipleAgentsParallel verifies that all agents associated with
// a watcher are dispatched and that they run in parallel.
func TestDispatcher_MultipleAgentsParallel(t *testing.T) {
	cfg := &config.Config{
		Watchers: map[string]config.WatcherConfig{
			"plans": {Agents: []string{"agent-a", "agent-b", "agent-c"}},
		},
		Settings: config.SettingsConfig{MaxParallel: 0}, // sem limite
	}

	var started atomic.Int32
	barrier := make(chan struct{})

	runner := &stubRunner{
		onRun: func() {
			started.Add(1)
			<-barrier // bloqueia até liberação
		},
	}

	ch := make(chan WatchEvent, 1)
	d := NewDispatcher(cfg, runner, ch, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run dispatcher in background; dispatch blocks until wg.Wait()
	dispatchDone := make(chan struct{})
	go func() {
		defer close(dispatchDone)
		d.Run(ctx)
	}()

	ch <- WatchEvent{WatcherName: "plans", Path: "/path/plan.md", Op: OpModify}

	// Wait until all 3 agents have started (confirms parallelism)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if started.Load() == 3 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if started.Load() != 3 {
		t.Fatalf("expected 3 agents to start in parallel, got %d", started.Load())
	}
	close(barrier) // libera todos os agentes bloqueados

	// Aguarda os agentes completarem (wg.Wait retorna em dispatch)
	// e depois cancela o contexto para d.Run retornar.
	deadline2 := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline2) {
		if len(runner.recorded()) == 3 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case <-dispatchDone:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("dispatcher did not stop after context cancel")
	}

	got := runner.recorded()
	if len(got) != 3 {
		t.Fatalf("expected 3 agent calls, got %d", len(got))
	}

	gotAgents := make([]string, 0, 3)
	for _, c := range got {
		gotAgents = append(gotAgents, c.agent)
		if c.file != "/path/plan.md" {
			t.Errorf("agent %q: unexpected file %q", c.agent, c.file)
		}
	}

	want := map[string]bool{"agent-a": true, "agent-b": true, "agent-c": true}
	for _, a := range gotAgents {
		if !want[a] {
			t.Errorf("unexpected agent called: %q", a)
		}
	}
}

// TestDispatcher_UnknownWatcher verifies that an event with an unknown WatcherName
// does not cause a panic — only a warn log — and no agents are called.
func TestDispatcher_UnknownWatcher(t *testing.T) {
	cfg := &config.Config{
		Watchers: map[string]config.WatcherConfig{
			"plans": {Agents: []string{"agent-a"}},
		},
	}
	runner := &stubRunner{}
	ch := make(chan WatchEvent, 1)
	d := NewDispatcher(cfg, runner, ch, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	ch <- WatchEvent{WatcherName: "nonexistent", Path: "/path/file.go", Op: OpCreate}

	d.Run(ctx) // deve retornar sem panic quando ctx expirar

	if got := runner.recorded(); len(got) != 0 {
		t.Errorf("expected no agent calls for unknown watcher, got %d", len(got))
	}
}

// TestDispatcher_ContextCancel verifies that cancelling the context stops
// the dispatcher loop without goroutine leak.
func TestDispatcher_ContextCancel(t *testing.T) {
	cfg := &config.Config{
		Watchers: map[string]config.WatcherConfig{},
	}
	runner := &stubRunner{}
	ch := make(chan WatchEvent)
	d := NewDispatcher(cfg, runner, ch, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		d.Run(ctx)
		close(done)
	}()

	cancel() // cancela imediatamente

	select {
	case <-done:
		// ok: Run retornou após cancel
	case <-time.After(time.Second):
		t.Fatal("dispatcher did not stop after context cancel")
	}
}

// TestDispatcher_MaxParallel verifies that the semaphore limits concurrency.
func TestDispatcher_MaxParallel(t *testing.T) {
	cfg := &config.Config{
		Watchers: map[string]config.WatcherConfig{
			"plans": {Agents: []string{"a", "b", "c"}},
		},
		Settings: config.SettingsConfig{MaxParallel: 1},
	}

	var concurrent atomic.Int32
	var maxConcurrent atomic.Int32

	runner := &stubRunner{
		onRun: func() {
			c := concurrent.Add(1)
			// Update max concurrency seen
			for {
				old := maxConcurrent.Load()
				if c <= old {
					break
				}
				if maxConcurrent.CompareAndSwap(old, c) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			concurrent.Add(-1)
		},
	}

	ch := make(chan WatchEvent, 1)
	d := NewDispatcher(cfg, runner, ch, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dispatchDone := make(chan struct{})
	go func() {
		defer close(dispatchDone)
		d.Run(ctx)
	}()

	ch <- WatchEvent{WatcherName: "plans", Path: "/path/plan.md", Op: OpModify}

	// Wait for all agents to complete
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(runner.recorded()) == 3 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	cancel()
	<-dispatchDone

	if got := maxConcurrent.Load(); got > 1 {
		t.Errorf("MaxParallel=1 but max concurrent was %d", got)
	}
	if len(runner.recorded()) != 3 {
		t.Errorf("expected 3 agent calls, got %d", len(runner.recorded()))
	}
}
