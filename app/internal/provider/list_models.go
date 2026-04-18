package provider

//go:generate go run ./gen/gen_swe_scores.go

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ModelInfo describes a model returned by the provider's /models endpoint.
type ModelInfo struct {
	ID            string
	Created       time.Time
	ContextWindow int     // max input tokens; 0 = unknown
	PricingInput  float64 // USD per million input tokens; 0 = unknown
	PricingOutput float64 // USD per million output tokens; 0 = unknown
	IsFree        bool    // true when the model is available at no cost
	SWEScore      float64 // SWE-bench Verified score (0–100); 0 = unknown
	LCBScore      float64 // LiveCodeBench score (0–100); 0 = unknown
	IOIScore      float64 // IOI algorithmic coding score (0–100); 0 = unknown
}

// lookupScore returns the score for modelID from the given map.
// It tries: exact match, then longest prefix match, then strips the
// "provider/" prefix and retries (for IDs like "claude-sonnet-4-6"
// matching "anthropic/claude-sonnet-4-6").
func lookupScore(m map[string]float64, modelID string) float64 {
	lower := strings.ToLower(modelID)
	// 1. Exact match.
	if v, ok := m[lower]; ok {
		return v
	}
	// 2. Longest prefix match (handles versioned suffixes like -20250929).
	best, bestLen := 0.0, 0
	for k, v := range m {
		if strings.HasPrefix(lower, k) && len(k) > bestLen {
			best, bestLen = v, len(k)
		}
	}
	if bestLen > 0 {
		return best
	}
	// 3. Strip provider prefix from map keys and retry.
	for k, v := range m {
		bare := k
		if i := strings.Index(k, "/"); i >= 0 {
			bare = k[i+1:]
		}
		if strings.HasPrefix(lower, bare) && len(bare) > bestLen {
			best, bestLen = v, len(bare)
		}
	}
	return best
}

// ModelLister is implemented by providers that can list available models.
type ModelLister interface {
	ListModels(ctx context.Context) ([]ModelInfo, error)
}

// oaiModelList is the OpenAI-compatible /models response.
type oaiModelList struct {
	Data []struct {
		ID      string `json:"id"`
		Created int64  `json:"created"` // unix timestamp; 0 if absent
		Object  string `json:"object"`
		// OpenRouter extras (ignored by other providers).
		ContextLength int `json:"context_length"`
		Pricing *struct {
			Prompt     string `json:"prompt"`     // USD per token as string, e.g. "0.000001"
			Completion string `json:"completion"` // USD per token as string
		} `json:"pricing"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// ListModels fetches available models from the provider's /models endpoint.
// Results are sorted newest-first (by created timestamp; alphabetically when
// timestamps are absent or equal).
func (p *openAIProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	p.setHeaders(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: listing models: %w", p.name, err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: reading models response: %w", p.name, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: listing models: HTTP %d", p.name, resp.StatusCode)
	}

	var list oaiModelList
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, fmt.Errorf("%s: decoding models: %w", p.name, err)
	}
	if list.Error != nil {
		return nil, fmt.Errorf("%s: listing models: %s", p.name, list.Error.Message)
	}

	models := make([]ModelInfo, 0, len(list.Data))
	for _, m := range list.Data {
		// Skip embedding / image / audio / moderation models — keep only
		// chat/text generation models.
		if isNonChatModel(m.ID) {
			continue
		}
		info := ModelInfo{
			ID:       m.ID,
			SWEScore: lookupScore(sweScores, m.ID),
			LCBScore: lookupScore(lcbScores, m.ID),
			IOIScore: lookupScore(ioiScores, m.ID),
		}
		if m.Created > 0 {
			info.Created = time.Unix(m.Created, 0)
		}
		if m.ContextLength > 0 {
			info.ContextWindow = m.ContextLength
		}
		if m.Pricing != nil {
			// OpenRouter expresses pricing as USD per token; convert to per million.
			if v, err := strconv.ParseFloat(m.Pricing.Prompt, 64); err == nil && v > 0 {
				info.PricingInput = v * 1_000_000
			}
			if v, err := strconv.ParseFloat(m.Pricing.Completion, 64); err == nil && v > 0 {
				info.PricingOutput = v * 1_000_000
			}
			info.IsFree = m.Pricing.Prompt == "0" && m.Pricing.Completion == "0"
		}
		models = append(models, info)
	}

	// Sort: newest first (by created desc), then alphabetically.
	sort.Slice(models, func(i, j int) bool {
		ci, cj := models[i].Created, models[j].Created
		if !ci.IsZero() && !cj.IsZero() {
			if !ci.Equal(cj) {
				return ci.After(cj)
			}
		} else if !ci.IsZero() {
			return true // dated models before undated
		} else if !cj.IsZero() {
			return false
		}
		return models[i].ID < models[j].ID
	})

	return models, nil
}

// isNonChatModel returns true for model IDs that are clearly not chat/text
// generation models (embeddings, image gen, TTS, STT, moderation, etc.).
func isNonChatModel(id string) bool {
	lower := strings.ToLower(id)
	for _, skip := range []string{
		"embed", "embedding",
		"tts", "whisper", "speech",
		"dall-e", "dall_e", "image",
		"moderation",
		"babbage", "davinci", "ada", "curie", // legacy completions
		"cogview", "glm-ocr",                 // Zhipu image/ocr
		"abab5",                              // MiniMax legacy
	} {
		if strings.Contains(lower, skip) {
			return true
		}
	}
	return false
}
