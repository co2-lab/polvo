package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/co2-lab/polvo/internal/assets"
)

// contextWindowCache implements the hybrid context window lookup cascade:
//  1. Anthropic API (exact, per model, needs API key)
//  2. LiteLLM embedded JSON snapshot (bundled at build time, refreshed to disk)
//  3. OpenRouter API (no auth, background fetch)
//  4. Static prefix table (modelContextPrefixes in pricing.go)
type contextWindowCache struct {
	// Tier 2: Anthropic API results — key = "apikey:modelID"
	anthropicMu    sync.RWMutex
	anthropicCache map[string]int

	// Tier 3: LiteLLM embedded snapshot
	litellmOnce  sync.Once
	litellmCache map[string]int

	// Tier 4: OpenRouter results
	openrouterMu      sync.RWMutex
	openrouterCache   map[string]int
	openrouterFetched sync.Once
}

var globalCWCache = &contextWindowCache{}

// lookup runs the full cascade for model, optionally using anthropicAPIKey for tier 2.
func (c *contextWindowCache) lookup(model, anthropicAPIKey string) int {
	// Tier 2: Anthropic API (if key provided)
	if anthropicAPIKey != "" && strings.HasPrefix(strings.ToLower(model), "claude-") {
		if v := c.lookupAnthropicAPI(model, anthropicAPIKey); v > 0 {
			return v
		}
	}
	// Tier 3: LiteLLM embedded snapshot
	if v := c.lookupLiteLLM(model); v > 0 {
		return v
	}
	// Tier 4: OpenRouter (non-blocking, returns 0 until fetch completes)
	if v := c.lookupOpenRouter(model); v > 0 {
		return v
	}
	// Tier 5: static prefix table
	return lookupContextWindowPrefix(model)
}

// ContextWindowForModelWithKey is like ContextWindowForModel but also queries
// the Anthropic API when an API key is provided, giving the most accurate result
// for Claude models.
func ContextWindowForModelWithKey(model, anthropicAPIKey string) int {
	return globalCWCache.lookup(model, anthropicAPIKey)
}

// --- Tier 2: Anthropic API ---

func (c *contextWindowCache) lookupAnthropicAPI(model, key string) int {
	cacheKey := key + ":" + model

	c.anthropicMu.RLock()
	v, ok := c.anthropicCache[cacheKey]
	c.anthropicMu.RUnlock()
	if ok {
		return v
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.anthropic.com/v1/models/"+model, nil)
	if err != nil {
		return 0
	}
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return 0
	}
	defer resp.Body.Close()

	var payload struct {
		MaxInputTokens int `json:"max_input_tokens"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0
	}

	if payload.MaxInputTokens > 0 {
		c.anthropicMu.Lock()
		if c.anthropicCache == nil {
			c.anthropicCache = make(map[string]int)
		}
		c.anthropicCache[cacheKey] = payload.MaxInputTokens
		c.anthropicMu.Unlock()
	}
	return payload.MaxInputTokens
}

// --- Tier 3: LiteLLM embedded snapshot ---

func (c *contextWindowCache) lookupLiteLLM(model string) int {
	c.litellmOnce.Do(func() {
		c.litellmCache = make(map[string]int)

		data, _ := assets.ReadModelPrices()

		// Prefer fresher disk cache when available.
		if diskData, err := readLiteLLMDiskCache(); err == nil {
			data = diskData
		}

		if len(data) > 0 {
			var raw map[string]struct {
				MaxInputTokens float64 `json:"max_input_tokens"`
			}
			if err := json.Unmarshal(data, &raw); err == nil {
				for k, v := range raw {
					if v.MaxInputTokens > 0 {
						c.litellmCache[k] = int(v.MaxInputTokens)
					}
				}
			}
		}

		// Background refresh — fetch fresh JSON from GitHub, write to disk.
		go refreshLiteLLMDiskCache()
	})

	lower := strings.ToLower(model)

	// Exact match.
	if v, ok := c.litellmCache[lower]; ok {
		return v
	}
	if v, ok := c.litellmCache[model]; ok {
		return v
	}

	// Longest prefix match (handles versioned IDs not in snapshot yet).
	best, bestLen := 0, 0
	for k, v := range c.litellmCache {
		if strings.HasPrefix(lower, k) && len(k) > bestLen {
			best, bestLen = v, len(k)
		}
	}
	return best
}

const liteLLMRemoteURL = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"

func liteLLMDiskCachePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".polvo", "cache", "model_prices.json")
}

func readLiteLLMDiskCache() ([]byte, error) {
	p := liteLLMDiskCachePath()
	if p == "" {
		return nil, os.ErrNotExist
	}
	info, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	// Treat as stale after 24 h.
	if time.Since(info.ModTime()) > 24*time.Hour {
		return nil, os.ErrNotExist
	}
	return os.ReadFile(p)
}

func refreshLiteLLMDiskCache() {
	p := liteLLMDiskCachePath()
	if p == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, liteLLMRemoteURL, nil)
	if err != nil {
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return
	}
	defer resp.Body.Close()

	// Parse and re-encode trimmed version (only max_input_tokens).
	var raw map[string]struct {
		MaxInputTokens float64 `json:"max_input_tokens"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return
	}
	type entry struct {
		MaxInputTokens float64 `json:"max_input_tokens"`
	}
	trimmed := make(map[string]entry, len(raw))
	for k, v := range raw {
		if v.MaxInputTokens > 0 {
			trimmed[k] = v
		}
	}
	b, err := json.Marshal(trimmed)
	if err != nil {
		return
	}

	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	// Write atomically via temp file.
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return
	}
	os.Rename(tmp, p)
}

// --- Tier 4: OpenRouter ---

func (c *contextWindowCache) lookupOpenRouter(model string) int {
	// Kick off background fetch once.
	c.openrouterFetched.Do(func() {
		go c.fetchOpenRouter()
	})

	c.openrouterMu.RLock()
	defer c.openrouterMu.RUnlock()
	if c.openrouterCache == nil {
		return 0 // fetch not complete yet
	}

	lower := strings.ToLower(model)
	if v, ok := c.openrouterCache[lower]; ok {
		return v
	}
	// Strip provider prefix: "anthropic/claude-sonnet-4-6" → "claude-sonnet-4-6"
	if i := strings.Index(lower, "/"); i >= 0 {
		if v, ok := c.openrouterCache[lower[i+1:]]; ok {
			return v
		}
	}
	return 0
}

func (c *contextWindowCache) fetchOpenRouter() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://openrouter.ai/api/v1/models", nil)
	if err != nil {
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return
	}
	defer resp.Body.Close()

	var list oaiModelList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return
	}

	m := make(map[string]int, len(list.Data))
	for _, item := range list.Data {
		if item.ContextLength <= 0 {
			continue
		}
		id := strings.ToLower(item.ID)
		m[id] = item.ContextLength
		// Also index bare ID without provider prefix.
		if i := strings.Index(id, "/"); i >= 0 {
			bare := id[i+1:]
			if _, exists := m[bare]; !exists {
				m[bare] = item.ContextLength
			}
		}
	}

	c.openrouterMu.Lock()
	c.openrouterCache = m
	c.openrouterMu.Unlock()
}

// lookupContextWindowPrefix is the static prefix table fallback (tier 5).
func lookupContextWindowPrefix(model string) int {
	for _, p := range modelContextPrefixes {
		if strings.HasPrefix(model, p.prefix) {
			return p.tokens
		}
	}
	return 0
}
