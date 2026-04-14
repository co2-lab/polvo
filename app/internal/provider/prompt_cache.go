package provider

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// cacheableModels lists Claude models that support prompt caching.
var cacheableModels = map[string]bool{
	"claude-3-5-sonnet-20241022": true,
	"claude-3-5-sonnet-20240620": true,
	"claude-3-opus-20240229":     true,
	"claude-3-haiku-20240307":    true,
	"claude-3-5-haiku-20241022":  true,
	"claude-3-7-sonnet-20250219": true,
	// claude-sonnet-4-x and newer models support caching natively without the
	// legacy beta header. Keep them here so IsCacheableModel reflects reality.
	"claude-opus-4-5":            true,
	"claude-sonnet-4-5":          true,
	"claude-haiku-4-5":           true,
	"claude-opus-4-0":            true,
	"claude-sonnet-4-6":          true,
}

// legacyBetaModels are models that require the prompt-caching-2024-07-31 beta
// header to activate caching. Newer Claude 4 models have caching enabled by
// default and do not need the header.
var legacyBetaModels = map[string]bool{
	"claude-3-5-sonnet-20241022": true,
	"claude-3-5-sonnet-20240620": true,
	"claude-3-opus-20240229":     true,
	"claude-3-haiku-20240307":    true,
	"claude-3-5-haiku-20241022":  true,
	"claude-3-7-sonnet-20250219": true,
}

// IsCacheableModel returns true if the model supports prompt caching.
func IsCacheableModel(model string) bool {
	return cacheableModels[model]
}

// requiresLegacyBetaHeader returns true if the model needs the
// prompt-caching-2024-07-31 beta header to enable caching.
func requiresLegacyBetaHeader(model string) bool {
	return legacyBetaModels[model]
}

// withCacheControlOpts returns additional request options that activate prompt
// caching for the given model. Returns nil if the model does not need any
// extra options (e.g. Claude 4+ models that have caching on by default).
func withCacheControlOpts(model string) []option.RequestOption {
	if requiresLegacyBetaHeader(model) {
		return []option.RequestOption{
			option.WithHeader("anthropic-beta", string(anthropic.AnthropicBetaPromptCaching2024_07_31)),
		}
	}
	return nil
}

// applySystemCacheControl returns a copy of the system blocks with a
// cache_control breakpoint added to the last block. This instructs the API to
// cache everything up to and including that block.
//
// The caller is responsible for checking IsCacheableModel before calling this.
func applySystemCacheControl(blocks []anthropic.TextBlockParam) []anthropic.TextBlockParam {
	if len(blocks) == 0 {
		return blocks
	}
	result := make([]anthropic.TextBlockParam, len(blocks))
	copy(result, blocks)
	result[len(result)-1].CacheControl = anthropic.NewCacheControlEphemeralParam()
	return result
}
