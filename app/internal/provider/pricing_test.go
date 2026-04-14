package provider

import (
	"testing"
)

func TestComputeCostUSD_KnownModel(t *testing.T) {
	usage := TokenUsage{
		PromptTokens:     1_000_000,
		CompletionTokens: 1_000_000,
	}
	cost := ComputeCostUSD(usage, "claude-3-5-sonnet-20241022")
	// 1M input @ $3 + 1M output @ $15 = $18
	if cost != 18.0 {
		t.Errorf("expected 18.0, got %f", cost)
	}
}

func TestComputeCostUSD_UnknownModelReturnsZero(t *testing.T) {
	usage := TokenUsage{
		PromptTokens:     100_000,
		CompletionTokens: 100_000,
	}
	cost := ComputeCostUSD(usage, "unknown-model-xyz")
	if cost != 0 {
		t.Errorf("expected 0 for unknown model, got %f", cost)
	}
}

func TestComputeCostUSD_CacheTokensContribute(t *testing.T) {
	usage := TokenUsage{
		CacheReadTokens:  1_000_000,
		CacheWriteTokens: 1_000_000,
	}
	// claude-3-5-sonnet: cache read $0.30/M, cache write $3.75/M → $4.05
	cost := ComputeCostUSD(usage, "claude-3-5-sonnet-20241022")
	expected := 0.30 + 3.75
	if cost != expected {
		t.Errorf("expected %f, got %f", expected, cost)
	}
}

func TestModelPricingTable_AllModelsHaveNonZeroPrices(t *testing.T) {
	for model, p := range modelPricingTable {
		if p.InputPerMillion <= 0 {
			t.Errorf("model %q has zero InputPerMillion", model)
		}
		if p.OutputPerMillion <= 0 {
			t.Errorf("model %q has zero OutputPerMillion", model)
		}
		if p.CacheReadPerMillion <= 0 {
			t.Errorf("model %q has zero CacheReadPerMillion", model)
		}
		if p.CacheWritePerMillion <= 0 {
			t.Errorf("model %q has zero CacheWritePerMillion", model)
		}
	}
}
