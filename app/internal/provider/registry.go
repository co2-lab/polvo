package provider

import (
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/co2-lab/polvo/internal/config"
)

// ModelRole identifies the semantic role of a model.
type ModelRole string

const (
	RolePrimary ModelRole = "primary"
	RoleReview  ModelRole = "review"
	RoleSummary ModelRole = "summary"
	RoleEmbed   ModelRole = "embed"
)

// ResolvedModel is a provider + model name pair ready to use.
type ResolvedModel struct {
	Provider LLMProvider
	Model    string
}

// Registry holds all configured providers and resolves models by role.
type Registry struct {
	providers       map[string]LLMProvider
	configs         map[string]config.ProviderConfig
	circuitBreakers map[string]*CircuitBreaker
}

// NewRegistry creates providers from config.
func NewRegistry(cfgProviders map[string]config.ProviderConfig) (*Registry, error) {
	r := &Registry{
		providers:       make(map[string]LLMProvider),
		configs:         make(map[string]config.ProviderConfig),
		circuitBreakers: make(map[string]*CircuitBreaker),
	}

	for name, pcfg := range cfgProviders {
		p, err := createProvider(name, pcfg)
		if err != nil {
			return nil, fmt.Errorf("creating provider %q: %w", name, err)
		}
		r.providers[name] = p
		r.configs[name] = pcfg
		r.circuitBreakers[name] = NewCircuitBreaker(0, 0) // defaults: threshold=5, open=30s
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

// GetForRole returns the provider and model name for the given semantic role
// on the named provider. Falls back to DefaultModel if no role config is set.
func (r *Registry) GetForRole(providerName string, role ModelRole) (ResolvedModel, error) {
	p, ok := r.providers[providerName]
	if !ok {
		return ResolvedModel{}, fmt.Errorf("provider %q not found", providerName)
	}
	cfg, ok := r.configs[providerName]
	if !ok {
		return ResolvedModel{}, fmt.Errorf("config for provider %q not found", providerName)
	}

	model := r.modelForRole(cfg, role)
	if model == "" {
		model = cfg.DefaultModel
	}
	return ResolvedModel{Provider: p, Model: model}, nil
}

// GetWithFallback tries the primary provider; on retriable error, tries the
// error_fallback provider configured for the primary's role.
//
// Circuit breakers are consulted before selecting a provider:
//   - If the primary provider's circuit is open, it is skipped and the fallback
//     is tried immediately (if configured).
//   - apiErr == nil signals a successful call: RecordSuccess is called on the
//     primary's circuit breaker.
//   - A retriable apiErr signals a transient failure: RecordFailure is called on
//     the primary's circuit breaker.
//
// Returns the ResolvedModel to use and a boolean indicating if fallback was used.
func (r *Registry) GetWithFallback(providerName string, role ModelRole, apiErr error) (ResolvedModel, bool, error) {
	// Check the primary provider's circuit breaker first.
	primaryCB := r.circuitBreakers[providerName]
	if primaryCB != nil && !primaryCB.Allow() {
		// Circuit is open — skip primary and attempt fallback directly.
		cfg := r.configs[providerName]
		fallbackName := r.errorFallbackProvider(cfg, role)
		if fallbackName == "" {
			return ResolvedModel{}, false, ErrCircuitOpen
		}
		fallback, err := r.GetForRole(fallbackName, role)
		if err != nil {
			return ResolvedModel{}, false, fmt.Errorf("error_fallback provider %q: %w", fallbackName, err)
		}
		return fallback, true, nil
	}

	primary, err := r.GetForRole(providerName, role)
	if err != nil {
		return ResolvedModel{}, false, err
	}

	if apiErr == nil {
		// Successful call — record success on the primary circuit breaker.
		if primaryCB != nil {
			primaryCB.RecordSuccess()
		}
		return primary, false, nil
	}

	if !isRetriableError(apiErr) {
		return ResolvedModel{}, false, apiErr
	}

	// Extract ProviderError for the RetryAfter hint if available.
	_ = apiErr // hint accessible via errors.As at the call site

	// Retriable error — record failure before attempting fallback.
	if primaryCB != nil {
		primaryCB.RecordFailure()
	}

	// Look up error_fallback provider.
	cfg := r.configs[providerName]
	fallbackName := r.errorFallbackProvider(cfg, role)
	if fallbackName == "" {
		return ResolvedModel{}, false, apiErr
	}

	fallback, err := r.GetForRole(fallbackName, role)
	if err != nil {
		return ResolvedModel{}, false, fmt.Errorf("error_fallback provider %q: %w", fallbackName, err)
	}
	return fallback, true, nil
}

// RetryConfig returns the retry settings for a provider (with defaults).
func (r *Registry) RetryConfig(providerName string) (maxRetries int, minWait, maxWait time.Duration) {
	cfg, ok := r.configs[providerName]
	if !ok {
		return 3, 2 * time.Second, 30 * time.Second
	}
	max := cfg.RetryMax
	if max == 0 {
		max = 3
	}
	min := cfg.RetryMinWait
	if min == 0 {
		min = 2
	}
	maxW := cfg.RetryMaxWait
	if maxW == 0 {
		maxW = 30
	}
	return max, time.Duration(min) * time.Second, time.Duration(maxW) * time.Second
}

// ExponentialBackoff returns the wait duration for the given retry attempt.
func ExponentialBackoff(attempt int, minWait, maxWait time.Duration) time.Duration {
	wait := minWait * (1 << uint(attempt))
	if wait > maxWait {
		wait = maxWait
	}
	// Add up to 25% jitter
	jitter := time.Duration(rand.Int63n(int64(wait / 4))) //nolint:gosec
	return wait + jitter
}

// ExponentialBackoffWithHint respects a server-suggested Retry-After delay
// (plus 500 ms buffer) when present. Falls back to ExponentialBackoff otherwise.
func ExponentialBackoffWithHint(attempt int, minWait, maxWait time.Duration, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		hint := retryAfter + 500*time.Millisecond
		if hint > maxWait {
			return maxWait
		}
		return hint
	}
	return ExponentialBackoff(attempt, minWait, maxWait)
}

// modelForRole extracts the model name for a semantic role from config.
func (r *Registry) modelForRole(cfg config.ProviderConfig, role ModelRole) string {
	switch role {
	case RolePrimary:
		return cfg.Roles.Primary.Model
	case RoleReview:
		return cfg.Roles.Review.Model
	case RoleSummary:
		return cfg.Roles.Summary.Model
	case RoleEmbed:
		return cfg.Roles.Embed.Model
	}
	return ""
}

// errorFallbackProvider returns the error_fallback provider name for a role.
func (r *Registry) errorFallbackProvider(cfg config.ProviderConfig, role ModelRole) string {
	switch role {
	case RolePrimary:
		return cfg.Roles.Primary.ErrorFallbackProvider
	case RoleReview:
		return cfg.Roles.Review.ErrorFallbackProvider
	case RoleSummary:
		return cfg.Roles.Summary.ErrorFallbackProvider
	}
	return ""
}

// isRetriableError returns true for transient API errors worth retrying.
// It uses ClassifyError for ProviderError values and falls back to heuristic
// message matching for generic errors.
func isRetriableError(err error) bool {
	if err == nil {
		return false
	}
	// ProviderError carries an explicit ErrorKind — use it directly.
	var pe *ProviderError
	if errors.As(err, &pe) {
		return pe.Kind.IsRetriable()
	}
	// HTTP status-based check for other wrapped errors.
	var httpErr interface{ StatusCode() int }
	if errors.As(err, &httpErr) {
		kind := ClassifyError(httpErr.StatusCode(), err.Error())
		return kind.IsRetriable()
	}
	msg := err.Error()
	return contains(msg, "rate limit", "overloaded", "timeout", "connection refused", "EOF")
}

func contains(s string, subs ...string) bool {
	lower := toLower(s)
	for _, sub := range subs {
		if indexOf(lower, sub) >= 0 {
			return true
		}
	}
	return false
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func indexOf(s, sub string) int {
	if len(sub) > len(s) {
		return -1
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func createProvider(name string, cfg config.ProviderConfig) (LLMProvider, error) {
	switch cfg.Type {
	case "ollama":
		return NewOllama(name, cfg.BaseURL, cfg.DefaultModel), nil
	case "claude":
		return NewClaude(name, cfg.APIKey, cfg.DefaultModel), nil
	case "gemini":
		return NewGemini(name, cfg.APIKey, cfg.DefaultModel), nil
	case "openai":
		return NewOpenAI(name, cfg.APIKey, "", cfg.DefaultModel), nil
	case "deepseek":
		return NewDeepSeek(name, cfg.APIKey, cfg.DefaultModel), nil
	case "groq":
		return NewGroq(name, cfg.APIKey, cfg.DefaultModel), nil
	case "mistral":
		return NewMistral(name, cfg.APIKey, cfg.DefaultModel), nil
	case "openrouter":
		return NewOpenRouter(name, cfg.APIKey, cfg.DefaultModel), nil
	case "xai":
		return NewXAI(name, cfg.APIKey, cfg.DefaultModel), nil
	case "openai-compatible":
		return NewOpenAI(name, cfg.APIKey, cfg.BaseURL, cfg.DefaultModel), nil
	case "glm":
		return NewGLM(name, cfg.APIKey, cfg.DefaultModel), nil
	case "minimax":
		return NewMiniMax(name, cfg.APIKey, cfg.DefaultModel), nil
	case "kimi":
		return NewKimi(name, cfg.APIKey, cfg.DefaultModel), nil
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", cfg.Type)
	}
}
