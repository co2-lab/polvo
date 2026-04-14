package agent

import "github.com/co2-lab/polvo/internal/tool"

// AutonomyMode controls how much human oversight an agent requires.
type AutonomyMode string

const (
	AutonomyFull       AutonomyMode = "full"       // executes and applies without approval
	AutonomySupervised AutonomyMode = "supervised" // write/edit/bash require confirmation
	AutonomyPlan       AutonomyMode = "plan"        // read-only: no writes, no bash
)

// ReadOnlyTools is the set of tools allowed in plan mode.
var readOnlyToolNames = map[string]bool{
	"read": true, "glob": true, "grep": true, "ls": true, "think": true,
	"memory_read": true, "web_fetch": true, "web_search": true,
}

// FilterRegistryForMode returns a new Registry containing only the tools
// permitted by the given autonomy mode.
func FilterRegistryForMode(reg *tool.Registry, mode AutonomyMode) *tool.Registry {
	if mode != AutonomyPlan {
		return reg // supervised and full use the full registry (guarded by permissions)
	}
	filtered := tool.NewRegistry()
	for _, t := range reg.All() {
		if readOnlyToolNames[t.Name()] {
			filtered.Register(t)
		}
	}
	return filtered
}

// DefaultPermissionsForMode returns permission rules appropriate for the mode.
func DefaultPermissionsForMode(mode AutonomyMode) []tool.PermissionRule {
	switch mode {
	case AutonomyFull:
		return []tool.PermissionRule{
			{Tool: "read", Level: tool.PermAllow},
			{Tool: "glob", Level: tool.PermAllow},
			{Tool: "grep", Level: tool.PermAllow},
			{Tool: "ls", Level: tool.PermAllow},
			{Tool: "think", Level: tool.PermAllow},
			{Tool: "memory_read", Level: tool.PermAllow},
			{Tool: "memory_write", Level: tool.PermAllow},
			{Tool: "write", Level: tool.PermAllow},
			{Tool: "edit", Level: tool.PermAllow},
			{Tool: "bash", Level: tool.PermAllow},
			{Tool: "web_fetch", Level: tool.PermAllow},
			{Tool: "web_search", Level: tool.PermAllow},
			{Tool: "patch", Level: tool.PermAllow},
		}
	case AutonomyPlan:
		return []tool.PermissionRule{
			{Tool: "read", Level: tool.PermAllow},
			{Tool: "glob", Level: tool.PermAllow},
			{Tool: "grep", Level: tool.PermAllow},
			{Tool: "ls", Level: tool.PermAllow},
			{Tool: "think", Level: tool.PermAllow},
			{Tool: "memory_read", Level: tool.PermAllow},
			{Tool: "web_fetch", Level: tool.PermAllow},
			{Tool: "web_search", Level: tool.PermAllow},
		}
	default: // supervised
		return tool.DefaultPermissionRules()
	}
}
