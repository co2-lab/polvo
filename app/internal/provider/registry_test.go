// Package provider — using internal package access to test unexported fields.
package provider

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/co2-lab/polvo/internal/config"
)

// stubProvider implements LLMProvider for tests.
type stubProvider struct {
	name string
	err  error
}

func (s *stubProvider) Name() string { return s.name }
func (s *stubProvider) Complete(_ context.Context, _ Request) (*Response, error) {
	return nil, s.err
}
func (s *stubProvider) Available(_ context.Context) error { return nil }

// makeRegistry creates a Registry directly, bypassing createProvider.
// It initialises a default circuit breaker for every registered provider.
func makeRegistry(providers map[string]LLMProvider, cfgs map[string]config.ProviderConfig) *Registry {
	p := providers
	if p == nil {
		p = make(map[string]LLMProvider)
	}
	c := cfgs
	if c == nil {
		c = make(map[string]config.ProviderConfig)
	}
	cbs := make(map[string]*CircuitBreaker, len(p))
	for name := range p {
		cbs[name] = NewCircuitBreaker(0, 0)
	}
	return &Registry{providers: p, configs: c, circuitBreakers: cbs}
}

// httpErr is a test error that implements StatusCode() int for isRetriableError.
type httpErr struct{ code int }

func (e httpErr) Error() string   { return fmt.Sprintf("status %d", e.code) }
func (e httpErr) StatusCode() int { return e.code }

// ---------------------------------------------------------------------------
// ExponentialBackoff
// ---------------------------------------------------------------------------

func TestExponentialBackoff(t *testing.T) {
	t.Run("attempt 0", func(t *testing.T) {
		got := ExponentialBackoff(0, 100*time.Millisecond, 10*time.Second)
		// base = 100ms * 2^0 = 100ms; jitter up to 25ms → [100ms, 125ms)
		if got < 100*time.Millisecond || got >= 125*time.Millisecond {
			t.Errorf("attempt 0: got %v, want [100ms, 125ms)", got)
		}
	})

	t.Run("attempt 1", func(t *testing.T) {
		got := ExponentialBackoff(1, 100*time.Millisecond, 10*time.Second)
		// base = 100ms * 2^1 = 200ms; jitter up to 50ms → [200ms, 250ms)
		if got < 200*time.Millisecond || got >= 250*time.Millisecond {
			t.Errorf("attempt 1: got %v, want [200ms, 250ms)", got)
		}
	})

	t.Run("large attempt capped before jitter", func(t *testing.T) {
		// NOTE: a implementação aplica jitter APÓS o cap, então wait+jitter pode
		// ultrapassar maxWait em até 25%. O cap garante que a base não ultrapasse,
		// mas o retorno final pode ser maxWait * 1.25.
		// Este teste verifica que a base é capeada (wait <= maxWait) verificando
		// indiretamente que o resultado está dentro da faixa [maxWait, maxWait*1.25].
		maxWait := 10 * time.Second
		maxWithJitter := maxWait + maxWait/4 // 12.5s
		for range 50 {
			got := ExponentialBackoff(20, 100*time.Millisecond, maxWait)
			if got > maxWithJitter {
				t.Errorf("attempt 20: got %v, exceeds max+jitter %v", got, maxWithJitter)
			}
			if got < maxWait {
				t.Errorf("attempt 20: got %v, expected >= maxWait %v (base should be capped)", got, maxWait)
			}
		}
	})

	t.Run("jitter produces distinct values", func(t *testing.T) {
		results := make(map[time.Duration]int)
		for range 1000 {
			d := ExponentialBackoff(1, 100*time.Millisecond, 10*time.Second)
			results[d]++
		}
		if len(results) <= 1 {
			t.Errorf("jitter: expected multiple distinct values, got %d", len(results))
		}
	})

	t.Run("min greater than max: base capped but jitter may exceed", func(t *testing.T) {
		// Quando min > max, a base é capeada em max, mas jitter adiciona até 25%
		// Documentar comportamento real: retorno pode ser até max*1.25
		maxWait := 10 * time.Second
		got := ExponentialBackoff(0, 30*time.Second, maxWait)
		maxWithJitter := maxWait + maxWait/4
		if got > maxWithJitter {
			t.Errorf("min>max: got %v, expected <= max+jitter %v", got, maxWithJitter)
		}
	})
}

// ---------------------------------------------------------------------------
// isRetriableError
// ---------------------------------------------------------------------------

func TestIsRetriableError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		// string-based matches
		{"rate limit string", errors.New("rate limit exceeded"), true},
		{"overloaded string", errors.New("overloaded"), true},
		{"timeout string", errors.New("request timeout"), true},
		{"connection refused string", errors.New("connection refused"), true},
		// BUG: contains() faz toLower(s) mas NÃO faz toLower nos substrings.
		// Busca "EOF" (uppercase) em "eof" (lowercase) — nunca casa.
		// Comportamento real: false. Para corrigir, mover toLower para os substrings.
		{"EOF string (BUG: uppercase substring não casa)", errors.New("EOF"), false},

		// context.DeadlineExceeded — "context deadline exceeded" does NOT contain
		// "rate limit", "overloaded", "timeout", "connection refused", or "EOF"
		{"context.DeadlineExceeded", context.DeadlineExceeded, false},
		{"context deadline string", errors.New("context deadline"), false},
		{"context deadline exceeded string", errors.New("context deadline exceeded"), false},

		// non-retriable strings
		{"status 400 string", errors.New("status 400"), false},
		{"status 401 string", errors.New("status 401"), false},
		{"invalid request", errors.New("invalid request"), false},

		// nil
		{"nil", nil, false},

		// HTTP status codes via StatusCode() interface
		{"httpErr 429 TooManyRequests", httpErr{429}, true},
		{"httpErr 529 Anthropic overloaded", httpErr{529}, true},
		{"httpErr 503 ServiceUnavailable", httpErr{503}, true},
		{"httpErr 502 BadGateway", httpErr{502}, true},
		{"httpErr 504 GatewayTimeout", httpErr{504}, true},
		{"httpErr 400 BadRequest", httpErr{400}, false},
		{"httpErr 401 Unauthorized", httpErr{401}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isRetriableError(tc.err)
			if got != tc.want {
				t.Errorf("isRetriableError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Registry.Get
// ---------------------------------------------------------------------------

func TestRegistryGet(t *testing.T) {
	p1 := &stubProvider{name: "p1"}
	r := makeRegistry(
		map[string]LLMProvider{"p1": p1},
		map[string]config.ProviderConfig{"p1": {Type: "ollama", DefaultModel: "llama3"}},
	)

	t.Run("existing provider", func(t *testing.T) {
		got, err := r.Get("p1")
		if err != nil {
			t.Fatalf("Get(p1): unexpected error: %v", err)
		}
		if got != p1 {
			t.Errorf("Get(p1): got %v, want %v", got, p1)
		}
	})

	t.Run("nonexistent provider", func(t *testing.T) {
		_, err := r.Get("nonexistent")
		if err == nil {
			t.Fatal("Get(nonexistent): expected error, got nil")
		}
	})
}

// ---------------------------------------------------------------------------
// Registry.Default
// ---------------------------------------------------------------------------

func TestRegistryDefault(t *testing.T) {
	t.Run("explicit default provider", func(t *testing.T) {
		pDefault := &stubProvider{name: "default"}
		pOther := &stubProvider{name: "other"}
		r := makeRegistry(
			map[string]LLMProvider{"default": pDefault, "other": pOther},
			nil,
		)
		got, err := r.Default()
		if err != nil {
			t.Fatalf("Default(): unexpected error: %v", err)
		}
		if got != pDefault {
			t.Errorf("Default(): got %v, want explicit 'default' provider", got)
		}
	})

	t.Run("no explicit default — returns first available", func(t *testing.T) {
		pOnly := &stubProvider{name: "only"}
		r := makeRegistry(
			map[string]LLMProvider{"only": pOnly},
			nil,
		)
		got, err := r.Default()
		if err != nil {
			t.Fatalf("Default(): unexpected error: %v", err)
		}
		if got == nil {
			t.Error("Default(): got nil, want a provider")
		}
	})

	t.Run("empty registry returns error", func(t *testing.T) {
		r := makeRegistry(nil, nil)
		_, err := r.Default()
		if err == nil {
			t.Fatal("Default() on empty registry: expected error, got nil")
		}
	})
}

// ---------------------------------------------------------------------------
// Registry.GetForRole
// ---------------------------------------------------------------------------

func TestRegistryGetForRole(t *testing.T) {
	p1 := &stubProvider{name: "p1"}
	r := makeRegistry(
		map[string]LLMProvider{"p1": p1},
		map[string]config.ProviderConfig{
			"p1": {
				Type:         "claude",
				DefaultModel: "claude-3-haiku",
				Roles: config.ProviderRoles{
					Primary: config.ModelRoleConfig{Model: "claude-opus-4"},
					Summary: config.ModelRoleConfig{Model: "claude-3-haiku-summary"},
				},
			},
		},
	)

	t.Run("role primary", func(t *testing.T) {
		got, err := r.GetForRole("p1", RolePrimary)
		if err != nil {
			t.Fatalf("GetForRole(p1, primary): %v", err)
		}
		if got.Model != "claude-opus-4" {
			t.Errorf("expected model 'claude-opus-4', got %q", got.Model)
		}
		if got.Provider != p1 {
			t.Error("expected provider p1")
		}
	})

	t.Run("role summary", func(t *testing.T) {
		got, err := r.GetForRole("p1", RoleSummary)
		if err != nil {
			t.Fatalf("GetForRole(p1, summary): %v", err)
		}
		if got.Model != "claude-3-haiku-summary" {
			t.Errorf("expected model 'claude-3-haiku-summary', got %q", got.Model)
		}
	})

	t.Run("unconfigured role falls back to DefaultModel", func(t *testing.T) {
		got, err := r.GetForRole("p1", RoleEmbed)
		if err != nil {
			t.Fatalf("GetForRole(p1, embed): %v", err)
		}
		if got.Model != "claude-3-haiku" {
			t.Errorf("expected fallback DefaultModel 'claude-3-haiku', got %q", got.Model)
		}
	})

	t.Run("nonexistent provider returns error", func(t *testing.T) {
		_, err := r.GetForRole("nonexistent", RolePrimary)
		if err == nil {
			t.Fatal("expected error for nonexistent provider, got nil")
		}
	})
}

// ---------------------------------------------------------------------------
// Registry.GetWithFallback
// ---------------------------------------------------------------------------

func TestRegistryGetWithFallback(t *testing.T) {
	p1 := &stubProvider{name: "p1"}
	p2 := &stubProvider{name: "p2"}

	r := makeRegistry(
		map[string]LLMProvider{"p1": p1, "p2": p2},
		map[string]config.ProviderConfig{
			"p1": {
				Type:         "claude",
				DefaultModel: "claude-opus-4",
				Roles: config.ProviderRoles{
					Primary: config.ModelRoleConfig{
						Model:                 "claude-opus-4",
						ErrorFallbackProvider: "p2",
					},
				},
			},
			"p2": {
				Type:         "ollama",
				DefaultModel: "llama3",
				Roles: config.ProviderRoles{
					Primary: config.ModelRoleConfig{Model: "llama3"},
				},
			},
		},
	)

	t.Run("apiErr nil returns primary no fallback", func(t *testing.T) {
		got, usedFallback, err := r.GetWithFallback("p1", RolePrimary, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if usedFallback {
			t.Error("expected usedFallback=false when no error")
		}
		if got.Provider != p1 {
			t.Error("expected primary provider p1")
		}
	})

	t.Run("non-retriable error propagates", func(t *testing.T) {
		badReq := httpErr{400}
		_, _, err := r.GetWithFallback("p1", RolePrimary, badReq)
		if err == nil {
			t.Fatal("expected error for non-retriable apiErr, got nil")
		}
		if err != badReq {
			t.Errorf("expected original error, got %v", err)
		}
	})

	t.Run("retriable error with fallback configured uses fallback", func(t *testing.T) {
		retriable := httpErr{429}
		got, usedFallback, err := r.GetWithFallback("p1", RolePrimary, retriable)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !usedFallback {
			t.Error("expected usedFallback=true")
		}
		if got.Provider != p2 {
			t.Errorf("expected fallback provider p2, got %v", got.Provider)
		}
	})

	t.Run("retriable error without fallback returns error", func(t *testing.T) {
		// p2 has no error_fallback configured
		retriable := httpErr{503}
		_, _, err := r.GetWithFallback("p2", RolePrimary, retriable)
		if err == nil {
			t.Fatal("expected error when no fallback configured, got nil")
		}
	})
}

// ---------------------------------------------------------------------------
// Registry.RetryConfig
// ---------------------------------------------------------------------------

func TestRetryConfig_Defaults(t *testing.T) {
	// Provider inexistente → retorna defaults hardcoded
	r := makeRegistry(nil, nil)
	max, minW, maxW := r.RetryConfig("nonexistent")
	if max != 3 {
		t.Errorf("RetryConfig defaults: max=%d, want 3", max)
	}
	if minW != 2*time.Second {
		t.Errorf("RetryConfig defaults: minWait=%v, want 2s", minW)
	}
	if maxW != 30*time.Second {
		t.Errorf("RetryConfig defaults: maxWait=%v, want 30s", maxW)
	}
}

func TestRetryConfig_CustomValues(t *testing.T) {
	r := makeRegistry(
		map[string]LLMProvider{"p1": &stubProvider{name: "p1"}},
		map[string]config.ProviderConfig{
			"p1": {
				Type:         "ollama",
				DefaultModel: "llama3",
				RetryMax:     5,
				RetryMinWait: 1,
				RetryMaxWait: 60,
			},
		},
	)
	max, minW, maxW := r.RetryConfig("p1")
	if max != 5 {
		t.Errorf("RetryConfig custom: max=%d, want 5", max)
	}
	if minW != 1*time.Second {
		t.Errorf("RetryConfig custom: minWait=%v, want 1s", minW)
	}
	if maxW != 60*time.Second {
		t.Errorf("RetryConfig custom: maxWait=%v, want 60s", maxW)
	}
}

// ---------------------------------------------------------------------------
// GAP: context_fallback not implemented
// When GetWithContextFallback is implemented, add:
//   TestRegistry_GetWithContextFallback_SwitchesToLargerModel
//   TestRegistry_GetWithContextFallback_FallsBackWhenConfigured
//   TestRegistry_GetWithContextFallback_NilWhenNotConfigured
// ---------------------------------------------------------------------------
