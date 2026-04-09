package provider

import (
	"fmt"

	"github.com/co2-lab/polvo/internal/config"
)

// Registry holds all configured providers.
type Registry struct {
	providers map[string]LLMProvider
}

// NewRegistry creates providers from config.
func NewRegistry(cfgProviders map[string]config.ProviderConfig) (*Registry, error) {
	r := &Registry{providers: make(map[string]LLMProvider)}

	for name, pcfg := range cfgProviders {
		p, err := createProvider(name, pcfg)
		if err != nil {
			return nil, fmt.Errorf("creating provider %q: %w", name, err)
		}
		r.providers[name] = p
	}

	return r, nil
}

// Get returns a provider by name.
func (r *Registry) Get(name string) (LLMProvider, error) {
	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("provider %q not found", name)
	}
	return p, nil
}

// Default returns the "default" provider, falling back to the first one.
func (r *Registry) Default() (LLMProvider, error) {
	if p, ok := r.providers["default"]; ok {
		return p, nil
	}
	for _, p := range r.providers {
		return p, nil
	}
	return nil, fmt.Errorf("no providers configured")
}

// All returns all registered providers.
func (r *Registry) All() map[string]LLMProvider {
	return r.providers
}

func createProvider(name string, cfg config.ProviderConfig) (LLMProvider, error) {
	switch cfg.Type {
	case "ollama":
		return NewOllama(name, cfg.BaseURL, cfg.DefaultModel), nil
	case "claude":
		return NewClaude(name, cfg.APIKey, cfg.DefaultModel), nil
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", cfg.Type)
	}
}
