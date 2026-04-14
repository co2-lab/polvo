package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/co2-lab/polvo/internal/tool"
)

// ---- helpers ----------------------------------------------------------------

// buildReadOnlyTestRegistry builds a tool.Registry containing only the tools
// in the ReadOnlyToolset whitelist, each backed by a noopToolImpl.
func buildReadOnlyTestRegistry() *tool.Registry {
	reg := tool.NewRegistry()
	for _, name := range ReadOnlyToolset {
		reg.Register(makeNoopTool(name))
	}
	return reg
}

// ---- TestNewReadOnlyExecutor_PanicsOnWriteTool ------------------------------

func TestNewReadOnlyExecutor_PanicsOnWriteTool(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when registry contains a write tool, got none")
		}
	}()

	reg := tool.NewRegistry()
	reg.Register(makeNoopTool("write")) // write tool must cause panic

	NewReadOnlyExecutor(reg) // should panic
}

// ---- TestNewReadOnlyExecutor_AllowedTools -----------------------------------

func TestNewReadOnlyExecutor_AllowedTools(t *testing.T) {
	reg := buildReadOnlyTestRegistry()
	exec := NewReadOnlyExecutor(reg) // must not panic
	if exec == nil {
		t.Fatal("expected non-nil executor")
	}
}

// ---- TestReadOnlyExecutor_PermittedTool ------------------------------------

func TestReadOnlyExecutor_PermittedTool(t *testing.T) {
	reg := buildReadOnlyTestRegistry()
	exec := NewReadOnlyExecutor(reg)

	result, err := exec.Execute(context.Background(), "read", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("expected no error for whitelisted tool 'read', got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.IsError {
		t.Errorf("expected success result, got error: %s", result.Content)
	}
}

// ---- TestReadOnlyExecutor_BlocksWriteTool ----------------------------------

func TestReadOnlyExecutor_BlocksWriteTool(t *testing.T) {
	// Build a registry that has no write tools (so no panic on construction),
	// then attempt to call a write tool by name that is NOT in the registry.
	reg := buildReadOnlyTestRegistry()
	exec := NewReadOnlyExecutor(reg)

	// Attempt to call "write" — it's not in the whitelist; must be blocked
	result, err := exec.Execute(context.Background(), "write", json.RawMessage(`{"path":"test.txt","content":"pwned"}`))

	// We expect an ErrToolNotPermitted error
	if err == nil {
		t.Fatal("expected ErrToolNotPermitted, got nil error")
	}

	var permErr ErrToolNotPermitted
	ok := false
	// Check via type assertion
	if e, isType := err.(ErrToolNotPermitted); isType {
		permErr = e
		ok = true
	}
	if !ok {
		t.Fatalf("expected ErrToolNotPermitted, got %T: %v", err, err)
	}
	if permErr.Tool != "write" {
		t.Errorf("expected Tool='write', got %q", permErr.Tool)
	}
	if len(permErr.Allowed) == 0 {
		t.Error("expected non-empty Allowed list")
	}
	if result != nil {
		t.Error("expected nil result when tool is not permitted")
	}
}

// ---- TestReadOnlyExecutor_BlocksAllWriteTools -------------------------------

func TestReadOnlyExecutor_BlocksAllWriteTools(t *testing.T) {
	reg := buildReadOnlyTestRegistry()
	exec := NewReadOnlyExecutor(reg)

	forbiddenTools := []string{"write", "edit", "bash", "patch", "memory_write", "delegate", "explore"}

	for _, name := range forbiddenTools {
		t.Run(name, func(t *testing.T) {
			_, err := exec.Execute(context.Background(), name, json.RawMessage(`{}`))
			if err == nil {
				t.Errorf("tool %q should be blocked but was not", name)
				return
			}
			if _, ok := err.(ErrToolNotPermitted); !ok {
				t.Errorf("tool %q: expected ErrToolNotPermitted, got %T: %v", name, err, err)
			}
		})
	}
}

// ---- TestReadOnlyExecutor_UnknownAllowedTool --------------------------------

func TestReadOnlyExecutor_UnknownAllowedTool(t *testing.T) {
	// A tool name that is in the whitelist but not in the registry
	// should return an error result (not an ErrToolNotPermitted)
	emptyReg := tool.NewRegistry()
	exec := NewReadOnlyExecutor(emptyReg)

	result, err := exec.Execute(context.Background(), "read", json.RawMessage(`{}`))
	// Should NOT return ErrToolNotPermitted (tool is whitelisted)
	if _, ok := err.(ErrToolNotPermitted); ok {
		t.Fatal("read is in whitelist — should not get ErrToolNotPermitted")
	}
	// But tool is not in registry, so we expect an error result (IsError=true)
	if result == nil {
		t.Fatal("expected non-nil result (error result)")
	}
	if !result.IsError {
		t.Errorf("expected IsError=true for unknown tool, got: %s", result.Content)
	}
}

// ---- TestErrToolNotPermitted_Error ------------------------------------------

func TestErrToolNotPermitted_Error(t *testing.T) {
	e := ErrToolNotPermitted{Tool: "bash", Allowed: []string{"read", "glob"}}
	msg := e.Error()
	if !containsStr(msg, "bash") {
		t.Errorf("expected error message to contain 'bash', got: %s", msg)
	}
	if !containsStr(msg, "read-only subagent") {
		t.Errorf("expected error message to mention 'read-only subagent', got: %s", msg)
	}
}
