package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultMaxChars    = 20000
	webFetchTimeout    = 30 * time.Second
	maxBodyBytes       = 5 << 20 // 5MB body read limit
)

type webFetchInput struct {
	URL      string `json:"url"`
	MaxChars int    `json:"max_chars"`
}

type webFetchTool struct{}

// NewWebFetch creates the web_fetch tool.
func NewWebFetch() Tool { return &webFetchTool{} }

func (t *webFetchTool) Name() string { return "web_fetch" }

func (t *webFetchTool) Description() string {
	return "Fetch the content of a URL and return it as plain text. HTML is stripped to readable text. Truncated to max_chars."
}

func (t *webFetchTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url":       {"type": "string", "description": "URL to fetch"},
			"max_chars": {"type": "integer", "description": "Maximum characters to return (default 20000)", "default": 20000}
		},
		"required": ["url"]
	}`)
}

func (t *webFetchTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in webFetchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ErrorResult("invalid input: " + err.Error()), nil
	}
	if in.URL == "" {
		return ErrorResult("url is required"), nil
	}
	maxChars := in.MaxChars
	if maxChars <= 0 {
		maxChars = defaultMaxChars
	}

	ctx, cancel := context.WithTimeout(ctx, webFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, in.URL, nil)
	if err != nil {
		return ErrorResult(fmt.Sprintf("building request: %v", err)), nil
	}
	req.Header.Set("User-Agent", "polvo-agent/1.0")
	req.Header.Set("Accept", "text/html,text/plain,application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ErrorResult(fmt.Sprintf("fetching URL: %v", err)), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return ErrorResult(fmt.Sprintf("HTTP %d from %s", resp.StatusCode, in.URL)), nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return ErrorResult(fmt.Sprintf("reading response body: %v", err)), nil
	}

	text := stripHTML(string(body))
	text = collapseWhitespace(text)

	if utf8.RuneCountInString(text) > maxChars {
		runes := []rune(text)
		text = string(runes[:maxChars]) + fmt.Sprintf("\n\n[truncated — %d chars omitted]", len(runes)-maxChars)
	}

	return &Result{Content: text}, nil
}

// stripHTML removes HTML tags and decodes common entities.
func stripHTML(s string) string {
	var sb strings.Builder
	inTag := false
	for i := 0; i < len(s); {
		r, size := rune(s[i]), 1
		if r >= utf8.RuneSelf {
			r2, sz := utf8.DecodeRuneInString(s[i:])
			r, size = r2, sz
		}
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
			sb.WriteByte(' ')
		case !inTag:
			sb.WriteRune(r)
		}
		i += size
	}
	// Decode common HTML entities
	text := sb.String()
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#39;", "'")
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	return text
}

// collapseWhitespace reduces consecutive whitespace to single newlines/spaces.
func collapseWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	var out []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}
