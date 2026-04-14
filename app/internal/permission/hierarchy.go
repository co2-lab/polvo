// Package permission implements a three-level permission hierarchy for tool execution.
package permission

// Level represents the permission scope.
type Level int

const (
	LevelSystem  Level = 0 // global defaults (binary-embedded or global config)
	LevelProject Level = 1 // .polvo/config.yaml in the repository root
	LevelSession Level = 2 // per-agent-run overrides supplied at runtime
)

// Decision is the result of a permission check.
type Decision int

const (
	DecisionAllow Decision = iota
	DecisionDeny
	DecisionAsk
)

// String returns the human-readable representation of a Decision.
func (d Decision) String() string {
	switch d {
	case DecisionAllow:
		return "allow"
	case DecisionDeny:
		return "deny"
	default:
		return "ask"
	}
}

// Rules holds the allow/deny/ask lists for one permission level.
// Each list contains tool names; the special value "all" is a wildcard that
// matches any tool name.
type Rules struct {
	Allow []string // tool names explicitly allowed, or "all"
	Deny  []string // tool names explicitly denied, or "all"
	Ask   []string // tool names that require interactive approval, or "all"
}

// Hierarchy resolves effective permissions across system/project/session levels.
//
// Rule precedence (evaluated in order):
//  1. Deny at ANY level → DecisionDeny  (deny wins globally)
//  2. Session Allow     → DecisionAllow
//  3. Project Allow     → DecisionAllow
//  4. System Allow      → DecisionAllow
//  5. Session Ask       → DecisionAsk
//  6. Project Ask       → DecisionAsk
//  7. System Ask        → DecisionAsk
//  8. Default           → DecisionAsk
type Hierarchy struct {
	levels [3]Rules
}

// NewHierarchy creates a Hierarchy with the provided rules per level.
func NewHierarchy(system, project, session Rules) *Hierarchy {
	return &Hierarchy{
		levels: [3]Rules{
			LevelSystem:  system,
			LevelProject: project,
			LevelSession: session,
		},
	}
}

// SetLevel updates the rules for the given level.
// Useful for applying runtime (session) overrides after construction.
func (h *Hierarchy) SetLevel(lvl Level, rules Rules) {
	h.levels[lvl] = rules
}

// Resolve returns the effective Decision for the given tool name.
func (h *Hierarchy) Resolve(tool string) Decision {
	// Step 1: deny at ANY level wins immediately.
	for _, r := range h.levels {
		if matchesRule(tool, r.Deny) {
			return DecisionDeny
		}
	}

	// Steps 2-4: allow wins in session → project → system order.
	for _, lvl := range []Level{LevelSession, LevelProject, LevelSystem} {
		if matchesRule(tool, h.levels[lvl].Allow) {
			return DecisionAllow
		}
	}

	// Steps 5-7: ask wins in session → project → system order.
	for _, lvl := range []Level{LevelSession, LevelProject, LevelSystem} {
		if matchesRule(tool, h.levels[lvl].Ask) {
			return DecisionAsk
		}
	}

	// Step 8: default is ask.
	return DecisionAsk
}

// matchesRule reports whether name appears in list or list contains "all".
func matchesRule(name string, list []string) bool {
	for _, entry := range list {
		if entry == "all" || entry == name {
			return true
		}
	}
	return false
}
