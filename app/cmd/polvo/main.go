package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/co2-lab/polvo/internal/agent"
	"github.com/co2-lab/polvo/internal/config"
	"github.com/co2-lab/polvo/internal/server"
	"github.com/co2-lab/polvo/internal/guide"
	"github.com/co2-lab/polvo/internal/pipeline"
	"github.com/co2-lab/polvo/internal/provider"
	"github.com/co2-lab/polvo/internal/watcher"
)

// Version is set via ldflags at build time.
var (
	Version   = "dev"
	CommitSHA = "unknown"
	BuildDate = "unknown"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	bus := server.NewBus()

	// POLVO_ROOT allows the Tauri sidecar launcher (or any wrapper) to set the
	// project directory explicitly. Falls back to the process working directory.
	cwd := os.Getenv("POLVO_ROOT")
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			slog.Error("cannot determine working directory", "error", err)
			os.Exit(1)
		}
	} else {
		// Change into the specified root so relative paths work throughout.
		if err := os.Chdir(cwd); err != nil {
			slog.Warn("POLVO_ROOT set but could not chdir", "path", cwd, "error", err)
		}
	}

	cfg, err := config.Load("polvo.yaml")
	if err != nil {
		slog.Warn("failed to load polvo.yaml, running in unconfigured mode", "error", err)
	}

	var registry *provider.Registry
	var resolver *guide.Resolver
	var executor *agent.Executor
	var scheduler *pipeline.Scheduler

	if cfg != nil {
		registry, _ = provider.NewRegistry(cfg.Providers)
		resolver = guide.NewResolver(cwd, cfg)
		executor = agent.NewExecutor(resolver, registry, cfg)
		scheduler = pipeline.NewScheduler(executor, nil, cfg, logger, bus)
	}

	// dashDeps is allocated first; OnConfigReload is assigned after to close over scheduler.
	dashDeps := &server.AppDeps{
		Version:   Version,
		CommitSHA: CommitSHA,
		BuildDate: BuildDate,
		Cfg:       cfg,
		Registry:  registry,
		Resolver:  resolver,
	}

	dashDeps.OnConfigReload = func(newCfg *config.Config) {
		slog.Info("config reloaded — rebuilding executor and scheduler")
		newRegistry, regErr := provider.NewRegistry(newCfg.Providers)
		if regErr != nil {
			slog.Error("failed to rebuild provider registry on reload", "error", regErr)
			return
		}
		newResolver := guide.NewResolver(cwd, newCfg)
		newExecutor := agent.NewExecutor(newResolver, newRegistry, newCfg)

		if scheduler != nil {
			scheduler.UpdateDeps(newExecutor, newCfg)
		} else {
			scheduler = pipeline.NewScheduler(newExecutor, nil, newCfg, logger, bus)
		}

		// Keep dashDeps current so status/provider handlers reflect new config.
		dashDeps.Cfg = newCfg
		dashDeps.Registry = newRegistry
		dashDeps.Resolver = newResolver
	}

	port := os.Getenv("POLVO_IDE_PORT")
	if port == "" {
		port = "7373"
	}
	addr := "127.0.0.1:" + port

	srv := server.NewServer(addr, bus, ".polvo/reports", dashDeps, cwd)

	// Wire the file watcher if we have a config.
	if cfg != nil {
		patterns := cfg.AllInterfacePatterns()
		patterns = append(patterns, "*.spec.md")
		debounceMs := cfg.Settings.DebounceMs
		if debounceMs == 0 {
			debounceMs = 300
		}

		w := watcher.New(cwd, patterns, debounceMs, func(event watcher.Event) {
			// Publish file-changed event to SSE stream.
			bus.PublishFileChanged(event.Path)

			if scheduler == nil {
				return
			}

			fileEvent := &pipeline.FileEvent{
				Type: classifyEvent(event.Path),
				File: event.Path,
			}
			if event.Op != "delete" {
				data, _ := os.ReadFile(event.Path)
				fileEvent.Content = string(data)
			}

			if err := scheduler.HandleEvent(context.Background(), fileEvent); err != nil {
				slog.Error("pipeline error", "error", err, "file", event.Path)
			}
		}, logger)

		ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer cancel()

		go func() {
			bus.PublishWatchStarted()
			if err := w.Start(); err != nil {
				slog.Error("watcher error", "error", err)
			}
			bus.PublishWatchStopped()
		}()

		go func() {
			<-ctx.Done()
			w.Stop()
		}()
	}

	slog.Info("Polvo IDE started", "addr", addr, "cwd", cwd)
	if err := srv.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func classifyEvent(path string) pipeline.EventType {
	if len(path) > 8 && path[len(path)-8:] == ".spec.md" {
		return pipeline.EventSpecChanged
	}
	return pipeline.EventInterfaceChanged
}
