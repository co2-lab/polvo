package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	ollamaapi "github.com/ollama/ollama/api"
)

// OllamaProvider implements LLMProvider for Ollama.
type OllamaProvider struct {
	name         string
	baseURL      string
	defaultModel string
	client       *ollamaapi.Client
}

// NewOllama creates a new Ollama provider.
func NewOllama(name, baseURL, defaultModel string) *OllamaProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if defaultModel == "" {
		defaultModel = "codellama:13b"
	}

	u, _ := url.Parse(baseURL)
	client := ollamaapi.NewClient(u, http.DefaultClient)

	return &OllamaProvider{
		name:         name,
		baseURL:      baseURL,
		defaultModel: defaultModel,
		client:       client,
	}
}

func (p *OllamaProvider) Name() string { return p.name }

func (p *OllamaProvider) Complete(ctx context.Context, req Request) (*Response, error) {
	model := req.Model
	if model == "" {
		model = p.defaultModel
	}

	prompt := req.Prompt
	if req.System != "" {
		prompt = req.System + "\n\n" + req.Prompt
	}

	var fullResponse string
	stream := false
	err := p.client.Generate(ctx, &ollamaapi.GenerateRequest{
		Model:  model,
		Prompt: prompt,
		Stream: &stream,
	}, func(resp ollamaapi.GenerateResponse) error {
		fullResponse += resp.Response
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("ollama generate: %w", err)
	}

	return &Response{
		Content: fullResponse,
	}, nil
}

func (p *OllamaProvider) Available(ctx context.Context) error {
	_, err := p.client.List(ctx)
	if err != nil {
		return fmt.Errorf("ollama not available at %s: %w", p.baseURL, err)
	}
	return nil
}

func (p *OllamaProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = p.defaultModel
	}

	ollamaReq := &ollamaapi.ChatRequest{
		Model:    model,
		Messages: convertMessagesToOllama(req.Messages),
	}

	if req.System != "" {
		ollamaReq.Messages = append([]ollamaapi.Message{
			{Role: "system", Content: req.System},
		}, ollamaReq.Messages...)
	}

	if len(req.Tools) > 0 {
		ollamaReq.Tools = convertToolsToOllama(req.Tools)
	}

	stream := false
	ollamaReq.Stream = &stream

	var finalResp ollamaapi.ChatResponse
	err := p.client.Chat(ctx, ollamaReq, func(resp ollamaapi.ChatResponse) error {
		finalResp = resp
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("ollama chat: %w", err)
	}

	result := &ChatResponse{
		StopReason: "end_turn",
	}

	result.Message.Role = "assistant"
	result.Message.Content = finalResp.Message.Content

	if len(finalResp.Message.ToolCalls) > 0 {
		result.StopReason = "tool_use"
		for _, tc := range finalResp.Message.ToolCalls {
			argsJSON, _ := json.Marshal(tc.Function.Arguments)
			result.Message.ToolCalls = append(result.Message.ToolCalls, ToolCall{
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: json.RawMessage(argsJSON),
			})
		}
	}

	return result, nil
}

func (p *OllamaProvider) ChatStream(ctx context.Context, req ChatRequest, handler func(StreamEvent)) (*ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = p.defaultModel
	}

	ollamaReq := &ollamaapi.ChatRequest{
		Model:    model,
		Messages: convertMessagesToOllama(req.Messages),
	}

	if req.System != "" {
		ollamaReq.Messages = append([]ollamaapi.Message{
			{Role: "system", Content: req.System},
		}, ollamaReq.Messages...)
	}

	if len(req.Tools) > 0 {
		ollamaReq.Tools = convertToolsToOllama(req.Tools)
	}

	// Stream = true (default)
	var finalResp ollamaapi.ChatResponse
	err := p.client.Chat(ctx, ollamaReq, func(resp ollamaapi.ChatResponse) error {
		finalResp = resp
		if resp.Message.Content != "" {
			handler(StreamEvent{
				Type:      "text_delta",
				TextDelta: resp.Message.Content,
			})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("ollama stream: %w", err)
	}

	handler(StreamEvent{Type: "done"})

	result := &ChatResponse{
		StopReason: "end_turn",
	}
	result.Message.Role = "assistant"
	result.Message.Content = finalResp.Message.Content

	if len(finalResp.Message.ToolCalls) > 0 {
		result.StopReason = "tool_use"
		for _, tc := range finalResp.Message.ToolCalls {
			argsJSON, _ := json.Marshal(tc.Function.Arguments)
			result.Message.ToolCalls = append(result.Message.ToolCalls, ToolCall{
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: json.RawMessage(argsJSON),
			})
		}
	}

	return result, nil
}

func convertMessagesToOllama(msgs []Message) []ollamaapi.Message {
	var result []ollamaapi.Message
	for _, m := range msgs {
		switch m.Role {
		case "user":
			result = append(result, ollamaapi.Message{
				Role:    "user",
				Content: m.Content,
			})
		case "assistant":
			om := ollamaapi.Message{
				Role:    "assistant",
				Content: m.Content,
			}
			for _, tc := range m.ToolCalls {
				args := ollamaapi.NewToolCallFunctionArguments()
				var parsed map[string]any
				_ = json.Unmarshal(tc.Input, &parsed)
				for k, v := range parsed {
					args.Set(k, v)
				}
				om.ToolCalls = append(om.ToolCalls, ollamaapi.ToolCall{
					ID: tc.ID,
					Function: ollamaapi.ToolCallFunction{
						Name:      tc.Name,
						Arguments: args,
					},
				})
			}
			result = append(result, om)
		case "tool":
			if m.ToolResult != nil {
				result = append(result, ollamaapi.Message{
					Role:       "tool",
					Content:    m.ToolResult.Content,
					ToolCallID: m.ToolResult.ToolCallID,
				})
			}
		}
	}
	return result
}

func convertToolsToOllama(tools []ToolDef) ollamaapi.Tools {
	var result ollamaapi.Tools
	for _, t := range tools {
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
			Required   []string                   `json:"required"`
		}
		_ = json.Unmarshal(t.InputSchema, &schema)

		props := ollamaapi.NewToolPropertiesMap()
		for name, raw := range schema.Properties {
			var prop ollamaapi.ToolProperty
			_ = json.Unmarshal(raw, &prop)
			props.Set(name, prop)
		}

		result = append(result, ollamaapi.Tool{
			Type: "function",
			Function: ollamaapi.ToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters: ollamaapi.ToolFunctionParameters{
					Type:       "object",
					Properties: props,
					Required:   schema.Required,
				},
			},
		})
	}
	return result
}
