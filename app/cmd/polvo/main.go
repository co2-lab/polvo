package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"database/sql"

	"github.com/co2-lab/polvo/internal/agent"
	"github.com/co2-lab/polvo/internal/config"
	"github.com/co2-lab/polvo/internal/telemetry"
	"github.com/co2-lab/polvo/internal/git"
	"github.com/co2-lab/polvo/internal/guide"
	"github.com/co2-lab/polvo/internal/ignore"
	"github.com/co2-lab/polvo/internal/memory"
	"github.com/co2-lab/polvo/internal/pipeline"
	"github.com/co2-lab/polvo/internal/provider"
	"github.com/co2-lab/polvo/internal/repomap"
	"github.com/co2-lab/polvo/internal/server"
	"github.com/co2-lab/polvo/internal/session"
	"github.com/co2-lab/polvo/internal/skill"
	"github.com/co2-lab/polvo/internal/tool"
	"github.com/co2-lab/polvo/internal/tool/mcp"
	"github.com/co2-lab/polvo/internal/tui"
	"github.com/co2-lab/polvo/internal/watcher"
	_ "modernc.org/sqlite"
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

	// Opt-in error reporting — only active when sentry_dsn is set in user config.
	if cfg != nil {
		telemetry.Init(telemetry.Config{
			Disabled:    cfg.Telemetry.Disabled,
			Environment: cfg.Telemetry.Environment,
			Release:     Version,
		})
		defer telemetry.Flush()
	}

	memStore, _ := memory.Open(cwd)
	if memStore != nil {
		defer memStore.Close()
	}

	// Start chunk indexer in background (non-fatal if it fails).
	// IndexAll runs in a goroutine; the indexer is also passed to tui.Config
	// so the TUI can update it when the user edits files via the agent.
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	_, tuiIndexer := startIndexer(ctx2, cwd)
	_ = tuiIndexer // used in tuiCfg below

	// Open session manager (shared memory.db, non-fatal).
	var sessMgr *session.Manager
	var summRunner *session.SummaryRunner
	if sessDB, err := openSessionDB(cwd); err == nil {
		if mgr, merr := session.Open(sessDB); merr == nil {
			sessMgr = mgr
			// Use LLM summarizer when summary_model is configured; otherwise noop.
			var summarizer session.Summarizer = session.NoopSummarizer{}
			if cfg != nil && cfg.Settings.SummaryModel != "" {
				// Provider registry not yet initialised here; wired after splash below.
				// We store the model name and create the LLM summarizer after registry is ready.
				_ = cfg.Settings.SummaryModel // placeholder — see post-splash wiring
			}
			summRunner = session.NewSummaryRunner(mgr, summarizer)
		} else {
			slog.Warn("session manager: open failed", "err", merr)
		}
	} else {
		slog.Warn("session manager: db open failed", "err", err)
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

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	ig, _ := ignore.Load(cwd)
	tuiToolOpts := tool.RegistryOptions{
		BraveAPIKey: cfg.Settings.BraveAPIKey,
		Ignore:      ig,
	}
	if hub := startMCPHub(ctx, cwd); hub != nil {
		tuiToolOpts.MCPHub = hub
	}
	toolReg := tool.DefaultRegistry(cwd, tuiToolOpts)

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

	// Build provider options for the /model picker.
	var providerOptions []tui.ProviderOption
	for name, pcfg := range cfg.Providers {
		p, err := registry.Get(name)
		if err != nil {
			continue
		}
		cp, ok := p.(provider.ChatProvider)
		if !ok {
			continue
		}
		model := pcfg.DefaultModel
		if model == "" {
			continue
		}
		providerOptions = append(providerOptions, tui.ProviderOption{
			Label:    name + " · " + model,
			Provider: cp,
			Model:    model,
		})
	}
	// Sort for stable order.
	sort.Slice(providerOptions, func(i, j int) bool {
		return providerOptions[i].Label < providerOptions[j].Label
	})

	// Upgrade SummaryRunner to LLM-backed if summary_model is configured.
	var summaryProvider provider.ChatProvider
	summaryModel := ""
	if cfg != nil && cfg.Settings.SummaryModel != "" {
		summaryModel = cfg.Settings.SummaryModel
		if p, err := registry.Default(); err == nil {
			if cp, ok := p.(provider.ChatProvider); ok {
				summaryProvider = cp
				if sessMgr != nil && summRunner != nil {
					summRunner = session.NewSummaryRunner(sessMgr, agent.LLMSummarizer{
						Provider: cp,
						Model:    summaryModel,
					})
				}
			}
		}
	}

	// Inject known project skills into the system prompt.
	if memStore != nil {
		if entries, rerr := memStore.Read(memory.Filter{Type: "decision", Limit: 5}); rerr == nil && len(entries) > 0 {
			var sb strings.Builder
			sb.WriteString("\n\n## Procedimentos conhecidos para este projeto:\n")
			for _, e := range entries {
				ts := time.Unix(0, e.Timestamp).Format("2006-01-02")
				fmt.Fprintf(&sb, "- [%s] %s\n", ts, e.Content)
			}
			systemPrompt += sb.String()
		}
	}

	// Build skill extractor (uses summary provider when available — cheaper model).
	var skillExtractor *skill.Extractor
	if memStore != nil && sel != nil {
		skillExtractor = &skill.Extractor{
			Provider: sel.Provider,
			Model:    sel.Model,
			Store:    memStore,
		}
		if summaryProvider != nil && summaryModel != "" {
			skillExtractor.Provider = summaryProvider
			skillExtractor.Model = summaryModel
		}
	}

	tuiCfg := tui.Config{
		WorkDir:         cwd,
		Provider:        sel.Provider,
		Model:           sel.Model,
		ToolReg:         toolReg,
		System:          systemPrompt,
		MaxTurns:        50,
		ProviderOptions: providerOptions,
		AddProviderFn:   buildAddProviderFn(),
		Indexer:         tuiIndexer,
		SessionManager:  sessMgr,
		SummaryRunner:   summRunner,
		SummaryModel:    summaryModel,
		SummaryProvider: summaryProvider,
		SkillExtractor:  skillExtractor,
	}

	return tui.Run(ctx, tuiCfg)
}

// buildAddProviderFn returns a function that runs the setup wizard and returns
// an updated list of ProviderOptions for the TUI /model picker.
func buildAddProviderFn() func() ([]tui.ProviderOption, error) {
	return func() ([]tui.ProviderOption, error) {
		if _, err := runSetupWizard(false); err != nil {
			return nil, err
		}

		newCfg, err := config.LoadWithUser("")
		if err != nil || newCfg == nil || len(newCfg.Providers) == 0 {
			return nil, fmt.Errorf("no provider configured after setup")
		}

		newRegistry, err := provider.NewRegistry(newCfg.Providers)
		if err != nil {
			return nil, fmt.Errorf("provider registry: %w", err)
		}

		var opts []tui.ProviderOption
		for name, pcfg := range newCfg.Providers {
			p, err := newRegistry.Get(name)
			if err != nil {
				continue
			}
			cp, ok := p.(provider.ChatProvider)
			if !ok {
				continue
			}
			model := pcfg.DefaultModel
			if model == "" {
				continue
			}
			opts = append(opts, tui.ProviderOption{
				Label:    name + " · " + model,
				Provider: cp,
				Model:    model,
			})
		}
		sort.Slice(opts, func(i, j int) bool {
			return opts[i].Label < opts[j].Label
		})
		return opts, nil
	}
}

// runServer is the existing HTTP server mode, used when launched as a Tauri sidecar.
func runServer() {
	logLevel := slog.LevelInfo
	seqURL := os.Getenv("SEQ_URL")
	if seqURL != "" {
		logLevel = slog.LevelDebug
	}
	h := slog.Handler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	if seqURL != "" {
		h = telemetry.NewMultiHandler(h, telemetry.NewSeqHandler(seqURL, "polvo-server", slog.LevelDebug))
	}
	logger := slog.New(h)
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

	// Opt-in error reporting — only active when sentry_dsn is set in user config.
	if cfg != nil {
		telemetry.Init(telemetry.Config{
			Disabled:    cfg.Telemetry.Disabled,
			Environment: cfg.Telemetry.Environment,
			Release:     Version,
		})
		defer telemetry.Flush()
	}

	var registry *provider.Registry
	var resolver *guide.Resolver
	var executor *agent.Executor
	var scheduler *pipeline.Scheduler
	var dispatcher *watcher.Dispatcher

	eventCh := make(chan watcher.WatchEvent, 64)

	// Start chunk indexer in background (non-fatal).
	_, srvIndexer := startIndexer(context.Background(), cwd)

	if cfg != nil {
		registry, _ = provider.NewRegistry(cfg.Providers)
		resolver = guide.NewResolver(cwd, cfg)
		executor = agent.NewExecutor(resolver, registry, cfg)
		if hub := startMCPHub(context.Background(), cwd); hub != nil {
			executor.WithMCP(hub)
		}
		scheduler = pipeline.NewScheduler(executor, nil, cfg, logger, bus)
		// Wire supervisor router when supervisor_model is configured.
		if cfg.Settings.SupervisorModel != "" {
			if p, err := registry.Default(); err == nil {
				if cp, ok := p.(provider.ChatProvider); ok {
					sup := pipeline.NewSupervisor(cp, cfg.Settings.SupervisorModel, nil, cfg.Settings.SupervisorConfidenceThreshold)
					scheduler.WithSupervisor(sup)
				}
			}
		}
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

	// If another instance is already listening on the port, exit cleanly.
	// This can happen when Tauri hot-reloads and spawns a new sidecar before
	// the old one has fully terminated.
	if ln, err := net.Listen("tcp", addr); err != nil {
		slog.Warn("port already in use, another instance is running", "addr", addr)
		os.Exit(0)
	} else {
		ln.Close()
	}

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
						handleIndexEvent(srvIndexer, event)
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
	logLevel := slog.LevelInfo
	seqURL := os.Getenv("SEQ_URL")
	if seqURL != "" {
		logLevel = slog.LevelDebug
	}
	h := slog.Handler(slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: logLevel}))
	if seqURL != "" {
		h = telemetry.NewMultiHandler(h, telemetry.NewSeqHandler(seqURL, "polvo-tui", slog.LevelDebug))
	}
	slog.SetDefault(slog.New(h))
	return nil
}


// startMCPHub loads MCP config from .polvo/mcp.json (project) or ~/.polvo/mcp.json (user)
// and starts the hub. Returns nil (non-fatal) if no config file is found or startup fails.
func startMCPHub(ctx context.Context, cwd string) *mcp.MCPHub {
	// Try project-local config first, then user-level config.
	paths := []string{
		filepath.Join(cwd, ".polvo", "mcp.json"),
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".polvo", "mcp.json"))
	}

	for _, p := range paths {
		cfg, err := mcp.LoadMCPConfig(p)
		if err != nil || len(cfg.MCPServers) == 0 {
			continue
		}
		hub := mcp.NewMCPHub(cfg)
		if err := hub.Start(ctx); err != nil {
			slog.Warn("mcp: hub start failed", "path", p, "error", err)
			continue
		}
		slog.Info("mcp: hub started", "path", p, "servers", len(cfg.MCPServers))
		return hub
	}
	return nil
}

// startIndexer opens the chunk index, creates an Indexer, and triggers IndexAll
// in a background goroutine. Returns (chunkIndex, indexer, cleanup).
// All return values may be nil if the DB cannot be opened — non-fatal.
func startIndexer(ctx context.Context, cwd string) (*repomap.ChunkIndex, *repomap.Indexer) {
	dbPath := filepath.Join(cwd, ".polvo", "index.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		slog.Warn("indexer: cannot create .polvo dir", "err", err)
		return nil, nil
	}
	idx, err := repomap.OpenChunkIndex(dbPath)
	if err != nil {
		slog.Warn("indexer: cannot open chunk index", "err", err)
		return nil, nil
	}
	ix := repomap.NewIndexer(cwd, idx)
	go func() {
		ictx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if err := ix.IndexAll(ictx); err != nil {
			slog.Warn("indexer: IndexAll failed", "err", err)
		} else {
			slog.Info("indexer: IndexAll done")
		}
	}()
	return idx, ix
}

// handleIndexEvent updates the chunk index and .symbols sidecar for a file event.
func handleIndexEvent(ix *repomap.Indexer, ev watcher.WatchEvent) {
	if ix == nil {
		return
	}
	go func() {
		switch ev.Op {
		case watcher.OpCreate, watcher.OpModify:
			if err := ix.IndexFile(ev.Path); err != nil {
				slog.Warn("indexer: IndexFile", "path", ev.Path, "err", err)
			}
		case watcher.OpDelete:
			if err := ix.RemoveFile(ev.Path); err != nil {
				slog.Warn("indexer: RemoveFile", "path", ev.Path, "err", err)
			}
		}
	}()
}

func classifyEvent(path string) pipeline.EventType {
	if len(path) > 8 && path[len(path)-8:] == ".spec.md" {
		return pipeline.EventSpecChanged
	}
	return pipeline.EventInterfaceChanged
}

// openSessionDB opens (or creates) .polvo/memory.db for use by the session manager.
// The session manager shares the same file as memory.Store but uses its own *sql.DB
// connection so it can create its own tables without touching memory.Store internals.
func openSessionDB(cwd string) (*sql.DB, error) {
	dir := filepath.Join(cwd, ".polvo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "memory.db"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}
