package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// ModelInfo describes a model returned by the provider's /models endpoint.
type ModelInfo struct {
	ID      string
	Created time.Time
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
		info := ModelInfo{ID: m.ID}
		if m.Created > 0 {
			info.Created = time.Unix(m.Created, 0)
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
