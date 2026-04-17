package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/co2-lab/polvo/internal/agent/checkpoint"
	"github.com/co2-lab/polvo/internal/agent/microagent"
	"github.com/co2-lab/polvo/internal/audit"
	"github.com/co2-lab/polvo/internal/hooks"
	"github.com/co2-lab/polvo/internal/memory"
	"github.com/co2-lab/polvo/internal/provider"
	"github.com/co2-lab/polvo/internal/risk"
	"github.com/co2-lab/polvo/internal/tool"
)

const defaultMaxTurns = 50

// ContextFallbackConfig configures context window overflow handling.
type ContextFallbackConfig struct {
	// ContextWindowFallbacks maps model names to ordered fallback model lists.
	ContextWindowFallbacks map[string][]string
	// MaxFallbackDepth is the maximum model-fallback hops (default 3).
	MaxFallbackDepth int
	// MinOutputTokens is the minimum token budget reserved for output (default 1000).
	MinOutputTokens int
	// ContextWindow is the context window size for the primary model (0 = no pre-call check).
	ContextWindow int
	// SummaryProvider is an optional cheap LLM for summarization.
	SummaryProvider provider.ChatProvider
	// SummaryModel is the model to use for summarization.
	SummaryModel string
}

// Phase identifies a named execution phase in a multi-phase loop.
type Phase string

const (
	PhaseContext Phase = "context" // read-only exploration
	PhasePlan    Phase = "plan"    // planning / reasoning
	PhaseBuild   Phase = "build"   // implementation (full tools)
	PhaseVerify  Phase = "verify"  // tests + lint
	PhaseCommit  Phase = "commit"  // git commit
)

// PhaseBudget configures per-phase execution parameters.
type PhaseBudget struct {
	MaxTokens int    // token budget for this phase (0 = use LoopConfig.MaxTokens)
	MaxTurns  int    // max turns (0 = use LoopConfig.MaxTurns)
	Model     string // model override (empty = use LoopConfig.Model)
}

// SuspendSignal is sent on SuspendCh when the loop needs human input to continue.
type SuspendSignal struct {
	SessionID    string
	CheckpointID string
	Reason       checkpoint.SuspendReason
	Preview      string // human-readable description of the situation
}

// LoopControl provides granular interrupt/abort/resume signalling for a running loop.
// All channels are created by the caller; zero values (nil channels) are ignored.
type LoopControl struct {
	// Close (or send) to request a clean pause after the current tool batch.
	Interrupt chan struct{}
	// Close (or send) to abort the loop immediately (cancels context).
	Abort chan struct{}
	// Close (or send) to resume after a clean pause.
	Resume chan struct{}
}

// NewLoopControl creates a LoopControl with all channels initialized.
func NewLoopControl() *LoopControl {
	return &LoopControl{
		Interrupt: make(chan struct{}),
		Abort:     make(chan struct{}),
		Resume:    make(chan struct{}),
	}
}

// LoopConfig configures the agentic loop.
type LoopConfig struct {
	Provider     provider.ChatProvider
	Tools        *tool.Registry
	GuardedTools *tool.GuardedRegistry // optional: permission-checked execution
	System       string
	Model        string
	MaxTurns     int
	MaxTokens    int
	RunTimeout   time.Duration // total execution timeout (0 = no limit)

	// Stuck detection
	StuckWindowSize int // default 6
	StuckThreshold  int // default 3

	// Consecutive timeout limit: abort after this many consecutive
	// context.DeadlineExceeded errors from LLM or tool calls (0 = disabled).
	MaxConsecutiveTimeouts int // default 0 (disabled); use 3 as a sensible default

	// Memory store for persisting stuck detection events (optional).
	Memory *memory.Store

	// Reflection loop
	Reflector *Reflector

	// Context window fallback cascade
	ContextFallback ContextFallbackConfig

	// Architect/editor two-phase loop
	ArchitectEditor ArchitectEditorConfig

	// PermissionRules overrides the default tool permission rules.
	// When nil, DefaultPermissionRules() is used.
	// These rules are consulted when GuardedTools is nil.
	PermissionRules []tool.PermissionRule

	// Approval gates: called before executing ask-permission tools.
	// nil = allow all (backward-compatible).
	PermissionCallback PermissionCallback

	// Audit logging: every tool call is logged with decision + duration.
	// nil = no audit logging.
	AuditLogger audit.Logger

	// Checkpoint recorder: records events and file snapshots for time-travel.
	// nil = no checkpointing.
	Checkpoint *checkpoint.Recorder

	// MicroagentLoader loads context-injecting microagents.
	// When set, matched microagents are prepended to the system prompt before the first turn.
	// nil = no microagent injection.
	MicroagentLoader *microagent.Loader

	// Hooks runner for lifecycle events (nil = disabled).
	Hooks *hooks.Runner

	// RiskScorer scores tool calls before execution.
	// nil = use risk.NoopScorer (backward compat).
	RiskScorer risk.RiskScorer

	// SessionID is propagated to audit entries.
	SessionID string

	// SuspendCh, when non-nil, receives a SuspendSignal when the loop suspends
	// (e.g. consecutive timeouts). The caller should persist state and then send
	// human input via ResumeCh to continue execution.
	SuspendCh chan<- SuspendSignal

	// ResumeCh receives the human guidance string that lets the loop continue
	// after a suspension. Only consulted when SuspendCh is non-nil.
	ResumeCh <-chan string

	// SteerCh, when non-nil, allows mid-task real-time steering.
	// Between tool-call batches the loop drains SteerCh and injects any
	// pending messages as user turns, changing the agent's direction without
	// restarting the session. Messages are injected non-blocking.
	SteerCh <-chan string

	// Control, when non-nil, provides granular interrupt/abort/resume signalling.
	// Interrupt: loop pauses cleanly after the current tool batch completes.
	// Abort: loop stops immediately (context cancelled).
	// Resume: loop continues after a pause.
	Control *LoopControl

	// InlineSummary enables extraction of <summary>...</summary> blocks from
	// model responses. When true, the summary tag is stripped from the visible
	// response and delivered via OnTurnSummary. Use when no dedicated
	// SummaryProvider is configured.
	InlineSummary bool

	// PhaseBudgets sets per-phase token, turn, and model overrides.
	// When CurrentPhase is set, the matching budget overrides MaxTokens/MaxTurns/Model.
	PhaseBudgets map[Phase]PhaseBudget

	// CurrentPhase, when set, activates the corresponding PhaseBudget overrides.
	// Emitted via EventStream as EventTurnStart.Kind annotations.
	CurrentPhase Phase

	// EventStream receives typed loop events (tool calls, LLM turns, approvals, errors).
	// nil = no event streaming. Subscribers call EventStream.Subscribe() before Run().
	EventStream *EventStream

	// Callbacks
	OnText        func(text string)
	OnTextDelta   func(delta string)
	OnToolCall    func(call provider.ToolCall)
	OnToolResult  func(id, name, result string, isError bool)
	OnTurnSummary func(turnIdx int, summary string) // called when inline summary extracted
}

// LoopResult is the outcome of a loop execution.
type LoopResult struct {
	FinalText         string
	TurnCount         int
	TokensUsed        provider.TokenUsage
	Metrics           *AgentMetrics
	StuckPattern      StuckPattern // pattern that caused the loop to abort (if any)
	FailedPhase       string       // name of the first reflection phase that failed
	ReflectionRetries int          // total reflection retries performed
}

// Loop implements the agentic prompt→LLM→tools→LLM cycle.
type Loop struct {
	cfg                 LoopConfig
	conv                *Conversation
	stuck               *StuckDetector
	consecutiveTimeouts int
}

// Conv returns the conversation managed by this loop. Used by callers that need
// to apply turn marks or inspect conversation state after execution.
func (l *Loop) Conv() *Conversation { return l.conv }

// checkInterrupt checks if the Interrupt or Abort signals are pending.
// If Interrupt is pending, it waits for Resume or ctx cancellation, then returns false.
// If Abort is pending, it returns true (caller should stop).
// Returns true if the loop should stop.
func (l *Loop) checkInterrupt(ctx context.Context) bool {
	ctrl := l.cfg.Control
	if ctrl == nil {
		return false
	}

	// Non-blocking check for Abort first (highest priority).
	select {
	case <-ctrl.Abort:
		slog.Info("loop: abort signal received", "agent", l.cfg.Model)
		return true
	default:
	}

	// Non-blocking check for Interrupt.
	select {
	case <-ctrl.Interrupt:
		slog.Info("loop: interrupt signal received — pausing", "agent", l.cfg.Model)
		if l.cfg.EventStream != nil {
			l.cfg.EventStream.Emit(StreamEvent{
				Kind:      EventError,
				AgentName: l.cfg.Model,
				SessionID: l.cfg.SessionID,
				Message:   "paused: waiting for resume",
			})
		}
		// Wait for Resume or ctx cancellation.
		select {
		case <-ctrl.Resume:
			slog.Info("loop: resumed", "agent", l.cfg.Model)
			return false
		case <-ctrl.Abort:
			slog.Info("loop: aborted while paused", "agent", l.cfg.Model)
			return true
		case <-ctx.Done():
			return true
		}
	default:
		return false
	}
}

// drainSteer drains all pending messages from SteerCh and injects each as a
// user turn in the conversation. This is called between tool-call batches so
// the user can redirect the agent without restarting.
func (l *Loop) drainSteer() {
	if l.cfg.SteerCh == nil {
		return
	}
	for {
		select {
		case msg, ok := <-l.cfg.SteerCh:
			if !ok {
				return
			}
			if msg != "" {
				l.conv.AddUser(msg)
				slog.Info("loop: steering message injected", "agent", l.cfg.Model)
				if l.cfg.EventStream != nil {
					l.cfg.EventStream.Emit(StreamEvent{
						Kind:      EventTurnStart,
						AgentName: l.cfg.Model,
						SessionID: l.cfg.SessionID,
						Message:   "[steering] " + msg,
					})
				}
			}
		default:
			return
		}
	}
}

// NewLoop creates a new agentic loop.
func NewLoop(cfg LoopConfig) *Loop {
	// Apply phase budget overrides if a current phase is set.
	if cfg.CurrentPhase != "" && len(cfg.PhaseBudgets) > 0 {
		if budget, ok := cfg.PhaseBudgets[cfg.CurrentPhase]; ok {
			if budget.MaxTokens > 0 {
				cfg.MaxTokens = budget.MaxTokens
			}
			if budget.MaxTurns > 0 {
				cfg.MaxTurns = budget.MaxTurns
			}
			if budget.Model != "" {
				cfg.Model = budget.Model
			}
		}
	}
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = defaultMaxTurns
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 16384
	}
	return &Loop{
		cfg:   cfg,
		conv:  NewConversation(),
		stuck: NewStuckDetector(cfg.StuckWindowSize, cfg.StuckThreshold),
	}
}

// Run executes the loop with a user prompt. It blocks until the LLM
// finishes (end_turn), the turn limit is reached, or the agent gets stuck.
// When ArchitectEditor.Enabled is true it runs the two-phase loop; otherwise
// it delegates to the single-phase path unchanged.
func (l *Loop) Run(ctx context.Context, userPrompt string) (*LoopResult, error) {
	if l.cfg.ArchitectEditor.Enabled {
		return l.runTwoPhase(ctx, userPrompt)
	}
	return l.runSinglePhase(ctx, userPrompt)
}

// runSinglePhase is the original Run implementation unchanged.
func (l *Loop) runSinglePhase(ctx context.Context, userPrompt string) (*LoopResult, error) {
	return l.runSinglePhaseWithPrompt(ctx, userPrompt)
}

// runSinglePhaseWithPrompt executes the agentic loop for a given prompt using
// the loop's current provider, tools, and system configuration.
func (l *Loop) runSinglePhaseWithPrompt(ctx context.Context, userPrompt string) (*LoopResult, error) {
	// Apply run-level timeout
	if l.cfg.RunTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, l.cfg.RunTimeout)
		defer cancel()
	}

	l.conv.AddUser(userPrompt)

	// Record initial user message for checkpoint.
	if l.cfg.Checkpoint != nil {
		_ = l.cfg.Checkpoint.RecordMessage(checkpoint.EventUserMessage, userPrompt)
	}

	// Fire on_agent_start hook.
	l.cfg.Hooks.RunOnAgentStart(l.cfg.System, userPrompt)

	// Inject matched microagents into the system prompt.
	system := l.cfg.System
	if l.cfg.MicroagentLoader != nil {
		agents, _ := l.cfg.MicroagentLoader.LoadAll()
		if len(agents) > 0 {
			evalCtx := microagent.EvalContext{UserMessage: userPrompt}
			matches := microagent.Match(agents, evalCtx)
			if injection := microagent.Inject(matches, 5); injection != "" {
				system = system + "\n\n" + injection
			}
		}
	}

	var totalTokens provider.TokenUsage
	turnCount := 0
	reflectionRetries := 0
	metrics := newAgentMetrics()
	startTime := time.Now()

	toolDefs := l.buildToolDefs()

	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("loop cancelled: %w", err)
		}

		turnCount++
		if turnCount > l.cfg.MaxTurns {
			return nil, fmt.Errorf("max turns (%d) exceeded", l.cfg.MaxTurns)
		}

		messages := l.conv.Messages()

		// Pre-call context window check: apply cascade if messages exceed limit.
		if cfw := l.cfg.ContextFallback; cfw.ContextWindow > 0 {
			minOut := cfw.MinOutputTokens
			if minOut <= 0 {
				minOut = 1000
			}
			if !provider.FitsInContext(messages, cfw.ContextWindow, minOut) {
				messages = l.applyContextFallback(ctx, messages, cfw, minOut)
				l.stuck.RecordCondensation()
			}
		}

		chatReq := provider.ChatRequest{
			Model:     l.cfg.Model,
			System:    system,
			Messages:  messages,
			Tools:     toolDefs,
			MaxTokens: l.cfg.MaxTokens,
		}

		var resp *provider.ChatResponse
		var err error

		// Fire before_model_call hook.
		l.cfg.Hooks.RunBeforeModelCall(l.cfg.System, l.cfg.Model, turnCount)

		// Use streaming when available and delta callback or EventStream is configured.
		if sp, ok := l.cfg.Provider.(provider.StreamProvider); ok && (l.cfg.OnTextDelta != nil || l.cfg.EventStream != nil) {
			resp, err = sp.ChatStream(ctx, chatReq, func(event provider.StreamEvent) {
				if event.Type == "text_delta" {
					if l.cfg.OnTextDelta != nil {
						l.cfg.OnTextDelta(event.TextDelta)
					}
					if l.cfg.EventStream != nil {
						l.cfg.EventStream.Emit(StreamEvent{
							Kind:      EventTurnStart,
							AgentName: l.cfg.Model,
							SessionID: l.cfg.SessionID,
							Message:   event.TextDelta,
						})
					}
				}
			})
		} else {
			resp, err = l.cfg.Provider.Chat(ctx, chatReq)
		}
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) && l.cfg.MaxConsecutiveTimeouts > 0 {
				l.consecutiveTimeouts++
				if l.consecutiveTimeouts >= l.cfg.MaxConsecutiveTimeouts {
					// Suspend if a channel is wired; otherwise abort.
					if l.cfg.SuspendCh != nil && l.cfg.ResumeCh != nil {
						cpID := ""
						if l.cfg.Checkpoint != nil {
							_ = l.cfg.Checkpoint.RecordSuspend(checkpoint.SuspendReasonError)
							cpID, _ = l.cfg.Checkpoint.CreateCheckpoint("auto-suspend", nil)
						}
						sig := SuspendSignal{
							CheckpointID: cpID,
							Reason:       checkpoint.SuspendReasonError,
							Preview:      fmt.Sprintf("Stuck after %d consecutive timeouts. Provide guidance to continue.", l.cfg.MaxConsecutiveTimeouts),
						}
						select {
						case l.cfg.SuspendCh <- sig:
						case <-ctx.Done():
							return nil, ctx.Err()
						}
						select {
						case <-ctx.Done():
							return nil, ctx.Err()
						case humanInput, ok := <-l.cfg.ResumeCh:
							if !ok {
								return nil, fmt.Errorf("resume channel closed during suspend")
							}
							if l.cfg.Checkpoint != nil {
								_ = l.cfg.Checkpoint.RecordResume(humanInput)
							}
							l.conv.AddUser("[Human guidance]: " + humanInput)
							l.consecutiveTimeouts = 0
							continue
						}
					}
					return nil, fmt.Errorf("chat turn %d: %d consecutive timeouts — aborting: %w",
						turnCount, l.consecutiveTimeouts, err)
				}
				// Below threshold: retry on next turn.
				continue
			}
			return nil, fmt.Errorf("chat turn %d: %w", turnCount, err)
		}
		l.consecutiveTimeouts = 0

		// Fire after_model_call hook.
		l.cfg.Hooks.RunAfterModelCall(l.cfg.System, l.cfg.Model, turnCount, resp.TokensUsed.TotalTokens)

		totalTokens.PromptTokens += resp.TokensUsed.PromptTokens
		totalTokens.CompletionTokens += resp.TokensUsed.CompletionTokens
		totalTokens.TotalTokens += resp.TokensUsed.TotalTokens
		totalTokens.CacheReadTokens += resp.TokensUsed.CacheReadTokens
		totalTokens.CacheWriteTokens += resp.TokensUsed.CacheWriteTokens

		// Extract inline summary before displaying content to the user.
		if l.cfg.InlineSummary && resp.Message.Content != "" {
			if summaryText, cleaned := ExtractInlineSummary(resp.Message.Content); summaryText != "" {
				resp.Message.Content = cleaned
				if l.cfg.OnTurnSummary != nil {
					l.cfg.OnTurnSummary(turnCount-1, summaryText)
				}
			}
		}

		// Add assistant message to history
		l.conv.AddAssistant(resp.Message)

		// Fire text callback for any text content
		if resp.Message.Content != "" && l.cfg.OnText != nil {
			l.cfg.OnText(resp.Message.Content)
		}
		if resp.Message.Content != "" && l.cfg.EventStream != nil {
			l.cfg.EventStream.Emit(StreamEvent{
				Kind:      EventTurnEnd,
				AgentName: l.cfg.Model,
				SessionID: l.cfg.SessionID,
				Message:   resp.Message.Content,
				Step:      turnCount,
			})
		}

		// Record assistant message for checkpoint.
		if l.cfg.Checkpoint != nil && resp.Message.Content != "" {
			_ = l.cfg.Checkpoint.RecordMessage(checkpoint.EventAssistant, resp.Message.Content)
		}

		// No tool calls → agent is done; run reflection before returning
		if resp.StopReason != "tool_use" || len(resp.Message.ToolCalls) == 0 {
			// Record text-only turn for monologue stuck detection.
			l.stuck.RecordTextTurn()

			var failedPhase string
			if l.cfg.Reflector != nil {
				feedback, phaseResults, allPassed := l.cfg.Reflector.RunPhases(ctx)
				if !allPassed {
					// Determine the failed phase name.
					for _, pr := range phaseResults {
						if !pr.Passed {
							failedPhase = pr.PhaseName
							break
						}
					}
					if reflectionRetries < l.cfg.Reflector.MaxRetries() {
						reflectionRetries++
						metrics.ReflectionCount++
						l.conv.AddUser(feedback)
						continue // feed error back for a new iteration
					}
				}
			}

			metrics.TurnCount = turnCount
			metrics.TokensUsed = totalTokens
			metrics.Duration = time.Since(startTime)
			metrics.CostUSD = provider.ComputeCostUSD(totalTokens, l.cfg.Model)
			metrics.computePressure(l.cfg.MaxTokens)
			slog.Info("agent_loop_completed",
				"turns", turnCount,
				"prompt_tokens", totalTokens.PromptTokens,
				"completion_tokens", totalTokens.CompletionTokens,
				"cache_read_tokens", totalTokens.CacheReadTokens,
				"cache_write_tokens", totalTokens.CacheWriteTokens,
				"cost_usd", metrics.CostUSD,
				"duration_ms", metrics.Duration.Milliseconds(),
				"context_pressure", metrics.ContextWindowPressure,
				"model", l.cfg.Model,
			)
			l.cfg.Hooks.RunOnAgentDone(l.cfg.System, turnCount, "")
			if l.cfg.EventStream != nil {
				l.cfg.EventStream.Emit(StreamEvent{
					Kind:      EventDone,
					AgentName: l.cfg.Model,
					SessionID: l.cfg.SessionID,
					Step:      turnCount,
				})
			}
			return &LoopResult{
				FinalText:         resp.Message.Content,
				TurnCount:         turnCount,
				TokensUsed:        totalTokens,
				Metrics:           metrics,
				FailedPhase:       failedPhase,
				ReflectionRetries: reflectionRetries,
			}, nil
		}

		// Execute tools
		for _, tc := range resp.Message.ToolCalls {
			metrics.recordToolCall(tc.Name)

			if l.cfg.OnToolCall != nil {
				l.cfg.OnToolCall(tc)
			}
			if l.cfg.EventStream != nil {
				l.cfg.EventStream.Emit(StreamEvent{
					Kind:      EventToolCall,
					AgentName: l.cfg.Model,
					SessionID: l.cfg.SessionID,
					ToolName:  tc.Name,
					ToolInput: string(tc.Input),
					Step:      turnCount,
				})
			}

			// Record tool call event for checkpoint time-travel.
			if l.cfg.Checkpoint != nil {
				_ = l.cfg.Checkpoint.RecordToolCall(tc.Name, tc.Input)
			}

			// Fire before_tool_call hook.
			l.cfg.Hooks.RunBeforeToolCall(l.cfg.System, tc.Name, tc.Input)

			result := l.executeTool(ctx, tc)

			// Track consecutive timeouts on tool calls
			if result.IsError && errors.Is(ctx.Err(), context.DeadlineExceeded) {
				l.consecutiveTimeouts++
				if l.cfg.MaxConsecutiveTimeouts > 0 && l.consecutiveTimeouts >= l.cfg.MaxConsecutiveTimeouts {
					return nil, fmt.Errorf("tool %q: %d consecutive timeouts — aborting",
						tc.Name, l.consecutiveTimeouts)
				}
			} else {
				l.consecutiveTimeouts = 0
			}

			l.conv.AddToolResult(tc.ID, result.Content, result.IsError)

			// Record tool result and file-modified events.
			if l.cfg.Checkpoint != nil {
				_ = l.cfg.Checkpoint.RecordToolResult(tc.Name, result.Content, result.IsError)
				if !result.IsError && isFileMutatingTool(tc.Name) {
					if path := extractPathArg(tc.Input); path != "" {
						_ = l.cfg.Checkpoint.RecordFileModified(path)
					}
				}
			}

			// Fire after_tool_call hook.
			l.cfg.Hooks.RunAfterToolCall(l.cfg.System, tc.Name, tc.Input, result.Content, result.IsError)

			// Track for stuck detection
			inputStr := ""
			if tc.Input != nil {
				inputStr = string(tc.Input)
			}
			l.stuck.RecordResult(tc.Name, inputStr, result.IsError)

			if l.cfg.OnToolResult != nil {
				l.cfg.OnToolResult(tc.ID, tc.Name, result.Content, result.IsError)
			}
			if l.cfg.EventStream != nil {
				l.cfg.EventStream.Emit(StreamEvent{
					Kind:       EventToolResult,
					AgentName:  l.cfg.Model,
					SessionID:  l.cfg.SessionID,
					ToolName:   tc.Name,
					ToolOutput: result.Content,
					Step:       turnCount,
				})
			}
		}

		// Real-time steering: inject any pending user guidance before next LLM call.
		l.drainSteer()

		// Granular interrupt: pause cleanly if Interrupt fired, wait for Resume.
		if l.cfg.Control != nil {
			if interrupted := l.checkInterrupt(ctx); interrupted {
				return &LoopResult{
					TurnCount:  turnCount,
					TokensUsed: totalTokens,
					Metrics:    metrics,
				}, fmt.Errorf("loop interrupted")
			}
		}

		// Check stuck after processing all tool calls in this turn
		if pattern, stuck := l.stuck.CheckAll(); stuck {
			metrics.StuckCount++
			stuckErr := fmt.Errorf("agent stuck: same tool+input repeated %d times in last %d calls — aborting",
				l.cfg.StuckThreshold, l.cfg.StuckWindowSize)
			if l.cfg.Memory != nil {
				content := fmt.Sprintf("stuck detected: threshold=%d window=%d pattern=%s", l.cfg.StuckThreshold, l.cfg.StuckWindowSize, pattern)
				_ = l.cfg.Memory.Write(memory.Entry{
					Agent:   l.cfg.Model,
					Type:    "issue",
					Content: content,
				})
			}
			return &LoopResult{
				TurnCount:    turnCount,
				TokensUsed:   totalTokens,
				Metrics:      metrics,
				StuckPattern: pattern,
			}, stuckErr
		}
	}
}

// runTwoPhase executes the architect/editor two-phase loop.
//
// Phase 1 (architect): read-only tools, capped at MaxArchitectTurns.
// Produces a WorkPlan embedded in the response as <work_plan>...</work_plan>.
// If the architect does not produce a plan we fall back to single-phase.
//
// Phase 2 (editor): full tool registry, capped at MaxEditorTurns.
// Receives the rendered work plan as its first user message.
//
// The two results are merged and returned as a single LoopResult.
func (l *Loop) runTwoPhase(ctx context.Context, userPrompt string) (*LoopResult, error) {
	archResult, plan, err := l.runArchitectPhase(ctx, userPrompt)
	if err != nil {
		return archResult, err
	}
	if plan == nil {
		// Architect didn't produce a work plan — fall back to single phase.
		return l.runSinglePhaseWithPrompt(ctx, userPrompt)
	}

	editorPrompt := RenderWorkPlan(plan)
	editorResult, err := l.runEditorPhase(ctx, editorPrompt)
	if err != nil {
		return editorResult, err
	}

	// Merge token and turn counts from both phases.
	editorResult.TurnCount += archResult.TurnCount
	editorResult.TokensUsed.PromptTokens += archResult.TokensUsed.PromptTokens
	editorResult.TokensUsed.CompletionTokens += archResult.TokensUsed.CompletionTokens
	editorResult.TokensUsed.TotalTokens += archResult.TokensUsed.TotalTokens
	return editorResult, nil
}

// runArchitectPhase runs a read-only sub-loop for the reasoning phase and
// returns the LoopResult plus the extracted WorkPlan (nil if none produced).
func (l *Loop) runArchitectPhase(ctx context.Context, userPrompt string) (*LoopResult, *WorkPlan, error) {
	ae := l.cfg.ArchitectEditor

	// Build read-only tool registry (only: read, glob, grep, ls, think).
	archTools := tool.NewRegistry()
	if l.cfg.Tools != nil {
		for _, t := range l.cfg.Tools.All() {
			switch t.Name() {
			case "read", "glob", "grep", "ls", "think":
				archTools.Register(t)
			}
		}
	}

	// Determine model: explicit override takes precedence over role.
	archModel := ae.ArchitectModel
	if archModel == "" {
		archModel = l.cfg.Model
	}

	archCfg := LoopConfig{
		Provider:               l.cfg.Provider,
		Tools:                  archTools,
		System:                 l.cfg.System + ArchitectSystemSuffix,
		Model:                  archModel,
		MaxTurns:               ae.architectMaxTurns(),
		MaxTokens:              l.cfg.MaxTokens,
		RunTimeout:             l.cfg.RunTimeout,
		StuckWindowSize:        l.cfg.StuckWindowSize,
		StuckThreshold:         l.cfg.StuckThreshold,
		MaxConsecutiveTimeouts: l.cfg.MaxConsecutiveTimeouts,
		OnText:                 l.cfg.OnText,
		OnTextDelta:            l.cfg.OnTextDelta,
		OnToolCall:             l.cfg.OnToolCall,
		OnToolResult:           l.cfg.OnToolResult,
	}

	archLoop := NewLoop(archCfg)
	result, err := archLoop.runSinglePhaseWithPrompt(ctx, userPrompt)
	if err != nil {
		return result, nil, err
	}

	plan := ExtractWorkPlan(result.FinalText)
	return result, plan, nil
}

// runEditorPhase runs the full-tools sub-loop for the editing phase.
func (l *Loop) runEditorPhase(ctx context.Context, editorPrompt string) (*LoopResult, error) {
	ae := l.cfg.ArchitectEditor

	// Determine model: explicit override takes precedence over role.
	editorModel := ae.EditorModel
	if editorModel == "" {
		editorModel = l.cfg.Model
	}

	editorCfg := LoopConfig{
		Provider:               l.cfg.Provider,
		Tools:                  l.cfg.Tools,
		GuardedTools:           l.cfg.GuardedTools,
		System:                 l.cfg.System + EditorSystemSuffix,
		Model:                  editorModel,
		MaxTurns:               ae.editorMaxTurns(),
		MaxTokens:              l.cfg.MaxTokens,
		RunTimeout:             l.cfg.RunTimeout,
		StuckWindowSize:        l.cfg.StuckWindowSize,
		StuckThreshold:         l.cfg.StuckThreshold,
		MaxConsecutiveTimeouts: l.cfg.MaxConsecutiveTimeouts,
		Memory:                 l.cfg.Memory,
		Reflector:              l.cfg.Reflector,
		ContextFallback:        l.cfg.ContextFallback,
		OnText:                 l.cfg.OnText,
		OnTextDelta:            l.cfg.OnTextDelta,
		OnToolCall:             l.cfg.OnToolCall,
		OnToolResult:           l.cfg.OnToolResult,
	}

	editorLoop := NewLoop(editorCfg)
	return editorLoop.runSinglePhaseWithPrompt(ctx, editorPrompt)
}

func (l *Loop) buildToolDefs() []provider.ToolDef {
	if l.cfg.Tools == nil {
		return nil
	}
	tools := l.cfg.Tools.All()
	defs := make([]provider.ToolDef, len(tools))
	for i, t := range tools {
		defs[i] = provider.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: t.InputSchema(),
		}
	}
	return defs
}

// applyContextFallback applies the context window overflow cascade:
// 1. Try SummarizeKeepTail (if summary provider available).
// 2. Try PruneMessages as last resort.
// Always logs what happened.
func (l *Loop) applyContextFallback(ctx context.Context, messages []provider.Message, cfw ContextFallbackConfig, minOutputTokens int) []provider.Message {
	original := len(messages)

	// Layer 1: Summarization via cheap LLM.
	if cfw.SummaryProvider != nil {
		model := cfw.SummaryModel
		if model == "" {
			model = l.cfg.Model
		}
		summarized, err := SummarizeKeepTail(ctx, messages, cfw.ContextWindow, cfw.SummaryProvider, model, 0)
		if err == nil && provider.FitsInContext(summarized, cfw.ContextWindow, minOutputTokens) {
			slog.Info("context_window_summarized",
				"original_messages", original,
				"after", len(summarized),
				"removed", original-len(summarized),
			)
			return summarized
		}
		// Summarization reduced size but still doesn't fit — use as input for pruning.
		if err == nil && len(summarized) < len(messages) {
			messages = summarized
		}
	}

	// Layer 2: Intelligent pruning as last resort.
	pruned := provider.PruneMessages(messages, cfw.ContextWindow, minOutputTokens)
	slog.Warn("context_window_pruned",
		"original_messages", original,
		"after", len(pruned),
		"removed", original-len(pruned),
	)
	return pruned
}

func (l *Loop) executeTool(ctx context.Context, tc provider.ToolCall) *tool.Result {
	if l.cfg.Tools == nil {
		return tool.ErrorResult("no tools available")
	}

	input := tc.Input
	if input == nil {
		input = json.RawMessage("{}")
	}

	start := time.Now()
	decision := "allow"
	var execErr string

	auditLog := func(result *tool.Result) *tool.Result {
		if l.cfg.AuditLogger != nil {
			errStr := execErr
			if result.IsError && errStr == "" {
				errStr = result.Content
			}
			l.cfg.AuditLogger.Log(ctx, audit.Entry{
				Timestamp:  time.Now(),
				AgentName:  l.cfg.Model,
				ToolName:   tc.Name,
				ToolInput:  input,
				Decision:   decision,
				DurationMs: time.Since(start).Milliseconds(),
				Error:      errStr,
			})
		}
		return result
	}

	// Resolve permission level: use GuardedTools if available, else rules from config or defaults.
	var permLevel tool.PermissionLevel
	if l.cfg.GuardedTools != nil {
		permLevel = l.cfg.GuardedTools.CheckLevel(tc.Name)
	} else {
		rules := l.cfg.PermissionRules
		if len(rules) == 0 {
			rules = tool.DefaultPermissionRules()
		}
		checker := tool.NewPermissionChecker(rules)
		permLevel = checker.Check(tc.Name)
	}

	// Enforce deny immediately.
	if permLevel == tool.PermDeny {
		decision = "deny"
		return auditLog(tool.ErrorResult(fmt.Sprintf("tool %q is denied by permission rules", tc.Name)))
	}

	// Ask-level: request user approval with a rich, informative preview.
	// Exception: bash commands classified as "low" risk are auto-approved —
	// this covers read-only shell commands (pwd, echo, git status, git log, etc.)
	// that would otherwise cause unnecessary interruptions.
	if permLevel == tool.PermAsk && l.cfg.PermissionCallback != nil {
		req := buildApprovalRequest(l.cfg.Model, tc.Name, input)
		if req.RiskLevel == "low" {
			decision = "allow" // auto-approve safe commands silently
		} else {
			ad, err := l.cfg.PermissionCallback.RequestApproval(ctx, req)
			if err != nil || ad == ApprovalDeny {
				decision = "ask-denied"
				return auditLog(tool.ErrorResult(fmt.Sprintf("tool %q denied", tc.Name)))
			}
			decision = "ask-approved"
		}
	}

	// Execute via GuardedRegistry or plain registry.
	if l.cfg.GuardedTools != nil {
		result, err := l.cfg.GuardedTools.Execute(ctx, tc.Name, input)
		if err != nil {
			execErr = err.Error()
			return auditLog(tool.ErrorResult(fmt.Sprintf("tool error: %v", err)))
		}
		return auditLog(result)
	}

	t, ok := l.cfg.Tools.Get(tc.Name)
	if !ok {
		decision = "deny"
		return auditLog(tool.ErrorResult(fmt.Sprintf("unknown tool: %s", tc.Name)))
	}
	result, err := t.Execute(ctx, input)
	if err != nil {
		execErr = err.Error()
		return auditLog(tool.ErrorResult(fmt.Sprintf("tool error: %v", err)))
	}
	return auditLog(result)
}

// isFileMutatingTool returns true for tools that modify files on disk.
func isFileMutatingTool(name string) bool {
	switch name {
	case "write", "edit", "patch", "undo_edit":
		return true
	}
	return false
}

// extractPathArg extracts the "path" field from a JSON tool input, if present.
func extractPathArg(input json.RawMessage) string {
	if input == nil {
		return ""
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(input, &m); err != nil {
		return ""
	}
	raw, ok := m["path"]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}
