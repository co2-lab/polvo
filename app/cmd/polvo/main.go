package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/co2-lab/polvo/internal/agent"
	"github.com/co2-lab/polvo/internal/config"
	"github.com/co2-lab/polvo/internal/git"
	"github.com/co2-lab/polvo/internal/guide"
	"github.com/co2-lab/polvo/internal/ignore"
	"github.com/co2-lab/polvo/internal/memory"
	"github.com/co2-lab/polvo/internal/pipeline"
	"github.com/co2-lab/polvo/internal/provider"
	"github.com/co2-lab/polvo/internal/server"
	"github.com/co2-lab/polvo/internal/tool"
	"github.com/co2-lab/polvo/internal/tui"
	"github.com/co2-lab/polvo/internal/watcher"
)

// Version is set via ldflags at build time.
var (
	Version   = "dev"
	CommitSHA = "unknown"
	BuildDate = "unknown"
)

func main() {
	// ── Mode detection ──────────────────────────────────────────────────────
	// POLVO_SIDECAR=1 is set by the Tauri desktop app when spawning us as a
	// background sidecar. In that case we always run as an HTTP server, no
	// matter what os.Args says.
	if os.Getenv("POLVO_SIDECAR") == "1" {
		runServer()
		return
	}

	args := os.Args[1:]

	// `polvo hook <name>` → git hook runner
	if len(args) >= 2 && args[0] == "hook" {
		if err := runHook(args[1]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	// `polvo /some/path` or `polvo .` or `polvo ..` → open desktop app
	if len(args) == 1 && looksLikePath(args[0]) {
		if err := launchDesktop(args[0]); err != nil {
			fmt.Fprintf(os.Stderr, "polvo: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// `polvo` (no args) → interactive TUI agent
	if len(args) == 0 {
		if err := runTUI(); err != nil {
			fmt.Fprintf(os.Stderr, "polvo: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Unrecognised args: print usage and exit.
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  polvo              — interactive agent TUI")
	fmt.Fprintln(os.Stderr, "  polvo <path>       — open desktop app with <path> as project root")
	os.Exit(1)
}

// looksLikePath returns true when s is clearly a filesystem path rather than
// a subcommand name.
func looksLikePath(s string) bool {
	if s == "" {
		return false
	}
	return s == "." || s == ".." ||
		strings.HasPrefix(s, "/") ||
		strings.HasPrefix(s, "./") ||
		strings.HasPrefix(s, "../") ||
		strings.HasPrefix(s, "~")
}

// launchDesktop opens the Polvo desktop app with the given path as project root.
// On macOS it uses `open -a Polvo <path>` which works regardless of whether
// the app is in /Applications or ~/Applications.
func launchDesktop(arg string) error {
	abs, err := filepath.Abs(arg)
	if err != nil {
		return fmt.Errorf("resolving path %q: %w", arg, err)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return fmt.Errorf("evaluating symlinks for %q: %w", abs, err)
	}
	info, err := os.Stat(real)
	if err != nil {
		return fmt.Errorf("stat %q: %w", real, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%q is not a directory", arg)
	}

	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", "-a", "Polvo", real).Run()
	case "linux":
		// When a packaged desktop binary exists alongside this CLI binary,
		// prefer it. Otherwise fall back to xdg-open (opens the project dir).
		desktopBin := filepath.Join(filepath.Dir(os.Args[0]), "polvo-desktop")
		if _, statErr := os.Stat(desktopBin); statErr == nil {
			return exec.Command(desktopBin, real).Run()
		}
		return exec.Command("xdg-open", real).Run()
	case "windows":
		return exec.Command("cmd", "/C", "start", "", "Polvo", real).Run()
	default:
		return fmt.Errorf("desktop launch not supported on %s", runtime.GOOS)
	}
}

// runTUI starts the interactive terminal agent UI.
func runTUI() error {
	cwd, err := resolveWorkDir()
	if err != nil {
		return err
	}

	// Redirect logs to a file so they don't corrupt the TUI.
	if logErr := redirectLogsToFile(cwd); logErr != nil {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	}

	// Install git hooks silently — protects against committing api_key to git.
	_ = git.InstallHooks(context.Background(), cwd)

	// Load: base → ~/.polvo/config.yaml → polvo.yaml (all optional except base).
	cfg, _ := config.LoadWithUser("polvo.yaml")
	if cfg == nil {
		cfg, _ = config.LoadWithUser("")
	}

	memStore, _ := memory.Open(cwd)
	if memStore != nil {
		defer memStore.Close()
	}

	// ── Splash screen ─────────────────────────────────────────────────────────
	// Always show the splash screen. If no providers are configured it will
	// return nil and we fall through to the setup wizard.
	var registry *provider.Registry
	if cfg != nil && len(cfg.Providers) > 0 {
		registry, err = provider.NewRegistry(cfg.Providers)
		if err != nil {
			return fmt.Errorf("provider registry: %w", err)
		}
	}

	sel, splashErr := showSplash(Version, cwd, cfg, registry)
	if splashErr != nil {
		return splashErr
	}

	// No provider selected (registry was nil / empty) → run setup wizard.
	if sel == nil {
		if _, wizErr := runSetupWizard(true); wizErr != nil {
			return wizErr
		}
		// Reload config after setup.
		cfg, _ = config.LoadWithUser("")
		if cfg == nil || len(cfg.Providers) == 0 {
			return fmt.Errorf("no provider configured after setup")
		}
		registry, err = provider.NewRegistry(cfg.Providers)
		if err != nil {
			return fmt.Errorf("provider registry: %w", err)
		}
		// Only one provider configured (the one just created): use it directly
		// without showing the splash selection screen again.
		if len(cfg.Providers) == 1 {
			sel, err = autoSelectProvider(cfg, registry)
			if err != nil {
				return err
			}
		} else {
			sel, splashErr = showSplash(Version, cwd, cfg, registry)
			if splashErr != nil {
				return splashErr
			}
		}
		if sel == nil {
			return fmt.Errorf("no provider selected")
		}
	}

	// ── Launch TUI ────────────────────────────────────────────────────────────

	ig, _ := ignore.Load(cwd)
	toolReg := tool.DefaultRegistry(cwd, tool.RegistryOptions{
		BraveAPIKey: cfg.Settings.BraveAPIKey,
		Ignore:      ig,
	})

	systemPrompt := fmt.Sprintf(`You are Polvo, an AI coding agent running in interactive CLI mode.
Working directory: %s

TOOL USE DISCIPLINE — follow strictly:
- Only use tools when the task genuinely requires inspecting or modifying files/system state.
- For simple factual questions or shell commands the user explicitly typed (e.g. "pwd", "ls", "echo hello"), run that single command and reply. Do NOT explore the project further.
- Do NOT chain multiple exploratory tools (ls → read → ls → read) unless the task clearly requires it.
- Do NOT run tests, builds, or refactors unless explicitly asked.
- Prefer reading specific files over broad ls/glob sweeps.
- If the user's intent is ambiguous, ask — don't explore speculatively.

RESPONSE STYLE:
- Be direct and concise. One tool call per logical step when possible.
- Show results, not process. Don't narrate what you're about to do.
- If a task is simple, answer simply.`, cwd)

	tuiCfg := tui.Config{
		WorkDir:  cwd,
		Provider: sel.Provider,
		Model:    sel.Model,
		ToolReg:  toolReg,
		System:   systemPrompt,
		MaxTurns: 50,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	return tui.Run(ctx, tuiCfg)
}

// runServer is the existing HTTP server mode, used when launched as a Tauri sidecar.
func runServer() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	bus := server.NewBus()

	cwd, err := resolveWorkDir()
	if err != nil {
		slog.Error("cannot determine working directory", "error", err)
		os.Exit(1)
	}

	memStore, err := memory.Open(cwd)
	if err != nil {
		slog.Warn("failed to open memory store, continuing without it", "error", err)
	} else {
		defer memStore.Close()
	}

	// Install git hooks silently — protects against committing api_key to git.
	_ = git.InstallHooks(context.Background(), cwd)

	cfg, err := config.LoadWithUser("polvo.yaml")
	if err != nil {
		slog.Warn("failed to load config, running in unconfigured mode", "error", err)
	}

	var registry *provider.Registry
	var resolver *guide.Resolver
	var executor *agent.Executor
	var scheduler *pipeline.Scheduler
	var dispatcher *watcher.Dispatcher

	eventCh := make(chan watcher.WatchEvent, 64)

	if cfg != nil {
		registry, _ = provider.NewRegistry(cfg.Providers)
		resolver = guide.NewResolver(cwd, cfg)
		executor = agent.NewExecutor(resolver, registry, cfg)
		scheduler = pipeline.NewScheduler(executor, nil, cfg, logger, bus)
		dispatcher = watcher.NewDispatcher(cfg, executor, eventCh, logger)
	}

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
		if dispatcher != nil {
			dispatcher.UpdateConfig(newCfg)
		}

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

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if cfg != nil {
		debounceMs := cfg.Settings.DebounceMs
		if debounceMs == 0 {
			debounceMs = 300
		}

		var watchers []*watcher.Watcher
		for name, wcfg := range cfg.Watchers {
			root := wcfg.Path
			if root == "" {
				root = cwd
			}
			w := watcher.New(name, root, wcfg.Pattern, debounceMs, eventCh, logger)
			watchers = append(watchers, w)
			go func(w *watcher.Watcher) {
				bus.PublishWatchStarted()
				if err := w.Start(); err != nil {
					slog.Error("watcher error", "error", err)
				}
			}(w)
		}

		if len(cfg.Interfaces) > 0 {
			legacyCh := make(chan watcher.WatchEvent, 32)
			patterns := cfg.AllInterfacePatterns()
			patterns = append(patterns, "*.spec.md")
			legacyW := watcher.New("__legacy__", cwd, patterns, debounceMs, legacyCh, logger)
			go func() {
				if err := legacyW.Start(); err != nil {
					slog.Error("legacy watcher error", "error", err)
				}
			}()
			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					case event, ok := <-legacyCh:
						if !ok {
							return
						}
						bus.PublishFileChanged(event.Path)
						if scheduler == nil {
							continue
						}
						fileEvent := &pipeline.FileEvent{
							Type: classifyEvent(event.Path),
							File: event.Path,
						}
						if event.Op != watcher.OpDelete {
							data, _ := os.ReadFile(event.Path)
							fileEvent.Content = string(data)
						}
						if err := scheduler.HandleEvent(context.Background(), fileEvent); err != nil {
							slog.Error("pipeline error", "error", err, "file", event.Path)
						}
					}
				}
			}()
			go func() {
				<-ctx.Done()
				legacyW.Stop()
			}()
		}

		if dispatcher != nil {
			go dispatcher.Run(ctx)
		}

		go func() {
			<-ctx.Done()
			for _, w := range watchers {
				w.Stop()
			}
			bus.PublishWatchStopped()
		}()
	}

	slog.Info("Polvo IDE started", "addr", addr, "cwd", cwd)
	if err := srv.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// resolveWorkDir returns the working directory from POLVO_ROOT env or os.Getwd.
func resolveWorkDir() (string, error) {
	if root := os.Getenv("POLVO_ROOT"); root != "" {
		if err := os.Chdir(root); err != nil {
			return "", fmt.Errorf("chdir to POLVO_ROOT %q: %w", root, err)
		}
		return root, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd: %w", err)
	}
	return cwd, nil
}

// redirectLogsToFile redirects the default slog logger to ~/.polvo/tui.log.
func redirectLogsToFile(cwd string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	logDir := filepath.Join(homeDir, ".polvo")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}
	logFile, err := os.OpenFile(filepath.Join(logDir, "tui.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_ = cwd // available for future structured log fields
	slog.SetDefault(slog.New(slog.NewTextHandler(logFile, nil)))
	return nil
}


func classifyEvent(path string) pipeline.EventType {
	if len(path) > 8 && path[len(path)-8:] == ".spec.md" {
		return pipeline.EventSpecChanged
	}
	return pipeline.EventInterfaceChanged
}
