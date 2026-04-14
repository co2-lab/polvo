package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/co2-lab/polvo/internal/assets"
	"github.com/co2-lab/polvo/internal/config"
	"github.com/co2-lab/polvo/internal/git"
	"github.com/co2-lab/polvo/internal/guide"
	"github.com/co2-lab/polvo/internal/ignore"
	"github.com/co2-lab/polvo/internal/memory"
	"github.com/co2-lab/polvo/internal/provider"
	"github.com/co2-lab/polvo/internal/tool"
)

// Executor builds and manages agents from config.
type Executor struct {
	resolver  *guide.Resolver
	registry  *provider.Registry
	cfg       *config.Config
	agents    map[string]Agent
	memStore  *memory.Store       // optional; may be nil
	recallCfg memory.RecallConfig // cross-session recall settings
}

// NewExecutor creates a new agent executor.
func NewExecutor(resolver *guide.Resolver, registry *provider.Registry, cfg *config.Config) *Executor {
	return &Executor{
		resolver:  resolver,
		registry:  registry,
		cfg:       cfg,
		agents:    make(map[string]Agent),
		recallCfg: memory.DefaultRecallConfig(),
	}
}

// WithMemory attaches a memory store and recall config to the executor.
// Call this after NewExecutor to enable cross-session recall.
func (e *Executor) WithMemory(store *memory.Store, cfg memory.RecallConfig) *Executor {
	e.memStore = store
	e.recallCfg = cfg
	return e
}

// RunAgentForFile runs the named agent with the given file path as input.
// This is the implementation used by the watcher dispatcher.
func (e *Executor) RunAgentForFile(ctx context.Context, agentName, filePath string) error {
	a, err := e.GetAgent(agentName, nil)
	if err != nil {
		return fmt.Errorf("getting agent %q: %w", agentName, err)
	}

	// Set up git orchestrator if any git integration is enabled.
	var orch *git.Orchestrator
	if e.cfg != nil && (e.cfg.Git.AutoCommit || e.cfg.Git.BranchPerRun || e.cfg.Git.DirtyCommit) {
		cwd, _ := os.Getwd()
		gitClient := &git.ExecClient{WorkDir: cwd}
		orchCfg := git.OrchestratorConfig{
			AutoCommit:     e.cfg.Git.AutoCommit,
			DirtyCommit:    e.cfg.Git.DirtyCommit,
			BranchPerRun:   e.cfg.Git.BranchPerRun,
			BranchTemplate: e.cfg.Git.BranchTemplate,
			Attribution:    e.cfg.Git.Attribution,
		}
		orch = git.NewOrchestrator(orchCfg, gitClient, nil)
		if preErr := orch.PreRun(ctx, agentName); preErr != nil {
			slog.Warn("git pre-run failed", "agent", agentName, "err", preErr)
		}
	}

	input := &Input{Event: "watcher", File: filePath}
	_, execErr := a.Execute(ctx, input)

	if execErr == nil && orch != nil {
		if postErr := orch.PostRun(ctx, agentName, nil); postErr != nil {
			slog.Warn("git post-run failed", "agent", agentName, "err", postErr)
		}
	}

	return execErr
}

// RunSubAgent implements tool.SubAgentRunner.
// It runs a named agent in read-only (plan) mode and returns its text output.
func (e *Executor) RunSubAgent(ctx context.Context, agentName, task string) (string, error) {
	a, err := e.GetAgent(agentName, nil)
	if err != nil {
		return "", fmt.Errorf("getting sub-agent %q: %w", agentName, err)
	}
	// Run with read-only mode by passing autonomy context via Input.Event
	input := &Input{Event: "delegate", Content: task}
	res, err := a.Execute(ctx, input)
	if err != nil {
		return "", err
	}
	if res.Summary != "" {
		return res.Summary, nil
	}
	return res.Content, nil
}

// RunExplore implements tool.ExploreRunner.
// It launches read-only subagents in parallel for each task and returns a
// markdown-formatted summary of all findings.
func (e *Executor) RunExplore(ctx context.Context, tasks []tool.ExploreTaskInput, tokenBudgetPerTask int) (string, error) {
	if DelegateLevelFromCtx(ctx) >= 1 {
		return "", fmt.Errorf("explore not available in subagents")
	}

	exploreTasks := make([]ExploreTask, len(tasks))
	for i, t := range tasks {
		exploreTasks[i] = ExploreTask{
			Description: t.Description,
			Focus:       t.Focus,
			TokenBudget: tokenBudgetPerTask,
		}
	}

	// Get the model from config (use summary role if configured, fall back to empty)
	model := ""
	if e.cfg != nil {
		for _, gcfg := range e.cfg.Guides {
			if gcfg.Model != "" {
				model = gcfg.Model
				break
			}
		}
	}

	results, err := ExploreParallel(ctx, exploreTasks, e, model, tokenBudgetPerTask*len(tasks), 0)
	if err != nil {
		return "", err
	}

	// Format results as markdown
	descs := make([]string, len(results))
	summaries := make([]string, len(results))
	errs := make([]error, len(results))
	for i, r := range results {
		descs[i] = r.TaskDescription
		summaries[i] = r.Summary
		errs[i] = r.Err
	}
	return tool.FormatExploreResults(descs, summaries, errs), nil
}

// GetAgent returns (or creates) an agent for the given guide name and interface group.
func (e *Executor) GetAgent(guideName string, group *config.InterfaceGroupConfig) (Agent, error) {
	key := guideName
	if group != nil {
		// We use the group pattern matching as a key prefix to cache per group
		key = strings.Join(group.Patterns, ",") + ":" + guideName
	}

	if a, ok := e.agents[key]; ok {
		return a, nil
	}

	a, err := e.buildAgent(guideName, group)
	if err != nil {
		return nil, err
	}
	e.agents[key] = a
	return a, nil
}

func (e *Executor) buildAgent(guideName string, group *config.InterfaceGroupConfig) (Agent, error) {
	// Resolve the correct guide configuration (Group overrides Global)
	var gcfg config.GuideConfig
	if group != nil {
		gcfg = group.GetGuideConfig(guideName, e.cfg.Guides)
	} else {
		gcfg = e.cfg.Guides[guideName]
	}

	// Resolve guide content
	g, err := e.resolver.Resolve(guideName, gcfg)
	if err != nil {
		return nil, fmt.Errorf("resolving guide %q: %w", guideName, err)
	}

	// Prepend cross-session recall context when memory store is available and recall is enabled.
	if e.memStore != nil && e.recallCfg.Enabled {
		recallCtx, recallErr := memory.Recall(context.Background(), e.memStore, guideName, e.recallCfg)
		if recallErr != nil {
			slog.Warn("cross-session recall failed", "agent", guideName, "error", recallErr)
		} else if recallCtx != "" {
			g.Content = recallCtx + "\n" + g.Content
		}
	}

	// Load prompt template
	var promptContent string
	if gcfg.Prompt != "" {
		promptContent = gcfg.Prompt
	} else {
		data, err := assets.ReadPrompt(guideName)
		if err != nil {
			return nil, fmt.Errorf("reading prompt for %q: %w", guideName, err)
		}
		promptContent = string(data)
	}

	// Resolve provider
	providerName := gcfg.Provider
	var p provider.LLMProvider
	if providerName != "" {
		p, err = e.registry.Get(providerName)
	} else {
		p, err = e.registry.Default()
	}
	if err != nil {
		return nil, fmt.Errorf("resolving provider for %q: %w", guideName, err)
	}

	role := RoleAuthor
	if g.Role == "reviewer" {
		role = RoleReviewer
	}

	// Use ToolLLMAgent if tools are enabled and provider supports Chat
	if gcfg.UseTools {
		if cp, ok := p.(provider.ChatProvider); ok {
			cwd, _ := os.Getwd()
			ig, _ := ignore.Load(cwd) // nil-safe: errors treated as empty set
			toolOpts := tool.RegistryOptions{
				BraveAPIKey:    e.cfg.Settings.BraveAPIKey,
				ExtraBlocklist: e.cfg.Permissions.Blocklist,
				Ignore:         ig,
				SubAgent:       e, // executor implements tool.SubAgentRunner
				Explore:        e, // executor implements tool.ExploreRunner (only at delegate level 0)
			}
			if e.cfg.Settings.PersistentBashSession {
				toolTimeout := time.Duration(e.cfg.Settings.ToolTimeout) * time.Second
				if toolTimeout <= 0 {
					toolTimeout = 120 * time.Second
				}
				bashLimits := tool.BashLimits{
					MaxCPUSecs:    e.cfg.Settings.BashMaxCPUSecs,
					MaxMemMB:      e.cfg.Settings.BashMaxMemMB,
					MaxFileSizeMB: e.cfg.Settings.BashMaxFileSizeMB,
				}
				if sess, err := tool.NewBashSessionWithLimits(cwd, toolTimeout, bashLimits); err == nil {
					toolOpts.BashSession = sess
				}
			}
			toolReg := tool.DefaultRegistry(cwd, toolOpts)
			agent := NewToolLLMAgent(guideName, role, g.Content, promptContent, cp, gcfg.Model, toolReg)
			if gcfg.ArchitectEditor.Enabled {
				agent.WithArchitectEditor(ArchitectEditorConfig{
					Enabled:           true,
					ArchitectModel:    gcfg.ArchitectEditor.ArchitectModel,
					EditorModel:       gcfg.ArchitectEditor.EditorModel,
					MaxArchitectTurns: gcfg.ArchitectEditor.MaxArchitectTurns,
					MaxEditorTurns:    gcfg.ArchitectEditor.MaxEditorTurns,
				})
			}
			return agent, nil
		}
	}

	return NewLLMAgent(guideName, role, g.Content, promptContent, p, gcfg.Model), nil
}
