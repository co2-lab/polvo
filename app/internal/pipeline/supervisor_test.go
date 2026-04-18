package pipeline

import (
	"context"
	"fmt"
	"testing"

	"github.com/co2-lab/polvo/internal/provider"
)

// mockChatProvider is a minimal ChatProvider that returns a preset response.
type mockChatProvider struct {
	response string
	err      error
}

func (m *mockChatProvider) Name() string                                       { return "mock" }
func (m *mockChatProvider) Available(_ context.Context) error                  { return nil }
func (m *mockChatProvider) Complete(_ context.Context, _ provider.Request) (*provider.Response, error) {
	return &provider.Response{Content: m.response}, m.err
}
func (m *mockChatProvider) Chat(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &provider.ChatResponse{
		Message: provider.Message{Role: "assistant", Content: m.response},
	}, nil
}

// TestRouteHighConfidence verifies that a high-confidence response is routed correctly.
func TestRouteHighConfidence(t *testing.T) {
	resp := `{"agent":"spec","confidence":0.9,"rationale":"spec file changed","fallthrough":false}`
	sup := NewSupervisor(&mockChatProvider{response: resp}, "test-model", DefaultChain(), 0.7)

	decision, err := sup.Route(context.Background(), &FileEvent{
		Type: EventSpecChanged,
		File: "spec.md",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Agent != "spec" {
		t.Errorf("expected agent 'spec', got %q", decision.Agent)
	}
	if decision.Confidence != 0.9 {
		t.Errorf("expected confidence 0.9, got %f", decision.Confidence)
	}
	if decision.Fallthrough {
		t.Error("expected fallthrough=false for high confidence")
	}
}

// TestRouteLowConfidenceFallthrough verifies confidence < threshold sets Fallthrough=true.
func TestRouteLowConfidenceFallthrough(t *testing.T) {
	resp := `{"agent":"spec","confidence":0.5,"rationale":"not sure","fallthrough":false}`
	sup := NewSupervisor(&mockChatProvider{response: resp}, "test-model", DefaultChain(), 0.7)

	decision, err := sup.Route(context.Background(), &FileEvent{Type: EventSpecChanged, File: "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !decision.Fallthrough {
		t.Error("expected fallthrough=true when confidence < threshold")
	}
}

// TestRouteExplicitFallthrough verifies fallthrough:true is respected.
func TestRouteExplicitFallthrough(t *testing.T) {
	resp := `{"agent":"","confidence":0.0,"rationale":"no match","fallthrough":true}`
	sup := NewSupervisor(&mockChatProvider{response: resp}, "test-model", DefaultChain(), 0.7)

	decision, err := sup.Route(context.Background(), &FileEvent{Type: EventSpecChanged, File: "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !decision.Fallthrough {
		t.Error("expected fallthrough=true when explicitly set")
	}
}

// TestRouteInvalidJSONFallthrough verifies that invalid JSON falls through silently.
func TestRouteInvalidJSONFallthrough(t *testing.T) {
	sup := NewSupervisor(&mockChatProvider{response: "not json at all"}, "test-model", DefaultChain(), 0.7)

	decision, err := sup.Route(context.Background(), &FileEvent{Type: EventSpecChanged, File: "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !decision.Fallthrough {
		t.Error("expected fallthrough=true for invalid JSON")
	}
}

// TestRouteProviderErrorFallthrough verifies that a provider error falls through.
func TestRouteProviderErrorFallthrough(t *testing.T) {
	sup := NewSupervisor(&mockChatProvider{err: fmt.Errorf("network error")}, "test-model", DefaultChain(), 0.7)

	decision, err := sup.Route(context.Background(), &FileEvent{Type: EventSpecChanged, File: "x"})
	if err == nil {
		t.Error("expected error from provider, got nil")
	}
	if !decision.Fallthrough {
		t.Error("expected fallthrough=true on provider error")
	}
}

// TestSpecialistRegistry verifies all DefaultChain agents appear in the registry.
func TestSpecialistRegistry(t *testing.T) {
	reg := SpecialistRegistry()
	for _, step := range DefaultChain() {
		if _, ok := reg[step.Agent]; !ok {
			t.Errorf("agent %q missing from SpecialistRegistry", step.Agent)
		}
	}
}
