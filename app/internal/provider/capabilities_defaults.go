package provider

import "strings"

// providerDefaults maps provider type names to their baseline Capabilities.
var providerDefaults = map[string]Capabilities{
	"claude": {
		SupportsTools:         true,
		SupportsStreaming:      true,
		SupportsVision:        true,
		SupportsPrefill:       true,
		SupportsPromptCaching: true,
		MaxContextTokens:      200_000,
		MaxOutputTokens:       64_000,
	},
	"openai": {
		SupportsTools:    true,
		SupportsStreaming: true,
		SupportsVision:   true,
		MaxContextTokens: 128_000,
		MaxOutputTokens:  16_384,
	},
	"gemini": {
		SupportsTools:    true,
		SupportsStreaming: true,
		SupportsVision:   true,
		MaxContextTokens: 1_048_576,
		MaxOutputTokens:  65_536,
	},
	"deepseek": {
		SupportsTools:    true,
		SupportsStreaming: true,
		MaxContextTokens: 64_000,
		MaxOutputTokens:  8_192,
	},
	"groq": {
		SupportsTools:    true,
		SupportsStreaming: true,
		MaxContextTokens: 128_000,
		MaxOutputTokens:  8_192,
	},
	"mistral": {
		SupportsTools:    true,
		SupportsStreaming: true,
		MaxContextTokens: 128_000,
		MaxOutputTokens:  8_192,
	},
	"openrouter": {
		SupportsTools:    true,
		SupportsStreaming: true,
		MaxContextTokens: 128_000,
		MaxOutputTokens:  16_384,
	},
	"xai": {
		SupportsTools:    true,
		SupportsStreaming: true,
		MaxContextTokens: 131_072,
		MaxOutputTokens:  16_384,
	},
	// Ollama local models generally do not support native tool calling.
	// Specific models (llama3.1+, qwen2.5) do support it, but we default
	// conservatively to XML injection.
	"ollama": {
		SupportsTools:    false,
		SupportsStreaming: true,
		MaxContextTokens: 32_768,
		MaxOutputTokens:  4_096,
	},
	"openai-compatible": {
		SupportsTools:    true,
		SupportsStreaming: true,
		MaxContextTokens: 128_000,
		MaxOutputTokens:  16_384,
	},
	// GLM (Zhipu AI)
	"glm": {
		SupportsTools:    true,
		SupportsStreaming: true,
		SupportsVision:   true, // GLM-4V supports vision
		MaxContextTokens: 128_000,
		MaxOutputTokens:  4_096,
	},
	// MiniMax
	"minimax": {
		SupportsTools:    true,
		SupportsStreaming: true,
		MaxContextTokens: 1_000_000,
		MaxOutputTokens:  4_096,
	},
	// Kimi (Moonshot AI)
	"kimi": {
		SupportsTools:    true,
		SupportsStreaming: true,
		MaxContextTokens: 128_000,
		MaxOutputTokens:  4_096,
	},
}

// DefaultCapabilities returns the default Capabilities for a provider type and
// optional model name. The model name is used to detect OpenAI reasoning models
// (o1, o3) which have different constraints.
func DefaultCapabilities(providerType, model string) Capabilities {
	// OpenAI reasoning models (o1/o3) do not support streaming or temperature.
	if providerType == "openai" && isReasoningModel(model) {
		return Capabilities{
			SupportsTools:    true,
			SupportsStreaming: false,
			MaxContextTokens: 200_000,
			MaxOutputTokens:  100_000,
		}
	}

	caps, ok := providerDefaults[providerType]
	if !ok {
		// Unknown provider — conservative defaults with tool support.
		return Capabilities{
			SupportsTools:    true,
			SupportsStreaming: true,
			MaxContextTokens: 32_768,
			MaxOutputTokens:  4_096,
		}
	}
	return caps
}

// isReasoningModel reports whether the model name indicates an OpenAI
// reasoning model (o1, o3 family).
func isReasoningModel(model string) bool {
	return strings.HasPrefix(model, "o1") || strings.HasPrefix(model, "o3")
}
