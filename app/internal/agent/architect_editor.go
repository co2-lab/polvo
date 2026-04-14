package agent

import "github.com/co2-lab/polvo/internal/provider"

// ArchitectEditorConfig enables the two-phase architect/editor loop.
type ArchitectEditorConfig struct {
	Enabled bool

	// ArchitectRole is the model role used for the reasoning phase (default: RolePrimary).
	ArchitectRole provider.ModelRole
	// EditorRole is the model role used for the editing phase (default: RoleReview = cheaper).
	EditorRole provider.ModelRole

	// ArchitectModel / EditorModel are explicit model overrides (take precedence over roles).
	ArchitectModel string
	EditorModel    string

	// MaxArchitectTurns caps reasoning turns (default: 3).
	MaxArchitectTurns int
	// MaxEditorTurns caps editing turns (default: 10).
	MaxEditorTurns int
}

func (c *ArchitectEditorConfig) architectMaxTurns() int {
	if c.MaxArchitectTurns <= 0 {
		return 3
	}
	return c.MaxArchitectTurns
}

func (c *ArchitectEditorConfig) editorMaxTurns() int {
	if c.MaxEditorTurns <= 0 {
		return 10
	}
	return c.MaxEditorTurns
}
