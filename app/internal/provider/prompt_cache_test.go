package provider

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// isCacheSet returns true if the CacheControlEphemeralParam has been populated
// (i.e. its Type constant is non-empty, which NewCacheControlEphemeralParam sets
// to "ephemeral").
func isCacheSet(cc anthropic.CacheControlEphemeralParam) bool {
	return string(cc.Type) != ""
}

func TestIsCacheableModel(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		{"claude-3-5-sonnet-20241022", true},
		{"claude-3-5-sonnet-20240620", true},
		{"claude-3-opus-20240229", true},
		{"claude-3-haiku-20240307", true},
		{"claude-3-5-haiku-20241022", true},
		{"claude-3-7-sonnet-20250219", true},
		{"claude-sonnet-4-6", true},
		{"claude-opus-4-5", true},
		{"gpt-4", false},
		{"llama3", false},
		{"", false},
	}
	for _, c := range cases {
		t.Run(c.model, func(t *testing.T) {
			if got := IsCacheableModel(c.model); got != c.want {
				t.Errorf("IsCacheableModel(%q) = %v, want %v", c.model, got, c.want)
			}
		})
	}
}

func TestRequiresLegacyBetaHeader(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		// Claude 3.x models need the beta header
		{"claude-3-5-sonnet-20241022", true},
		{"claude-3-opus-20240229", true},
		{"claude-3-haiku-20240307", true},
		{"claude-3-5-haiku-20241022", true},
		{"claude-3-7-sonnet-20250219", true},
		// Claude 4+ models do NOT need the legacy header
		{"claude-sonnet-4-6", false},
		{"claude-opus-4-5", false},
		// Non-cacheable models also return false
		{"gpt-4", false},
		{"", false},
	}
	for _, c := range cases {
		t.Run(c.model, func(t *testing.T) {
			if got := requiresLegacyBetaHeader(c.model); got != c.want {
				t.Errorf("requiresLegacyBetaHeader(%q) = %v, want %v", c.model, got, c.want)
			}
		})
	}
}

func TestApplySystemCacheControl_Empty(t *testing.T) {
	result := applySystemCacheControl(nil)
	if len(result) != 0 {
		t.Errorf("applySystemCacheControl(nil): expected empty, got %v", result)
	}
}

func TestApplySystemCacheControl_SingleBlock(t *testing.T) {
	blocks := []anthropic.TextBlockParam{
		{Text: "You are a helpful assistant."},
	}
	result := applySystemCacheControl(blocks)

	if len(result) != 1 {
		t.Fatalf("expected 1 block, got %d", len(result))
	}
	cc := result[0].CacheControl
	if !isCacheSet(cc) {
		t.Error("expected CacheControl to be set (Valid=true) on the last block")
	}
}

func TestApplySystemCacheControl_MultipleBlocks(t *testing.T) {
	blocks := []anthropic.TextBlockParam{
		{Text: "Block one."},
		{Text: "Block two."},
		{Text: "Block three — this should be cached."},
	}
	result := applySystemCacheControl(blocks)

	if len(result) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(result))
	}

	// Only the last block should have cache_control set.
	for i, b := range result[:2] {
		if isCacheSet(b.CacheControl) {
			t.Errorf("block[%d] should not have CacheControl set", i)
		}
	}
	if !isCacheSet(result[2].CacheControl) {
		t.Error("expected CacheControl to be set on the last block")
	}
}

func TestApplySystemCacheControl_DoesNotMutateInput(t *testing.T) {
	original := []anthropic.TextBlockParam{
		{Text: "original block"},
	}
	_ = applySystemCacheControl(original)

	// The original slice should be unchanged.
	if isCacheSet(original[0].CacheControl) {
		t.Error("applySystemCacheControl mutated the input slice")
	}
}

func TestWithCacheControlOpts_LegacyModel(t *testing.T) {
	opts := withCacheControlOpts("claude-3-5-sonnet-20241022")
	if len(opts) == 0 {
		t.Error("expected at least one request option for legacy beta model, got none")
	}
}

func TestWithCacheControlOpts_NewModel(t *testing.T) {
	opts := withCacheControlOpts("claude-sonnet-4-6")
	if len(opts) != 0 {
		t.Errorf("expected no extra options for Claude 4+ model, got %d", len(opts))
	}
}
