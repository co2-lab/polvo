package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"time"

	ollamaapi "github.com/ollama/ollama/api"
)

type modelsRequest struct {
	Type    string `json:"type"`
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req modelsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	var models []string
	var fetchErr error

	switch req.Type {
	case "claude":
		models, fetchErr = fetchClaudeModels(ctx, req.APIKey)
	case "openai", "openai-compatible":
		models, fetchErr = fetchOpenAIModels(ctx, req.APIKey, req.BaseURL)
	case "ollama":
		models, fetchErr = fetchOllamaModels(ctx, req.BaseURL)
	case "gemini":
		models, fetchErr = fetchGeminiModels(ctx, req.APIKey)
	default:
		writeJSON(w, []string{})
		return
	}

	if fetchErr != nil {
		http.Error(w, fetchErr.Error(), http.StatusBadGateway)
		return
	}

	writeJSON(w, models)
}

func fetchClaudeModels(ctx context.Context, apiKey string) ([]string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.anthropic.com/v1/models", nil)
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := make([]string, 0, len(result.Data))
	for _, m := range result.Data {
		models = append(models, m.ID)
	}
	return models, nil
}

func fetchOpenAIModels(ctx context.Context, apiKey, baseURL string) ([]string, error) {
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := make([]string, 0, len(result.Data))
	for _, m := range result.Data {
		models = append(models, m.ID)
	}
	sort.Strings(models)
	return models, nil
}

func fetchOllamaModels(ctx context.Context, baseURL string) ([]string, error) {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	client := ollamaapi.NewClient(u, http.DefaultClient)

	list, err := client.List(ctx)
	if err != nil {
		return nil, err
	}

	models := make([]string, 0, len(list.Models))
	for _, m := range list.Models {
		models = append(models, m.Name)
	}
	return models, nil
}

func fetchGeminiModels(ctx context.Context, apiKey string) ([]string, error) {
	url := "https://generativelanguage.googleapis.com/v1beta/models?key=" + apiKey
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := make([]string, 0, len(result.Models))
	for _, m := range result.Models {
		// strip "models/" prefix
		name := m.Name
		if len(name) > 7 && name[:7] == "models/" {
			name = name[7:]
		}
		models = append(models, name)
	}
	return models, nil
}
