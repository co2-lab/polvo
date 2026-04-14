package mcp

import "testing"

func TestNamespacedToolName(t *testing.T) {
	tests := []struct {
		server, tool, want string
	}{
		{"filesystem", "read_file", "mcp__filesystem__read_file"},
		{"github", "create_issue", "mcp__github__create_issue"},
		{"my-server", "do_thing", "mcp__my-server__do_thing"},
	}
	for _, tt := range tests {
		got := NamespacedToolName(tt.server, tt.tool)
		if got != tt.want {
			t.Errorf("NamespacedToolName(%q, %q) = %q; want %q", tt.server, tt.tool, got, tt.want)
		}
	}
}

func TestParseNamespacedTool_Valid(t *testing.T) {
	tests := []struct {
		input      string
		wantServer string
		wantTool   string
	}{
		{"mcp__filesystem__read_file", "filesystem", "read_file"},
		{"mcp__github__create_issue", "github", "create_issue"},
		{"mcp__my-server__do_thing", "my-server", "do_thing"},
	}
	for _, tt := range tests {
		server, tool, ok := ParseNamespacedTool(tt.input)
		if !ok {
			t.Errorf("ParseNamespacedTool(%q) returned ok=false, want ok=true", tt.input)
			continue
		}
		if server != tt.wantServer {
			t.Errorf("ParseNamespacedTool(%q) server=%q; want %q", tt.input, server, tt.wantServer)
		}
		if tool != tt.wantTool {
			t.Errorf("ParseNamespacedTool(%q) tool=%q; want %q", tt.input, tool, tt.wantTool)
		}
	}
}

func TestParseNamespacedTool_Invalid(t *testing.T) {
	cases := []string{
		"read_file",
		"mcp__",
		"mcp__filesystem",
		"mcp____tool",
		"",
		"some__other__format",
	}
	for _, c := range cases {
		_, _, ok := ParseNamespacedTool(c)
		if ok {
			t.Errorf("ParseNamespacedTool(%q) returned ok=true, want ok=false", c)
		}
	}
}

func TestNamespacedRoundTrip(t *testing.T) {
	server, tool := "filesystem", "read_file"
	namespaced := NamespacedToolName(server, tool)
	gotServer, gotTool, ok := ParseNamespacedTool(namespaced)
	if !ok {
		t.Fatalf("ParseNamespacedTool(%q) returned ok=false", namespaced)
	}
	if gotServer != server {
		t.Errorf("round-trip server: got %q, want %q", gotServer, server)
	}
	if gotTool != tool {
		t.Errorf("round-trip tool: got %q, want %q", gotTool, tool)
	}
}
