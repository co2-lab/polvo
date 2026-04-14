package browser

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// ---- disabled tool ----

func TestBrowserTool_DisabledConfig(t *testing.T) {
	cfg := DefaultConfig() // Enabled = false
	bt := NewBrowserTool(cfg)

	input, _ := json.Marshal(BrowserInput{Action: "snapshot"})
	result, err := bt.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true when browser is disabled")
	}
	if !strings.Contains(result.Content, "disabled") {
		t.Errorf("expected 'disabled' in error message, got: %s", result.Content)
	}
}

// ---- URL allowlist enforcement ----

func TestBrowserTool_NavigateBlockedByAllowlist(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Security.URLAllowlist = true
	bt := NewBrowserTool(cfg)

	// No URLs have been added to the allowlist; navigating to an unknown domain must fail.
	input, _ := json.Marshal(BrowserInput{Action: "navigate", URL: "https://unknown-external-site.com/page"})
	result, err := bt.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for URL not in allowlist")
	}
	if !strings.Contains(result.Content, "not allowed") {
		t.Errorf("expected 'not allowed' in error message, got: %s", result.Content)
	}
}

func TestBrowserTool_NavigateAllowedAfterAddFromText(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Security.URLAllowlist = true
	bt := NewBrowserTool(cfg)

	// Seed the allowlist as if the user mentioned the URL in conversation.
	bt.AddURLsFromText("Please go to https://example.com/docs")

	input, _ := json.Marshal(BrowserInput{Action: "navigate", URL: "https://example.com/docs"})
	result, err := bt.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	// The tool will fail because playwright-mcp is not running, but the error
	// should be about the process, not about the URL being blocked.
	if result.IsError && strings.Contains(result.Content, "not allowed") {
		t.Errorf("URL should have passed allowlist check, got: %s", result.Content)
	}
}

func TestBrowserTool_NavigateLocalhostAlwaysAllowed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Security.URLAllowlist = true
	bt := NewBrowserTool(cfg)

	// localhost must always pass, even with no seeded allowlist.
	input, _ := json.Marshal(BrowserInput{Action: "navigate", URL: "http://localhost:3000"})
	result, err := bt.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if result.IsError && strings.Contains(result.Content, "not allowed") {
		t.Errorf("localhost should always be allowed, got: %s", result.Content)
	}
}

// ---- blocked actions ----

func TestBrowserTool_FileUploadBlocked(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	bt := NewBrowserTool(cfg)

	input, _ := json.Marshal(BrowserInput{Action: "file_upload"})
	result, err := bt.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for file_upload when BlockFileUpload=true")
	}
}

func TestBrowserTool_JSEvalBlocked(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	bt := NewBrowserTool(cfg)

	input, _ := json.Marshal(BrowserInput{Action: "evaluate"})
	result, err := bt.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for evaluate when BlockJSEval=true")
	}
}

// ---- invalid input ----

func TestBrowserTool_InvalidJSON(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	bt := NewBrowserTool(cfg)

	result, err := bt.Execute(context.Background(), json.RawMessage(`{bad json`))
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for invalid JSON input")
	}
}
