package tool

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

// fakeToolForMiddleware is a test double for Tool.
type fakeToolForMiddleware struct {
	name    string
	content string
	isError bool
}

func (f *fakeToolForMiddleware) Name() string               { return f.name }
func (f *fakeToolForMiddleware) Description() string        { return "fake" }
func (f *fakeToolForMiddleware) InputSchema() json.RawMessage { return json.RawMessage(`{}`) }
func (f *fakeToolForMiddleware) Execute(_ context.Context, _ json.RawMessage) (*Result, error) {
	return &Result{Content: f.content, IsError: f.isError}, nil
}

func TestMiddleware_WrapsName(t *testing.T) {
	inner := &fakeToolForMiddleware{name: "read"}
	wrapped := SecretsMaskingMiddleware(inner)
	if wrapped.Name() != "read" {
		t.Errorf("Name() = %q, want %q", wrapped.Name(), "read")
	}
}

func TestMiddleware_MasksResult(t *testing.T) {
	token := "ghp_" + strings.Repeat("X", 36)
	inner := &fakeToolForMiddleware{name: "read", content: "token: " + token}
	wrapped := SecretsMaskingMiddleware(inner)

	result, err := wrapped.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.Contains(result.Content, token) {
		t.Errorf("secret not masked: %q", result.Content)
	}
	if !strings.Contains(result.Content, "[GITHUB_PAT_REDACTED]") {
		t.Errorf("expected redaction label in output: %q", result.Content)
	}
}

func TestMiddleware_MasksErrorResult(t *testing.T) {
	token := "ghp_" + strings.Repeat("Y", 36)
	inner := &fakeToolForMiddleware{name: "read", content: "error: " + token, isError: true}
	wrapped := SecretsMaskingMiddleware(inner)

	result, err := wrapped.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true to be preserved")
	}
	if strings.Contains(result.Content, token) {
		t.Errorf("secret not masked in error result: %q", result.Content)
	}
}

func TestMiddleware_NilResult(t *testing.T) {
	// A tool that returns nil result.
	inner := &nilResultTool{}
	wrapped := SecretsMaskingMiddleware(inner)

	result, err := wrapped.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}

type nilResultTool struct{}

func (n *nilResultTool) Name() string                  { return "nil_tool" }
func (n *nilResultTool) Description() string           { return "returns nil" }
func (n *nilResultTool) InputSchema() json.RawMessage  { return json.RawMessage(`{}`) }
func (n *nilResultTool) Execute(_ context.Context, _ json.RawMessage) (*Result, error) {
	return nil, nil
}

func TestMiddleware_CleanContent(t *testing.T) {
	inner := &fakeToolForMiddleware{name: "read", content: "no secrets here"}
	wrapped := SecretsMaskingMiddleware(inner)

	result, err := wrapped.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Content != "no secrets here" {
		t.Errorf("clean content should be unchanged, got %q", result.Content)
	}
}

func TestMiddleware_Race(t *testing.T) {
	token := "ghp_" + strings.Repeat("Z", 36)
	inner := &fakeToolForMiddleware{name: "read", content: "token: " + token}
	wrapped := SecretsMaskingMiddleware(inner)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wrapped.Execute(context.Background(), nil) //nolint:errcheck
		}()
	}
	wg.Wait()
}
