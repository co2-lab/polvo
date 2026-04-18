package agent

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// FailureStrategy controls how SupervisorAgent handles agent failures.
type FailureStrategy int

const (
	// AllOrNothing cancels all agents when any agent fails.
	AllOrNothing FailureStrategy = iota
	// BestEffort collects all results; errors are embedded in AgentResult.
	BestEffort
	// FirstSuccess returns the first successful result and cancels the rest.
	FirstSuccess
)

// SupervisorAgent coordinates multiple agent tasks, applies failure strategies,
// and optionally publishes results to an AgentBus.
type SupervisorAgent struct {
	bus      *AgentBus
	exec     *Executor
	strategy FailureStrategy

	mu      sync.Mutex
	tasks   map[string]AgentTask
	results map[string]AgentResult
	done    chan struct{}

	cancel  context.CancelFunc
	taskSeq atomic.Int64
	wg      sync.WaitGroup
	resCh   chan AgentResult
}

// NewSupervisorAgent creates a SupervisorAgent.
// exec must not be nil. bus may be nil (no bus publishing).
func NewSupervisorAgent(exec *Executor, bus *AgentBus, strategy FailureStrategy) *SupervisorAgent {
	return &SupervisorAgent{
		bus:      bus,
		exec:     exec,
		strategy: strategy,
		tasks:    make(map[string]AgentTask),
		results:  make(map[string]AgentResult),
		done:     make(chan struct{}),
		resCh:    make(chan AgentResult, 64),
	}
}

// Assign launches an agent task asynchronously. Returns a task ID.
// The parent context is used; Assign creates a child context for cancellation.
func (s *SupervisorAgent) Assign(ctx context.Context, task AgentTask) string {
	id := fmt.Sprintf("task-%d", s.taskSeq.Add(1))

	s.mu.Lock()
	s.tasks[id] = task
	// If this is the first task, capture cancel for Cancel().
	if s.cancel == nil {
		ctx, s.cancel = context.WithCancel(ctx)
	}
	taskCtx := ctx
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				s.resCh <- AgentResult{
					AgentName: task.AgentName,
					Err:       fmt.Errorf("agent panicked: %v", r),
				}
			}
		}()

		a, err := s.exec.GetAgent(task.AgentName, nil)
		var res AgentResult
		if err != nil {
			res = AgentResult{AgentName: task.AgentName, Err: fmt.Errorf("getting agent: %w", err)}
		} else {
			r, execErr := a.Execute(taskCtx, task.Input)
			res = AgentResult{AgentName: task.AgentName, Result: r, Err: execErr}
		}

		s.resCh <- res

		if s.bus != nil {
			s.bus.Publish(AgentMessage{
				From:    task.AgentName,
				To:      "supervisor",
				Type:    MessageResult,
				Payload: res.AgentName,
			})
		}
	}()
	return id
}

// WaitAll waits for all assigned tasks to complete or timeout to expire.
// Applies AllOrNothing and BestEffort failure strategies.
func (s *SupervisorAgent) WaitAll(timeout time.Duration) ([]AgentResult, error) {
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	var results []AgentResult
	var firstErr error

	for {
		select {
		case <-ctx.Done():
			return results, fmt.Errorf("supervisor WaitAll timeout: %w", ctx.Err())
		case <-done:
			// Drain remaining results.
			for {
				select {
				case r := <-s.resCh:
					results = append(results, r)
					s.mu.Lock()
					s.results[r.AgentName] = r
					s.mu.Unlock()
				default:
					goto drained
				}
			}
		drained:
			if s.strategy == AllOrNothing && firstErr != nil {
				return results, firstErr
			}
			return results, nil
		case r := <-s.resCh:
			results = append(results, r)
			s.mu.Lock()
			s.results[r.AgentName] = r
			s.mu.Unlock()
			if r.Err != nil {
				if s.strategy == AllOrNothing {
					firstErr = r.Err
					s.Cancel()
				}
			}
		}
	}
}

// WaitAny returns the first result available. Suitable for FirstSuccess strategy.
func (s *SupervisorAgent) WaitAny(timeout time.Duration) (AgentResult, error) {
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	select {
	case <-ctx.Done():
		return AgentResult{}, fmt.Errorf("supervisor WaitAny timeout: %w", ctx.Err())
	case r := <-s.resCh:
		if s.strategy == FirstSuccess && r.Err == nil {
			s.Cancel()
		}
		return r, nil
	}
}

// Cancel cancels all running tasks.
func (s *SupervisorAgent) Cancel() {
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}
