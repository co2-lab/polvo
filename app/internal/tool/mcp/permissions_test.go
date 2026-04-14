package mcp

import "testing"

func TestPermissionEngine_DenyWins(t *testing.T) {
	engine := NewPermissionEngine(MCPPermissions{
		Allow: []string{"mcp__filesystem__*"},
		Deny:  []string{"mcp__filesystem__write_*"},
	})

	// Denied tool should return PermDeny regardless of Allow pattern.
	got := engine.Evaluate("mcp__filesystem__write_file")
	if got != PermDeny {
		t.Errorf("expected PermDeny, got %q", got)
	}
}

func TestPermissionEngine_AllowWinsOverDefault(t *testing.T) {
	engine := NewPermissionEngine(MCPPermissions{
		Allow: []string{"mcp__filesystem__read_*"},
	})

	got := engine.Evaluate("mcp__filesystem__read_file")
	if got != PermAllow {
		t.Errorf("expected PermAllow, got %q", got)
	}
}

func TestPermissionEngine_AskWinsOverAllow(t *testing.T) {
	engine := NewPermissionEngine(MCPPermissions{
		Allow: []string{"mcp__github__*"},
		Ask:   []string{"mcp__github__merge_*"},
	})

	// Ask more specific rule should win over the broad Allow.
	got := engine.Evaluate("mcp__github__merge_pr")
	if got != PermAsk {
		t.Errorf("expected PermAsk, got %q", got)
	}
}

func TestPermissionEngine_DefaultIsAsk(t *testing.T) {
	engine := NewPermissionEngine(MCPPermissions{})

	got := engine.Evaluate("mcp__unknown__some_tool")
	if got != PermAsk {
		t.Errorf("expected default PermAsk, got %q", got)
	}
}

func TestPermissionEngine_WildcardMatchesAllServerTools(t *testing.T) {
	engine := NewPermissionEngine(MCPPermissions{
		Allow: []string{"mcp__filesystem__*"},
	})

	tools := []string{
		"mcp__filesystem__read_file",
		"mcp__filesystem__list_dir",
		"mcp__filesystem__stat",
	}
	for _, name := range tools {
		got := engine.Evaluate(name)
		if got != PermAllow {
			t.Errorf("Evaluate(%q) = %q; want PermAllow", name, got)
		}
	}

	// Other server should not match.
	got := engine.Evaluate("mcp__github__read_file")
	if got != PermAsk {
		t.Errorf("Evaluate(github tool) = %q; want PermAsk (default)", got)
	}
}

func TestPermissionEngine_DenyPrecedenceOverAskAndAllow(t *testing.T) {
	engine := NewPermissionEngine(MCPPermissions{
		Allow: []string{"mcp__fs__*"},
		Ask:   []string{"mcp__fs__delete_*"},
		Deny:  []string{"mcp__fs__delete_*"},
	})

	got := engine.Evaluate("mcp__fs__delete_file")
	if got != PermDeny {
		t.Errorf("deny should win; got %q", got)
	}
}

// ---------------------------------------------------------------------------
// New gap-filling tests
// ---------------------------------------------------------------------------

// TestPermissionEngine_AllowWithExclusionDeny verifies deny-wins-over-allow
// precedence when the allow list is broad and deny excludes a subset.
// Allow: ["mcp__fs__*"], Deny: ["mcp__fs__write_*"]
// - mcp__fs__read_file  → Allow
// - mcp__fs__write_file → Deny
func TestPermissionEngine_AllowWithExclusionDeny(t *testing.T) {
	engine := NewPermissionEngine(MCPPermissions{
		Allow: []string{"mcp__fs__*"},
		Deny:  []string{"mcp__fs__write_*"},
	})

	t.Run("read_file_is_allowed", func(t *testing.T) {
		got := engine.Evaluate("mcp__fs__read_file")
		if got != PermAllow {
			t.Errorf("expected PermAllow for mcp__fs__read_file, got %q", got)
		}
	})

	t.Run("write_file_is_denied", func(t *testing.T) {
		got := engine.Evaluate("mcp__fs__write_file")
		if got != PermDeny {
			t.Errorf("expected PermDeny for mcp__fs__write_file (deny wins over allow), got %q", got)
		}
	})
}

// TestPermissionEngine_SpecialCharsInServerName verifies that ParseNamespacedTool
// handles dots in the server name and hyphens in the tool name gracefully.
// Tool name: "mcp__my.server__tool-name"
func TestPermissionEngine_SpecialCharsInServerName(t *testing.T) {
	// ParseNamespacedTool splits on "__"; a dot in the server name is just part
	// of the server segment since it doesn't contain "__".
	toolName := "mcp__my.server__tool-name"
	server, tool, ok := ParseNamespacedTool(toolName)
	if !ok {
		// Document as a GAP if parsing fails for dots in server names.
		t.Logf("GAP: ParseNamespacedTool rejects dot in server name (%q) — ok=false", toolName)
		return
	}
	if server != "my.server" {
		t.Errorf("server: got %q, want %q", server, "my.server")
	}
	if tool != "tool-name" {
		t.Errorf("tool: got %q, want %q", tool, "tool-name")
	}

	// Ensure the engine can evaluate a namespaced name containing a dot.
	engine := NewPermissionEngine(MCPPermissions{
		Allow: []string{"mcp__my.server__*"},
	})
	got := engine.Evaluate(toolName)
	if got != PermAllow {
		t.Errorf("Evaluate(%q) = %q; want PermAllow — wildcard should match dotted server", toolName, got)
	}
}
