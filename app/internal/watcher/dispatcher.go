package watcher

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/co2-lab/polvo/internal/config"
)

// AgentRunner is the interface the dispatcher uses to run agents.
// Implemented by *agent.Executor via RunAgentForFile.
type AgentRunner interface {
	RunAgentForFile(ctx context.Context, agentName, filePath string) error
}

// Dispatcher consumes WatchEvents from a channel, batches them via an
// EventAccumulator, and runs the registered agents for each event in
// the batch in parallel, bounded by MaxParallel.
type Dispatcher struct {
	cfg    *config.Config
	runner AgentRunner
	ch     <-chan WatchEvent
	logger *slog.Logger
	sem    chan struct{} // semaphore for MaxParallel
}

// NewDispatcher creates a Dispatcher.
//
// ch is the shared channel all Watchers publish to.
// maxParallel limits total concurrent agent goroutines (0 = unlimited).
func NewDispatcher(cfg *config.Config, runner AgentRunner, ch <-chan WatchEvent, logger *slog.Logger) *Dispatcher {
	maxParallel := cfg.Settings.MaxParallel
	var sem chan struct{}
	if maxParallel > 0 {
		sem = make(chan struct{}, maxParallel)
	}
	return &Dispatcher{
		cfg:    cfg,
		runner: runner,
		ch:     ch,
		logger: logger,
		sem:    sem,
	}
}

// UpdateConfig swaps in a new config after a reload.
func (d *Dispatcher) UpdateConfig(cfg *config.Config) {
	d.cfg = cfg
	maxParallel := cfg.Settings.MaxParallel
	if maxParallel > 0 {
		d.sem = make(chan struct{}, maxParallel)
	} else {
		d.sem = nil
	}
}

// Run starts the dispatcher loop. Blocks until ctx is cancelled.
// Events are fed through an EventAccumulator (150 ms window, max 50 per batch)
// before being dispatched so that rapid bursts are coalesced.
func (d *Dispatcher) Run(ctx context.Context) {
	acc := NewEventAccumulator(150*time.Millisecond, 50)
	defer acc.Close()

	// Feed raw watcher events into the accumulator.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-d.ch:
				if !ok {
					return
				}
				acc.Add(event)
			}
		}
	}()

	// Consume batches from the accumulator.
	for {
		select {
		case <-ctx.Done():
			return
		case batch, ok := <-acc.Events():
			if !ok {
				return
			}
			d.dispatchBatch(ctx, batch)
		}
	}
}

// dispatchBatch runs all agents for every event in the batch.
// All files in a batch share a single semaphore slot: the batch is treated
// as one unit of work (we acquire/release once, then call RunAgentForFile
// sequentially per file per agent).
func (d *Dispatcher) dispatchBatch(ctx context.Context, batch []WatchEvent) {
	// Group events by watcher name so we can look up agent lists once.
	byWatcher := make(map[string][]WatchEvent)
	for _, ev := range batch {
		byWatcher[ev.WatcherName] = append(byWatcher[ev.WatcherName], ev)
	}

	var wg sync.WaitGroup
	for watcherName, events := range byWatcher {
		watcherCfg, ok := d.cfg.Watchers[watcherName]
		if !ok {
			d.logger.Warn("no watcher config for event", "watcher", watcherName)
			continue
		}

		for _, agentName := range watcherCfg.Agents {
			wg.Add(1)
			go func(name string, evs []WatchEvent) {
				defer wg.Done()
				d.acquireSem()
				defer d.releaseSem()

				for _, ev := range evs {
					d.logger.Info("dispatching agent", "watcher", watcherName, "agent", name, "path", ev.Path)
					if err := d.runner.RunAgentForFile(ctx, name, ev.Path); err != nil {
						d.logger.Error("agent error", "watcher", watcherName, "agent", name, "error", err)
					}
				}
			}(agentName, events)
		}
	}
	wg.Wait()
}

func (d *Dispatcher) acquireSem() {
	if d.sem != nil {
		d.sem <- struct{}{}
	}
}

func (d *Dispatcher) releaseSem() {
	if d.sem != nil {
		<-d.sem
	}
}
