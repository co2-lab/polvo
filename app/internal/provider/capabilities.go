package provider

// Capabilities describes the features supported by a provider/model pair.
type Capabilities struct {
	// SupportsTools indicates native function-calling (tool use) support.
	// When false, the openAIProvider falls back to XML tool calling via prompt injection.
	SupportsTools bool

	// SupportsStreaming indicates SSE/streaming token support.
	SupportsStreaming bool

	// SupportsVision indicates image input support.
	SupportsVision bool

	// SupportsPrefill indicates assistant-turn prefill support (Anthropic-only).
	SupportsPrefill bool

	// SupportsPromptCaching indicates cache_control support (Anthropic-only).
	SupportsPromptCaching bool

	// MaxContextTokens is the model's context window size. 0 = unknown.
	MaxContextTokens int

	// MaxOutputTokens is the model's maximum output token limit. 0 = unknown.
	MaxOutputTokens int
}

// CapabilitiesProvider is implemented by providers that can report their
// capabilities for a given model name.
type CapabilitiesProvider interface {
	Capabilities(model string) Capabilities
}

// GetCapabilities returns capabilities for a provider, using the optional
// model name to specialise (e.g. o1/o3 reasoning models). Falls back to
// provider-type defaults when the provider does not implement CapabilitiesProvider.
func GetCapabilities(p LLMProvider, providerType, model string) Capabilities {
	if cp, ok := p.(CapabilitiesProvider); ok {
		return cp.Capabilities(model)
	}
	return DefaultCapabilities(providerType, model)
}
