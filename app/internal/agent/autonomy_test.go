package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/co2-lab/polvo/internal/tool"
)

// buildTestRegistry creates a registry with a set of named tools for testing.
func buildTestRegistry(names ...string) *tool.Registry {
	reg := tool.NewRegistry()
	for _, name := range names {
		n := name // capture
		reg.Register(&autonomyTestTool{name: n})
	}
	return reg
}

// autonomyTestTool is a minimal tool.Tool implementation for autonomy tests.
type autonomyTestTool struct {
	name string
}

func (t *autonomyTestTool) Name() string                                                        { return t.name }
func (t *autonomyTestTool) Description() string                                                 { return "test tool " + t.name }
func (t *autonomyTestTool) InputSchema() json.RawMessage                                        { return json.RawMessage(`{}`) }
func (t *autonomyTestTool) Execute(_ context.Context, _ json.RawMessage) (*tool.Result, error) {
	return &tool.Result{Content: "ok"}, nil
}

func registryNames(reg *tool.Registry) map[string]bool {
	names := make(map[string]bool)
	for _, t := range reg.All() {
		names[t.Name()] = true
	}
	return names
}

func TestFilterRegistryForMode(t *testing.T) {
	allTools := []string{"read", "glob", "grep", "ls", "think", "memory_read", "web_fetch", "web_search", "write", "edit", "bash", "patch"}

	t.Run("full retorna registry original", func(t *testing.T) {
		reg := buildTestRegistry(allTools...)
		result := FilterRegistryForMode(reg, AutonomyFull)
		if result != reg {
			t.Error("expected same registry reference for full mode")
		}
	})

	t.Run("supervised retorna registry original", func(t *testing.T) {
		reg := buildTestRegistry(allTools...)
		result := FilterRegistryForMode(reg, AutonomySupervised)
		if result != reg {
			t.Error("expected same registry reference for supervised mode")
		}
	})

	t.Run("plan retorna apenas tools read-only", func(t *testing.T) {
		reg := buildTestRegistry(allTools...)
		result := FilterRegistryForMode(reg, AutonomyPlan)
		names := registryNames(result)

		readOnly := []string{"read", "glob", "grep", "ls", "think", "memory_read", "web_fetch", "web_search"}
		for _, name := range readOnly {
			if !names[name] {
				t.Errorf("expected %q in plan mode registry", name)
			}
		}

		banned := []string{"write", "edit", "bash", "patch"}
		for _, name := range banned {
			if names[name] {
				t.Errorf("expected %q to be absent in plan mode registry", name)
			}
		}
	})

	t.Run("plan com tool desconhecida ausente no resultado", func(t *testing.T) {
		reg := buildTestRegistry("read", "deploy")
		result := FilterRegistryForMode(reg, AutonomyPlan)
		names := registryNames(result)
		if names["deploy"] {
			t.Error("expected 'deploy' to be absent in plan mode registry")
		}
		if !names["read"] {
			t.Error("expected 'read' to be present in plan mode registry")
		}
	})

	t.Run("registry vazio com plan → resultado vazio sem panic", func(t *testing.T) {
		reg := tool.NewRegistry()
		result := FilterRegistryForMode(reg, AutonomyPlan)
		if len(result.All()) != 0 {
			t.Error("expected empty registry for plan mode with empty input")
		}
	})
}

func TestDefaultPermissionsForMode(t *testing.T) {
	t.Run("full: bash/write/edit têm PermAllow", func(t *testing.T) {
		rules := DefaultPermissionsForMode(AutonomyFull)
		if rules == nil {
			t.Fatal("expected non-nil rules for full mode")
		}
		perms := make(map[string]tool.PermissionLevel)
		for _, r := range rules {
			perms[r.Tool] = r.Level
		}
		for _, name := range []string{"bash", "write", "edit"} {
			if perms[name] != tool.PermAllow {
				t.Errorf("expected %q to be PermAllow in full mode, got %v", name, perms[name])
			}
		}
	})

	t.Run("plan: apenas read-only tools têm PermAllow", func(t *testing.T) {
		rules := DefaultPermissionsForMode(AutonomyPlan)
		if rules == nil {
			t.Fatal("expected non-nil rules for plan mode")
		}
		perms := make(map[string]tool.PermissionLevel)
		for _, r := range rules {
			perms[r.Tool] = r.Level
		}
		// Write tools should NOT be present (not in plan mode rules)
		for _, name := range []string{"bash", "write", "edit"} {
			if _, exists := perms[name]; exists {
				t.Errorf("expected %q to be absent in plan mode permissions", name)
			}
		}
		// Read-only tools should be PermAllow
		for _, name := range []string{"read", "glob", "grep", "ls"} {
			if perms[name] != tool.PermAllow {
				t.Errorf("expected %q to be PermAllow in plan mode, got %v", name, perms[name])
			}
		}
	})

	t.Run("supervised: bash/write/edit têm PermAsk", func(t *testing.T) {
		rules := DefaultPermissionsForMode(AutonomySupervised)
		if rules == nil {
			t.Fatal("expected non-nil rules for supervised mode")
		}
		perms := make(map[string]tool.PermissionLevel)
		for _, r := range rules {
			perms[r.Tool] = r.Level
		}
		for _, name := range []string{"bash", "write", "edit"} {
			if perms[name] != tool.PermAsk {
				t.Errorf("expected %q to be PermAsk in supervised mode, got %v", name, perms[name])
			}
		}
	})

	t.Run("modo desconhecido → retorna supervised (sem panic, sem nil)", func(t *testing.T) {
		rules := DefaultPermissionsForMode(AutonomyMode("unknown"))
		if rules == nil {
			t.Fatal("expected non-nil rules for unknown mode")
		}
		// Should behave like supervised (uses DefaultPermissionRules)
		perms := make(map[string]tool.PermissionLevel)
		for _, r := range rules {
			perms[r.Tool] = r.Level
		}
		for _, name := range []string{"bash", "write", "edit"} {
			if perms[name] != tool.PermAsk {
				t.Errorf("expected %q to be PermAsk for unknown mode (supervised fallback), got %v", name, perms[name])
			}
		}
	})

	// GAP: não implementado — subagentes em RunParallel não têm autonomy mode = plan
	// O draft (§ "Subagentes Read-only") especifica que subagentes de exploração são
	// read-only, mas RunParallel não propaga nem aplica FilterRegistryForMode.
	// Quando isso for implementado, adicionar TestRunParallel_PlanModeEnforced.
}
