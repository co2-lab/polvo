package provider

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseXMLToolCall(t *testing.T) {
	t.Run("valid tool call", func(t *testing.T) {
		text := "I'll read that file.\n<tool_call>\n<name>read</name>\n<parameters>\n{\"file_path\": \"/foo/bar.go\"}\n</parameters>\n</tool_call>"
		tc := ParseXMLToolCall(text)
		if tc == nil {
			t.Fatal("expected tool call, got nil")
		}
		if tc.Name != "read" {
			t.Errorf("expected name=read, got %q", tc.Name)
		}
		var args map[string]string
		if err := json.Unmarshal(tc.Input, &args); err != nil {
			t.Fatalf("input not valid JSON: %v", err)
		}
		if args["file_path"] != "/foo/bar.go" {
			t.Errorf("unexpected file_path: %q", args["file_path"])
		}
	})

	t.Run("no tool call", func(t *testing.T) {
		if tc := ParseXMLToolCall("Just a plain response."); tc != nil {
			t.Errorf("expected nil, got %+v", tc)
		}
	})

	t.Run("invalid JSON params falls back to empty object", func(t *testing.T) {
		text := "<tool_call><name>ls</name><parameters>not json</parameters></tool_call>"
		tc := ParseXMLToolCall(text)
		if tc == nil {
			t.Fatal("expected tool call, got nil")
		}
		if tc.Name != "ls" {
			t.Errorf("expected name=ls, got %q", tc.Name)
		}
		if string(tc.Input) != "{}" {
			t.Errorf("expected {}, got %q", string(tc.Input))
		}
	})

	t.Run("empty name returns nil", func(t *testing.T) {
		text := "<tool_call><name>   </name><parameters>{}</parameters></tool_call>"
		tc := ParseXMLToolCall(text)
		if tc != nil {
			t.Errorf("expected nil for empty name, got %+v", tc)
		}
	})

	t.Run("tool call ID is set", func(t *testing.T) {
		text := "<tool_call><name>glob</name><parameters>{\"pattern\":\"*.go\"}</parameters></tool_call>"
		tc := ParseXMLToolCall(text)
		if tc == nil {
			t.Fatal("expected tool call, got nil")
		}
		if !strings.HasPrefix(tc.ID, "xml-") {
			t.Errorf("expected ID to start with xml-, got %q", tc.ID)
		}
	})
}

func TestXMLToolPrompt(t *testing.T) {
	tools := []ToolDef{
		{Name: "read", Description: "Read a file", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "bash", Description: "Run a command", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}

	prompt := XMLToolPrompt(tools)
	if prompt == "" {
		t.Fatal("expected non-empty prompt")
	}
	if !strings.Contains(prompt, "<tool_call>") {
		t.Error("prompt should contain <tool_call> example")
	}
	if !strings.Contains(prompt, "read") {
		t.Error("prompt should mention 'read' tool")
	}
	if !strings.Contains(prompt, "bash") {
		t.Error("prompt should mention 'bash' tool")
	}

	// Empty tools returns empty string
	if XMLToolPrompt(nil) != "" {
		t.Error("expected empty string for nil tools")
	}
}
