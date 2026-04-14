package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const webSearchTimeout = 15 * time.Second

type webSearchInput struct {
	Query      string `json:"query"`
	NumResults int    `json:"num_results"`
}

type webSearchTool struct {
	apiKey   string // Brave Search API key (from config)
	endpoint string // defaults to Brave Search API
}

// NewWebSearch creates the web_search tool.
// apiKey is the Brave Search API key; empty disables web search.
func NewWebSearch(apiKey string) Tool {
	ep := "https://api.search.brave.com/res/v1/web/search"
	return &webSearchTool{apiKey: apiKey, endpoint: ep}
}

func (t *webSearchTool) Name() string { return "web_search" }

func (t *webSearchTool) Description() string {
	return "Search the web and return a list of results with title, URL, and snippet. Requires BRAVE_SEARCH_API_KEY configured."
}

func (t *webSearchTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query":       {"type": "string", "description": "Search query"},
			"num_results": {"type": "integer", "description": "Number of results (default 5, max 10)", "default": 5}
		},
		"required": ["query"]
	}`)
}

func (t *webSearchTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	if t.apiKey == "" {
		return ErrorResult("web_search requires BRAVE_SEARCH_API_KEY to be configured in polvo.yaml under settings.brave_api_key"), nil
	}

	var in webSearchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ErrorResult("invalid input: " + err.Error()), nil
	}
	if in.Query == "" {
		return ErrorResult("query is required"), nil
	}
	n := in.NumResults
	if n <= 0 {
		n = 5
	}
	if n > 10 {
		n = 10
	}

	ctx, cancel := context.WithTimeout(ctx, webSearchTimeout)
	defer cancel()

	reqURL := fmt.Sprintf("%s?q=%s&count=%d", t.endpoint, url.QueryEscape(in.Query), n)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return ErrorResult(fmt.Sprintf("building request: %v", err)), nil
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("X-Subscription-Token", t.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ErrorResult(fmt.Sprintf("search request failed: %v", err)), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return ErrorResult(fmt.Sprintf("search API returned HTTP %d", resp.StatusCode)), nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return ErrorResult(fmt.Sprintf("reading response: %v", err)), nil
	}

	// Parse Brave Search response
	var braveResp struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(body, &braveResp); err != nil {
		return ErrorResult(fmt.Sprintf("parsing search response: %v", err)), nil
	}

	if len(braveResp.Web.Results) == 0 {
		return &Result{Content: "no results found"}, nil
	}

	var sb strings.Builder
	for i, r := range braveResp.Web.Results {
		fmt.Fprintf(&sb, "%d. **%s**\n   %s\n   %s\n\n", i+1, r.Title, r.URL, r.Description)
	}
	return &Result{Content: strings.TrimSpace(sb.String())}, nil
}
