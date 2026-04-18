package provider

import (
	"testing"
)

func TestContextWindowHybridLookup(t *testing.T) {
	cases := []struct {
		model string
		want  int
	}{
		{"claude-sonnet-4-6", 1_000_000}, // LiteLLM exact: 1M
		{"claude-opus-4-6", 1_000_000},   // LiteLLM exact: 1M
		{"claude-opus-4-5", 200_000},     // LiteLLM exact: 200k
		{"claude-3-5-sonnet-20241022", 200_000}, // LiteLLM exact: 200k
		{"gpt-4o-2024-11-20", 128_000},   // LiteLLM exact: 128k
		{"gpt-4o", 128_000},              // LiteLLM or prefix
		{"gemini-2.5-pro", 1_048_576},    // LiteLLM or prefix
		{"unknown-model-xyz", 0},         // unknown
	}
	for _, c := range cases {
		got := ContextWindowForModel(c.model)
		if got != c.want {
			t.Errorf("ContextWindowForModel(%q) = %d, want %d", c.model, got, c.want)
		}
	}
}
