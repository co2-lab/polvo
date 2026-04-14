package mcp

import "path"

// PermissionLevel represents an MCP tool permission outcome.
type PermissionLevel string

const (
	PermAllow PermissionLevel = "allow"
	PermAsk   PermissionLevel = "ask"
	PermDeny  PermissionLevel = "deny"
)

// PermissionEngine evaluates allow/ask/deny rules against MCP tool names.
// Precedence: deny wins over ask, ask wins over allow; default is ask.
type PermissionEngine struct {
	allow []string
	ask   []string
	deny  []string
}

// NewPermissionEngine creates a PermissionEngine from an MCPPermissions config.
func NewPermissionEngine(p MCPPermissions) *PermissionEngine {
	return &PermissionEngine{
		allow: p.Allow,
		ask:   p.Ask,
		deny:  p.Deny,
	}
}

// Evaluate returns the permission level that applies to toolName.
// Evaluation order: deny first → ask → allow → default (ask).
func (e *PermissionEngine) Evaluate(toolName string) PermissionLevel {
	if matchesAny(e.deny, toolName) {
		return PermDeny
	}
	if matchesAny(e.ask, toolName) {
		return PermAsk
	}
	if matchesAny(e.allow, toolName) {
		return PermAllow
	}
	return PermAsk // default
}

// matchesAny returns true if toolName matches any of the glob patterns.
// Uses path.Match which supports * and ? wildcards.
// The pattern "mcp__server__*" matches all tools for a server.
func matchesAny(patterns []string, toolName string) bool {
	for _, p := range patterns {
		if matched, err := path.Match(p, toolName); err == nil && matched {
			return true
		}
	}
	return false
}
