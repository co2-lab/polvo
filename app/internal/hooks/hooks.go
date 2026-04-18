// Package hooks implements the Polvo agent hooks system.
// Hooks are shell scripts that are executed at specific points in the agent
// lifecycle and receive JSON-encoded event data on stdin.
package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

// Config holds the agent hooks configuration.
type Config struct {
	// BeforeToolCall is executed before every tool call.
	// Receives: {"tool": "...", "input": {...}, "agent": "..."}
	BeforeToolCall []string `koanf:"before_tool_call"`

	// AfterToolCall is executed after every tool call.
	// Receives: {"tool": "...", "input": {...}, "result": "...", "is_error": false, "agent": "..."}
	AfterToolCall []string `koanf:"after_tool_call"`

	// BeforeModelCall is executed before every LLM call.
	// Receives: {"model": "...", "turn": N, "agent": "..."}
	BeforeModelCall []string `koanf:"before_model_call"`

	// AfterModelCall is executed after every LLM call.
	// Receives: {"model": "...", "turn": N, "tokens_used": N, "agent": "..."}
	AfterModelCall []string `koanf:"after_model_call"`

	// OnAgentStart is executed when an agent run begins.
	// Receives: {"agent": "...", "prompt": "..."}
	OnAgentStart []string `koanf:"on_agent_start"`

	// OnAgentDone is executed when an agent run completes.
	// Receives: {"agent": "...", "turns": N, "error": "..."}
	OnAgentDone []string `koanf:"on_agent_done"`
}

// IsEmpty returns true when no hooks are configured.
func (c *Config) IsEmpty() bool {
	return len(c.BeforeToolCall) == 0 &&
		len(c.AfterToolCall) == 0 &&
		len(c.BeforeModelCall) == 0 &&
		len(c.AfterModelCall) == 0 &&
		len(c.OnAgentStart) == 0 &&
		len(c.OnAgentDone) == 0
}

// Runner executes hooks asynchronously, non-blocking.
type Runner struct {
	cfg     *Config
	timeout time.Duration
}

// NewRunner creates a Runner. timeout is the max duration for each hook script.
func NewRunner(cfg *Config, timeout time.Duration) *Runner {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Runner{cfg: cfg, timeout: timeout}
}

// RunBeforeToolCall fires the before_tool_call hooks asynchronously.
func (r *Runner) RunBeforeToolCall(agentName, toolName string, input json.RawMessage) {
	if r == nil || len(r.cfg.BeforeToolCall) == 0 {
		return
	}
	payload := map[string]any{
		"agent": agentName,
		"tool":  toolName,
		"input": input,
	}
	r.runAsync("before_tool_call", r.cfg.BeforeToolCall, payload)
}

// RunAfterToolCall fires the after_tool_call hooks asynchronously.
func (r *Runner) RunAfterToolCall(agentName, toolName string, input json.RawMessage, result string, isError bool) {
	if r == nil || len(r.cfg.AfterToolCall) == 0 {
		return
	}
	payload := map[string]any{
		"agent":    agentName,
		"tool":     toolName,
		"input":    input,
		"result":   result,
		"is_error": isError,
	}
	r.runAsync("after_tool_call", r.cfg.AfterToolCall, payload)
}

// RunBeforeModelCall fires the before_model_call hooks asynchronously.
func (r *Runner) RunBeforeModelCall(agentName, model string, turn int) {
	if r == nil || len(r.cfg.BeforeModelCall) == 0 {
		return
	}
	payload := map[string]any{
		"agent": agentName,
		"model": model,
		"turn":  turn,
	}
	r.runAsync("before_model_call", r.cfg.BeforeModelCall, payload)
}

// RunAfterModelCall fires the after_model_call hooks asynchronously.
func (r *Runner) RunAfterModelCall(agentName, model string, turn, tokensUsed int) {
	if r == nil || len(r.cfg.AfterModelCall) == 0 {
		return
	}
	payload := map[string]any{
		"agent":       agentName,
		"model":       model,
		"turn":        turn,
		"tokens_used": tokensUsed,
	}
	r.runAsync("after_model_call", r.cfg.AfterModelCall, payload)
}

// RunOnAgentStart fires the on_agent_start hooks asynchronously.
func (r *Runner) RunOnAgentStart(agentName, prompt string) {
	if r == nil || len(r.cfg.OnAgentStart) == 0 {
		return
	}
	payload := map[string]any{
		"agent":  agentName,
		"prompt": prompt,
	}
	r.runAsync("on_agent_start", r.cfg.OnAgentStart, payload)
}

// RunOnAgentDone fires the on_agent_done hooks asynchronously.
func (r *Runner) RunOnAgentDone(agentName string, turns int, errMsg string) {
	if r == nil || len(r.cfg.OnAgentDone) == 0 {
		return
	}
	payload := map[string]any{
		"agent": agentName,
		"turns": turns,
		"error": errMsg,
	}
	r.runAsync("on_agent_done", r.cfg.OnAgentDone, payload)
}

// runAsync runs each command in cmds in a separate goroutine.
func (r *Runner) runAsync(hookName string, cmds []string, payload map[string]any) {
	data, err := json.Marshal(payload)
	if err != nil {
		slog.Warn("hooks: failed to marshal payload", "hook", hookName, "error", err)
		return
	}

	for _, cmdStr := range cmds {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
			defer cancel()

			// Execute via shell to support pipes, env vars, etc.
			cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
			cmd.Stdin = bytes.NewReader(data)

			var out strings.Builder
			cmd.Stdout = &out
			cmd.Stderr = &out

			if err := cmd.Run(); err != nil {
				slog.Warn("hooks: hook failed",
					"hook", hookName,
					"cmd", cmdStr,
					"error", err,
					"output", out.String(),
				)
			}
		}()
	}
}
