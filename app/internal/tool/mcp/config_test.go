package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMCPConfig_ValidJSON(t *testing.T) {
	raw := `{
		"mcpServers": {
			"filesystem": {
				"command": "npx",
				"args": ["-y", "@modelcontextprotocol/server-filesystem", "/workspace"],
				"transport": "stdio",
				"filterTools": "^(read|list).*"
			},
			"github": {
				"url": "https://api.example.com/mcp/",
				"transport": "streamable-http",
				"disabled": true
			}
		},
		"permissions": {
			"allow": ["mcp__filesystem__read_*"],
			"ask":   ["mcp__github__*"],
			"deny":  ["mcp__filesystem__write_*"]
		}
	}`

	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadMCPConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.MCPServers) != 2 {
		t.Errorf("want 2 servers, got %d", len(cfg.MCPServers))
	}

	fs, ok := cfg.MCPServers["filesystem"]
	if !ok {
		t.Fatal("expected filesystem server")
	}
	if fs.Name != "filesystem" {
		t.Errorf("Name field not populated: got %q", fs.Name)
	}
	if fs.Command != "npx" {
		t.Errorf("Command: got %q", fs.Command)
	}
	if len(fs.Args) != 3 {
		t.Errorf("Args: got %d elements, want 3", len(fs.Args))
	}
	if fs.FilterTools != "^(read|list).*" {
		t.Errorf("FilterTools: got %q", fs.FilterTools)
	}

	gh, ok := cfg.MCPServers["github"]
	if !ok {
		t.Fatal("expected github server")
	}
	if !gh.Disabled {
		t.Error("github server should be disabled")
	}

	if len(cfg.Permissions.Allow) != 1 || cfg.Permissions.Allow[0] != "mcp__filesystem__read_*" {
		t.Errorf("Permissions.Allow: got %v", cfg.Permissions.Allow)
	}
	if len(cfg.Permissions.Deny) != 1 || cfg.Permissions.Deny[0] != "mcp__filesystem__write_*" {
		t.Errorf("Permissions.Deny: got %v", cfg.Permissions.Deny)
	}
}

func TestLoadMCPConfig_MissingFile(t *testing.T) {
	cfg, err := LoadMCPConfig("/nonexistent/path/mcp.json")
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if len(cfg.MCPServers) != 0 {
		t.Errorf("expected empty MCPServers, got %d", len(cfg.MCPServers))
	}
}

func TestLoadMCPConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(path, []byte("not valid json {{{"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := LoadMCPConfig(path)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestExpandEnvVars_KnownVar(t *testing.T) {
	t.Setenv("TEST_MCP_TOKEN", "secret123")

	result := ExpandEnvVars("Bearer ${env:TEST_MCP_TOKEN}")
	if result != "Bearer secret123" {
		t.Errorf("got %q, want %q", result, "Bearer secret123")
	}
}

func TestExpandEnvVars_UnknownVar(t *testing.T) {
	// Ensure variable is not set.
	os.Unsetenv("POLVO_UNDEFINED_VAR_XYZ")

	result := ExpandEnvVars("prefix-${env:POLVO_UNDEFINED_VAR_XYZ}-suffix")
	if result != "prefix--suffix" {
		t.Errorf("got %q, want %q", result, "prefix--suffix")
	}
}

func TestExpandEnvVars_NoPlaceholder(t *testing.T) {
	input := "just a plain string"
	result := ExpandEnvVars(input)
	if result != input {
		t.Errorf("got %q, want %q", result, input)
	}
}

func TestLoadMCPConfig_EnvExpansion(t *testing.T) {
	t.Setenv("MY_API_KEY", "token-abc")

	raw := `{
		"mcpServers": {
			"svc": {
				"command": "node",
				"env": {"API_KEY": "${env:MY_API_KEY}"}
			}
		}
	}`

	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadMCPConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	svc := cfg.MCPServers["svc"]
	if svc.Env["API_KEY"] != "token-abc" {
		t.Errorf("env expansion: got %q, want %q", svc.Env["API_KEY"], "token-abc")
	}
}

// ---------------------------------------------------------------------------
// New gap-filling tests
// ---------------------------------------------------------------------------

// TestConfig_DisabledServerNotLoaded verifies that a server with disabled:true
// is present in the parsed config with the Disabled field set to true.
// (The MCPServerConfig struct has a Disabled bool field.)
// Filtering disabled servers out of active connections is the caller's
// responsibility; LoadMCPConfig itself retains them.
func TestConfig_DisabledServerNotLoaded(t *testing.T) {
	raw := `{
		"mcpServers": {
			"active": {
				"command": "npx",
				"args": ["server"]
			},
			"inactive": {
				"command": "npx",
				"args": ["server"],
				"disabled": true
			}
		}
	}`

	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadMCPConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	inactive, ok := cfg.MCPServers["inactive"]
	if !ok {
		t.Fatal("expected 'inactive' server to be present in MCPServers map")
	}
	if !inactive.Disabled {
		t.Errorf("expected inactive.Disabled=true, got false")
	}

	active, ok := cfg.MCPServers["active"]
	if !ok {
		t.Fatal("expected 'active' server to be present in MCPServers map")
	}
	if active.Disabled {
		t.Errorf("expected active.Disabled=false, got true")
	}
}

// TestConfig_FilterToolsRegex verifies that the FilterTools field is preserved
// correctly through LoadMCPConfig.  This tests config parsing, not live
// tool filtering (which requires an active MCP connection).
func TestConfig_FilterToolsRegex(t *testing.T) {
	raw := `{
		"mcpServers": {
			"filesystem": {
				"command": "npx",
				"args": ["-y", "@modelcontextprotocol/server-filesystem"],
				"filterTools": "^(read|list).*"
			}
		}
	}`

	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadMCPConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fs, ok := cfg.MCPServers["filesystem"]
	if !ok {
		t.Fatal("expected 'filesystem' server")
	}
	if fs.FilterTools != "^(read|list).*" {
		t.Errorf("FilterTools: got %q, want %q", fs.FilterTools, "^(read|list).*")
	}
}

// Verify that MCPServerConfig fields can be marshalled/unmarshalled consistently.
func TestMCPServerConfig_JSONRoundTrip(t *testing.T) {
	orig := MCPServerConfig{
		Name:        "test",
		Command:     "npx",
		Args:        []string{"-y", "server"},
		Transport:   "stdio",
		FilterTools: "^read.*",
		Disabled:    false,
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var decoded MCPServerConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Command != orig.Command {
		t.Errorf("Command mismatch: %q vs %q", decoded.Command, orig.Command)
	}
}
