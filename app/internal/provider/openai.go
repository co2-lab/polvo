package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// openAIProvider implements LLMProvider, ChatProvider, StreamProvider and
// CapabilitiesProvider using the OpenAI chat completions API.
// Compatible with: OpenAI, Gemini (v1beta/openai), DeepSeek, Groq, Mistral,
// OpenRouter, and any other OpenAI-compatible endpoint.
type openAIProvider struct {
	name         string
	apiKey       string
	baseURL      string
	defaultModel string
	providerType string // used for capability lookup
	client       *http.Client
}

// base URLs for known provider types.
const (
	openAIBaseURL     = "https://api.openai.com/v1"
	geminiBaseURL     = "https://generativelanguage.googleapis.com/v1beta/openai"
	deepseekBaseURL   = "https://api.deepseek.com/v1"
	groqBaseURL       = "https://api.groq.com/openai/v1"
	mistralBaseURL    = "https://api.mistral.ai/v1"
	openrouterBaseURL = "https://openrouter.ai/api/v1"
	xaiBaseURL        = "https://api.x.ai/v1"
	glmBaseURL        = "https://open.bigmodel.cn/api/paas/v4"
	minimaxBaseURL    = "https://api.minimax.chat/v1"
	kimiBaseURL       = "https://api.moonshot.cn/v1"
)

func NewOpenAI(name, apiKey, baseURL, defaultModel string) *openAIProvider {
	if baseURL == "" {
		baseURL = openAIBaseURL
	}
	if defaultModel == "" {
		defaultModel = "gpt-4o"
	}
	return &openAIProvider{
		name:         name,
		apiKey:       apiKey,
		baseURL:      strings.TrimRight(baseURL, "/"),
		defaultModel: defaultModel,
		providerType: "openai",
		client:       &http.Client{},
	}
}

func NewGemini(name, apiKey, defaultModel string) *openAIProvider {
	if defaultModel == "" {
		defaultModel = "gemini-2.5-pro"
	}
	return &openAIProvider{name: name, apiKey: apiKey, baseURL: geminiBaseURL, defaultModel: defaultModel, providerType: "gemini", client: &http.Client{}}
}

func NewDeepSeek(name, apiKey, defaultModel string) *openAIProvider {
	if defaultModel == "" {
		defaultModel = "deepseek-chat"
	}
	return &openAIProvider{name: name, apiKey: apiKey, baseURL: deepseekBaseURL, defaultModel: defaultModel, providerType: "deepseek", client: &http.Client{}}
}

func NewGroq(name, apiKey, defaultModel string) *openAIProvider {
	if defaultModel == "" {
		defaultModel = "llama-3.3-70b-versatile"
	}
	return &openAIProvider{name: name, apiKey: apiKey, baseURL: groqBaseURL, defaultModel: defaultModel, providerType: "groq", client: &http.Client{}}
}

func NewMistral(name, apiKey, defaultModel string) *openAIProvider {
	if defaultModel == "" {
		defaultModel = "mistral-large-latest"
	}
	return &openAIProvider{name: name, apiKey: apiKey, baseURL: mistralBaseURL, defaultModel: defaultModel, providerType: "mistral", client: &http.Client{}}
}

func NewOpenRouter(name, apiKey, defaultModel string) *openAIProvider {
	if defaultModel == "" {
		defaultModel = "anthropic/claude-sonnet-4-6"
	}
	return &openAIProvider{name: name, apiKey: apiKey, baseURL: openrouterBaseURL, defaultModel: defaultModel, providerType: "openrouter", client: &http.Client{}}
}

func NewXAI(name, apiKey, defaultModel string) *openAIProvider {
	if defaultModel == "" {
		defaultModel = "grok-3"
	}
	return &openAIProvider{name: name, apiKey: apiKey, baseURL: xaiBaseURL, defaultModel: defaultModel, providerType: "xai", client: &http.Client{}}
}

func NewGLM(name, apiKey, defaultModel string) *openAIProvider {
	if defaultModel == "" {
		defaultModel = "glm-4-plus"
	}
	return &openAIProvider{name: name, apiKey: apiKey, baseURL: glmBaseURL, defaultModel: defaultModel, providerType: "glm", client: &http.Client{}}
}

func NewMiniMax(name, apiKey, defaultModel string) *openAIProvider {
	if defaultModel == "" {
		defaultModel = "MiniMax-Text-01"
	}
	return &openAIProvider{name: name, apiKey: apiKey, baseURL: minimaxBaseURL, defaultModel: defaultModel, providerType: "minimax", client: &http.Client{}}
}

func NewKimi(name, apiKey, defaultModel string) *openAIProvider {
	if defaultModel == "" {
		defaultModel = "moonshot-v1-8k"
	}
	return &openAIProvider{name: name, apiKey: apiKey, baseURL: kimiBaseURL, defaultModel: defaultModel, providerType: "kimi", client: &http.Client{}}
}

func (p *openAIProvider) Name() string { return p.name }

func (p *openAIProvider) Available(ctx context.Context) error {
	if p.apiKey == "" {
		return fmt.Errorf("%s: api_key not configured", p.name)
	}
	return nil
}

// Capabilities implements CapabilitiesProvider.
func (p *openAIProvider) Capabilities(model string) Capabilities {
	if model == "" {
		model = p.defaultModel
	}
	return DefaultCapabilities(p.providerType, model)
}

// ── OpenAI wire types ────────────────────────────────────────────────────────

type oaiMessage struct {
	Role       string      `json:"role"`
	Content    any         `json:"content"` // string or []oaiContentPart
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
	Name       string      `json:"name,omitempty"`
}


type oaiToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function oaiToolFunction `json:"function"`
}

type oaiToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type oaiTool struct {
	Type     string      `json:"type"`
	Function oaiFunction `json:"function"`
}

type oaiFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type oaiRequest struct {
	Model               string       `json:"model"`
	Messages            []oaiMessage `json:"messages"`
	Tools               []oaiTool    `json:"tools,omitempty"`
	MaxCompletionTokens int          `json:"max_completion_tokens,omitempty"`
	MaxTokens           int          `json:"max_tokens,omitempty"`
	Temperature         *float64     `json:"temperature,omitempty"` // nil = omit (required for o1/o3)
	Stream              bool         `json:"stream,omitempty"`
	Stop                []string     `json:"stop,omitempty"` // stop sequences (used for XML tool calling)
}

type oaiResponse struct {
	Choices []struct {
		Message      oaiMessage `json:"message"`
		Delta        oaiMessage `json:"delta"`
		FinishReason string     `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// ── Complete ─────────────────────────────────────────────────────────────────

func (p *openAIProvider) Complete(ctx context.Context, req Request) (*Response, error) {
	msgs := []oaiMessage{}
	if req.System != "" {
		msgs = append(msgs, oaiMessage{Role: "system", Content: req.System})
	}
	msgs = append(msgs, oaiMessage{Role: "user", Content: req.Prompt})

	body := oaiRequest{
		Model:    p.resolveModel(req.Model),
		Messages: msgs,
	}
	setMaxTokens(&body, req.MaxTokens, 4096)

	resp, err := p.do(ctx, body)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("%s: empty response", p.name)
	}
	content, _ := resp.Choices[0].Message.Content.(string)
	return &Response{
		Content: content,
		TokensUsed: TokenUsage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}, nil
}

// ── Chat ─────────────────────────────────────────────────────────────────────

func (p *openAIProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	model := p.resolveModel(req.Model)
	caps := DefaultCapabilities(p.providerType, model)

	system := req.System
	var tools []oaiTool
	if caps.SupportsTools {
		tools = convertToolsToOAI(req.Tools)
	} else if len(req.Tools) > 0 {
		// XML tool calling fallback: inject tool definitions into system prompt.
		system = system + XMLToolPrompt(req.Tools)
	}

	body := oaiRequest{
		Model:    model,
		Messages: p.convertMessages(system, req.Messages),
		Tools:    tools,
	}
	setMaxTokens(&body, req.MaxTokens, 16384)

	resp, err := p.do(ctx, body)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("%s: empty response", p.name)
	}

	result := p.buildChatResponse(resp)

	// When using XML tool calling, parse the text response for a tool call block.
	if !caps.SupportsTools && result.Message.Content != "" {
		if tc := ParseXMLToolCall(result.Message.Content); tc != nil {
			result.Message.ToolCalls = []ToolCall{*tc}
			result.StopReason = "tool_use"
		}
	}

	return result, nil
}

// ── ChatStream ────────────────────────────────────────────────────────────────

func (p *openAIProvider) ChatStream(ctx context.Context, req ChatRequest, handler func(StreamEvent)) (*ChatResponse, error) {
	model := p.resolveModel(req.Model)
	caps := DefaultCapabilities(p.providerType, model)

	system := req.System
	var tools []oaiTool
	if caps.SupportsTools {
		tools = convertToolsToOAI(req.Tools)
	} else if len(req.Tools) > 0 {
		system = system + XMLToolPrompt(req.Tools)
	}

	body := oaiRequest{
		Model:    model,
		Messages: p.convertMessages(system, req.Messages),
		Tools:    tools,
		Stream:   true,
	}
	// When using XML tool calling, stop generation after </tool_call> to avoid
	// the model generating extra text after the XML block.
	if !caps.SupportsTools && len(req.Tools) > 0 {
		body.Stop = []string{"</tool_call>"}
	}
	setMaxTokens(&body, req.MaxTokens, 16384)

	rawBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(rawBody))
	if err != nil {
		return nil, err
	}
	p.setHeaders(httpReq)

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s stream: %w", p.name, err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(httpResp.Body)
		msg := fmt.Sprintf("%s stream: HTTP %d: %s", p.name, httpResp.StatusCode, string(b))
		retryAfter := ParseRetryAfter(httpResp.Header.Get("Retry-After"), string(b))
		return nil, NewProviderError(httpResp.StatusCode, msg, retryAfter)
	}

	// Accumulate full response while streaming deltas (Builder = O(1) amortized).
	var fullContent strings.Builder
	var toolCalls []ToolCall
	finishReason := "end_turn"

	// Active partial tool call being assembled across chunks.
	type partialTool struct {
		id        string
		name      string
		arguments strings.Builder
	}
	partials := map[int]*partialTool{}

	scanner := bufio.NewScanner(httpResp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk oaiResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]
		if choice.FinishReason != "" {
			switch choice.FinishReason {
			case "tool_calls":
				finishReason = "tool_use"
			case "stop":
				finishReason = "end_turn"
			default:
				finishReason = choice.FinishReason
			}
		}

		// Text delta.
		if s, ok := choice.Delta.Content.(string); ok && s != "" {
			fullContent.WriteString(s)
			handler(StreamEvent{Type: "text_delta", TextDelta: s})
		}

		// Tool call deltas.
		for _, tc := range choice.Delta.ToolCalls {
			idx := 0 // OpenAI sends index; use first if not tracked
			_ = idx
			pt, exists := partials[0]
			if !exists || tc.ID != "" {
				pt = &partialTool{id: tc.ID, name: tc.Function.Name}
				partials[0] = pt
				if tc.ID != "" {
					handler(StreamEvent{
						Type:     "tool_use_start",
						ToolCall: &ToolCall{ID: tc.ID, Name: tc.Function.Name},
					})
				}
			}
			pt.arguments.WriteString(tc.Function.Arguments)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%s stream scanner: %w", p.name, err)
	}

	// Flush partial tool calls into result.
	for _, pt := range partials {
		if pt.id != "" {
			toolCalls = append(toolCalls, ToolCall{
				ID:    pt.id,
				Name:  pt.name,
				Input: json.RawMessage(pt.arguments.String()),
			})
		}
	}

	handler(StreamEvent{Type: "done"})

	result := &ChatResponse{StopReason: finishReason}
	result.Message.Role = "assistant"
	result.Message.Content = fullContent.String()
	result.Message.ToolCalls = toolCalls

	// When using XML tool calling, parse the accumulated text for a tool call block.
	if !caps.SupportsTools && len(toolCalls) == 0 && result.Message.Content != "" {
		if tc := ParseXMLToolCall(result.Message.Content); tc != nil {
			result.Message.ToolCalls = []ToolCall{*tc}
			result.StopReason = "tool_use"
		}
	}

	return result, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (p *openAIProvider) resolveModel(model string) string {
	if model != "" {
		return model
	}
	return p.defaultModel
}

func (p *openAIProvider) setHeaders(r *http.Request) {
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+p.apiKey)
}

func (p *openAIProvider) do(ctx context.Context, body oaiRequest) (*oaiResponse, error) {
	rawBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(rawBody))
	if err != nil {
		return nil, err
	}
	p.setHeaders(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.name, err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: reading response: %w", p.name, err)
	}

	var result oaiResponse
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, fmt.Errorf("%s: decoding response: %w", p.name, err)
	}

	// Build a ProviderError for non-200 responses so callers get ErrorKind
	// and the Retry-After hint parsed from the server response.
	if resp.StatusCode != http.StatusOK || result.Error != nil {
		msg := fmt.Sprintf("%s: HTTP %d: %s", p.name, resp.StatusCode, string(b))
		if result.Error != nil {
			msg = fmt.Sprintf("%s: %s", p.name, result.Error.Message)
		}
		retryAfter := ParseRetryAfter(resp.Header.Get("Retry-After"), string(b))
		return nil, NewProviderError(resp.StatusCode, msg, retryAfter)
	}

	return &result, nil
}

func (p *openAIProvider) convertMessages(system string, msgs []Message) []oaiMessage {
	var result []oaiMessage
	if system != "" {
		result = append(result, oaiMessage{Role: "system", Content: system})
	}
	for _, m := range msgs {
		switch m.Role {
		case "user":
			result = append(result, oaiMessage{Role: "user", Content: m.Content})
		case "assistant":
			om := oaiMessage{Role: "assistant", Content: m.Content}
			for _, tc := range m.ToolCalls {
				om.ToolCalls = append(om.ToolCalls, oaiToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: oaiToolFunction{
						Name:      tc.Name,
						Arguments: string(tc.Input),
					},
				})
			}
			result = append(result, om)
		case "tool":
			if m.ToolResult != nil {
				result = append(result, oaiMessage{
					Role:       "tool",
					Content:    m.ToolResult.Content,
					ToolCallID: m.ToolResult.ToolCallID,
				})
			}
		}
	}
	return result
}

func (p *openAIProvider) buildChatResponse(resp *oaiResponse) *ChatResponse {
	if len(resp.Choices) == 0 {
		return &ChatResponse{}
	}
	choice := resp.Choices[0]
	result := &ChatResponse{
		TokensUsed: TokenUsage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}
	switch choice.FinishReason {
	case "tool_calls":
		result.StopReason = "tool_use"
	case "stop":
		result.StopReason = "end_turn"
	default:
		result.StopReason = choice.FinishReason
	}

	result.Message.Role = "assistant"
	if s, ok := choice.Message.Content.(string); ok {
		result.Message.Content = s
	}
	for _, tc := range choice.Message.ToolCalls {
		result.Message.ToolCalls = append(result.Message.ToolCalls, ToolCall{
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: json.RawMessage(tc.Function.Arguments),
		})
	}
	return result
}

func convertToolsToOAI(tools []ToolDef) []oaiTool {
	if len(tools) == 0 {
		return nil
	}
	result := make([]oaiTool, len(tools))
	for i, t := range tools {
		result[i] = oaiTool{
			Type: "function",
			Function: oaiFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		}
	}
	return result
}

// setMaxTokens sets the right token field depending on the model family and
// strips parameters that reasoning models (o1/o3) do not support.
func setMaxTokens(body *oaiRequest, requested, fallback int) {
	n := requested
	if n == 0 {
		n = fallback
	}
	isReasoning := strings.HasPrefix(body.Model, "o1") || strings.HasPrefix(body.Model, "o3")
	if isReasoning {
		body.MaxCompletionTokens = n
		// o1/o3 do not support temperature — leave nil so it is omitted from JSON.
	} else {
		body.MaxTokens = n
	}
}
