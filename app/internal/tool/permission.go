package tool

import (
	"context"
	"encoding/json"
	"fmt"
)

// PermissionLevel controls tool execution behavior.
type PermissionLevel string

const (
	PermAllow PermissionLevel = "allow"
	PermAsk   PermissionLevel = "ask"
	PermDeny  PermissionLevel = "deny"
)

// PermissionRule maps a tool name to a permission level.
type PermissionRule struct {
	Tool  string          `koanf:"tool" json:"tool"`
	Level PermissionLevel `koanf:"level" json:"level"`
}

// AskFunc is called when a tool requires user confirmation.
// It should return true if the user approves the execution.
type AskFunc func(toolName string, input json.RawMessage) (bool, error)

// PermissionChecker evaluates whether a tool can run.
type PermissionChecker struct {
	rules map[string]PermissionLevel
}

// NewPermissionChecker creates a checker from rules.
func NewPermissionChecker(rules []PermissionRule) *PermissionChecker {
	m := make(map[string]PermissionLevel)
	for _, r := range rules {
		m[r.Tool] = r.Level
	}
	return &PermissionChecker{rules: m}
}

// DefaultPermissionRules returns sensible defaults aligned with industry practice:
//
//   - Read-only filesystem tools: auto-allowed (no side-effects)
//   - Exploration tools (web, memory, think): auto-allowed (external reads, no mutation)
//   - File mutations (write, edit): ask — developer should review changes
//   - Shell execution (bash): ask — but low-risk commands are auto-approved by the loop
//
// This matches the model used by Claude Code, Cline, and Aider.
func DefaultPermissionRules() []PermissionRule {
	return []PermissionRule{
		// Filesystem reads — always safe
		{Tool: "read", Level: PermAllow},
		{Tool: "glob", Level: PermAllow},
		{Tool: "grep", Level: PermAllow},
		{Tool: "ls", Level: PermAllow},
		// Exploration — external reads, no local mutation
		{Tool: "web_fetch", Level: PermAllow},
		{Tool: "web_search", Level: PermAllow},
		// Internal agent tools — no user-visible side effects
		{Tool: "think", Level: PermAllow},
		{Tool: "memory_read", Level: PermAllow},
		{Tool: "memory_write", Level: PermAllow},
		// File mutations — ask, show preview
		{Tool: "write", Level: PermAsk},
		{Tool: "edit", Level: PermAsk},
		{Tool: "patch", Level: PermAsk},
		// Shell execution — ask, but loop auto-approves low-risk commands
		{Tool: "bash", Level: PermAsk},
	}
}

// Check returns the permission level for a tool.
func (pc *PermissionChecker) Check(toolName string) PermissionLevel {
	if level, ok := pc.rules[toolName]; ok {
		return level
	}
	return PermAsk // default to ask for unknown tools
}

// GuardedRegistry wraps a Registry with permission checks.
type GuardedRegistry struct {
	inner   *Registry
	checker *PermissionChecker
	askFn   AskFunc
}

// NewGuardedRegistry creates a permission-guarded registry.
func NewGuardedRegistry(inner *Registry, rules []PermissionRule, askFn AskFunc) *GuardedRegistry {
	return &GuardedRegistry{
		inner:   inner,
		checker: NewPermissionChecker(rules),
		askFn:   askFn,
	}
}

// Get returns a tool by name (delegates to inner registry).
func (g *GuardedRegistry) Get(name string) (Tool, bool) {
	return g.inner.Get(name)
}

// CheckLevel returns the permission level for the named tool without executing it.
// This allows callers to inspect whether a tool requires approval before calling Execute.
func (g *GuardedRegistry) CheckLevel(name string) PermissionLevel {
	return g.checker.Check(name)
}

// All returns all tools (delegates to inner registry).
func (g *GuardedRegistry) All() []Tool {
	return g.inner.All()
}

// Execute checks permissions then executes the tool.
func (g *GuardedRegistry) Execute(ctx context.Context, name string, input json.RawMessage) (*Result, error) {
	t, ok := g.inner.Get(name)
	if !ok {
		return ErrorResult(fmt.Sprintf("unknown tool: %s", name)), nil
	}

	level := g.checker.Check(name)
	switch level {
	case PermDeny:
		return ErrorResult(fmt.Sprintf("tool %q is denied by permission rules", name)), nil
	case PermAsk:
		if g.askFn != nil {
			allowed, err := g.askFn(name, input)
			if err != nil {
				return ErrorResult(fmt.Sprintf("permission check error: %v", err)), nil
			}
			if !allowed {
				return ErrorResult(fmt.Sprintf("tool %q was denied by user", name)), nil
			}
		}
	}

	return t.Execute(ctx, input)
}
