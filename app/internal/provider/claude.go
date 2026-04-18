package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// ClaudeProvider implements LLMProvider for Anthropic Claude.
type ClaudeProvider struct {
	name         string
	apiKey       string
	defaultModel string
	client       anthropic.Client
}

// NewClaude creates a new Claude provider.
func NewClaude(name, apiKey, defaultModel string) *ClaudeProvider {
	if defaultModel == "" {
		defaultModel = "claude-sonnet-4-6"
	}

	var opts []option.RequestOption
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}

	client := anthropic.NewClient(opts...)

	return &ClaudeProvider{
		name:         name,
		apiKey:       apiKey,
		defaultModel: defaultModel,
		client:       client,
	}
}

func (p *ClaudeProvider) Name() string   { return p.name }
func (p *ClaudeProvider) APIKey() string { return p.apiKey }

func (p *ClaudeProvider) Complete(ctx context.Context, req Request) (*Response, error) {
	model := req.Model
	if model == "" {
		model = p.defaultModel
	}

	maxTokens := int64(req.MaxTokens)
	if maxTokens == 0 {
		maxTokens = 4096
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: maxTokens,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(req.Prompt)),
		},
	}

	if req.System != "" {
		systemBlocks := []anthropic.TextBlockParam{
			{Text: req.System, Type: "text"},
		}
		if IsCacheableModel(model) {
			systemBlocks = applySystemCacheControl(systemBlocks)
		}
		params.System = systemBlocks
	}

	var callOpts []option.RequestOption
	if IsCacheableModel(model) {
		callOpts = append(callOpts, withCacheControlOpts(model)...)
	}

	msg, err := p.client.Messages.New(ctx, params, callOpts...)
	if err != nil {
		return nil, fmt.Errorf("claude completion: %w", err)
	}

	var content string
	for _, block := range msg.Content {
		if block.Type == "text" {
			content += block.Text
		}
	}

	return &Response{
		Content: content,
		TokensUsed: TokenUsage{
			PromptTokens:     int(msg.Usage.InputTokens),
			CompletionTokens: int(msg.Usage.OutputTokens),
			TotalTokens:      int(msg.Usage.InputTokens + msg.Usage.OutputTokens),
			CacheReadTokens:  int(msg.Usage.CacheReadInputTokens),
			CacheWriteTokens: int(msg.Usage.CacheCreationInputTokens),
		},
	}, nil
}

func (p *ClaudeProvider) Available(ctx context.Context) error {
	if p.apiKey == "" {
		return fmt.Errorf("claude API key not configured")
	}
	return nil
}

func (p *ClaudeProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = p.defaultModel
	}

	maxTokens := int64(req.MaxTokens)
	if maxTokens == 0 {
		maxTokens = 16384
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: maxTokens,
		Messages:  convertMessagesToClaude(req.Messages),
	}

	if req.System != "" {
		systemBlocks := []anthropic.TextBlockParam{
			{Text: req.System, Type: "text"},
		}
		if IsCacheableModel(model) {
			systemBlocks = applySystemCacheControl(systemBlocks)
		}
		params.System = systemBlocks
	}

	if len(req.Tools) > 0 {
		params.Tools = convertToolsToClaude(req.Tools)
	}

	var callOpts []option.RequestOption
	if IsCacheableModel(model) {
		callOpts = append(callOpts, withCacheControlOpts(model)...)
	}

	msg, err := p.client.Messages.New(ctx, params, callOpts...)
	if err != nil {
		return nil, fmt.Errorf("claude chat: %w", err)
	}

	resp := &ChatResponse{
		StopReason: string(msg.StopReason),
		TokensUsed: TokenUsage{
			PromptTokens:     int(msg.Usage.InputTokens),
			CompletionTokens: int(msg.Usage.OutputTokens),
			TotalTokens:      int(msg.Usage.InputTokens + msg.Usage.OutputTokens),
			CacheReadTokens:  int(msg.Usage.CacheReadInputTokens),
			CacheWriteTokens: int(msg.Usage.CacheCreationInputTokens),
		},
	}

	resp.Message.Role = "assistant"
	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			resp.Message.Content += block.Text
		case "tool_use":
			tu := block.AsToolUse()
			resp.Message.ToolCalls = append(resp.Message.ToolCalls, ToolCall{
				ID:    tu.ID,
				Name:  tu.Name,
				Input: json.RawMessage(tu.Input),
			})
		}
	}

	return resp, nil
}

func (p *ClaudeProvider) ChatStream(ctx context.Context, req ChatRequest, handler func(StreamEvent)) (*ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = p.defaultModel
	}

	maxTokens := int64(req.MaxTokens)
	if maxTokens == 0 {
		maxTokens = 16384
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: maxTokens,
		Messages:  convertMessagesToClaude(req.Messages),
	}

	if req.System != "" {
		systemBlocks := []anthropic.TextBlockParam{
			{Text: req.System, Type: "text"},
		}
		if IsCacheableModel(model) {
			systemBlocks = applySystemCacheControl(systemBlocks)
		}
		params.System = systemBlocks
	}

	if len(req.Tools) > 0 {
		params.Tools = convertToolsToClaude(req.Tools)
	}

	var streamOpts []option.RequestOption
	if IsCacheableModel(model) {
		streamOpts = append(streamOpts, withCacheControlOpts(model)...)
	}

	stream := p.client.Messages.NewStreaming(ctx, params, streamOpts...)
	defer stream.Close()

	var msg anthropic.Message
	for stream.Next() {
		event := stream.Current()
		if err := msg.Accumulate(event); err != nil {
			continue
		}

		switch event.Type {
		case "content_block_delta":
			if event.Delta.Type == "text_delta" {
				handler(StreamEvent{
					Type:      "text_delta",
					TextDelta: event.Delta.Text,
				})
			}
		case "content_block_start":
			if event.ContentBlock.Type == "tool_use" {
				tu := event.ContentBlock.AsToolUse()
				handler(StreamEvent{
					Type: "tool_use_start",
					ToolCall: &ToolCall{
						ID:   tu.ID,
						Name: tu.Name,
					},
				})
			}
		}
	}

	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("claude stream: %w", err)
	}

	handler(StreamEvent{Type: "done"})

	resp := &ChatResponse{
		StopReason: string(msg.StopReason),
		TokensUsed: TokenUsage{
			PromptTokens:     int(msg.Usage.InputTokens),
			CompletionTokens: int(msg.Usage.OutputTokens),
			TotalTokens:      int(msg.Usage.InputTokens + msg.Usage.OutputTokens),
			CacheReadTokens:  int(msg.Usage.CacheReadInputTokens),
			CacheWriteTokens: int(msg.Usage.CacheCreationInputTokens),
		},
	}

	resp.Message.Role = "assistant"
	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			resp.Message.Content += block.Text
		case "tool_use":
			tu := block.AsToolUse()
			resp.Message.ToolCalls = append(resp.Message.ToolCalls, ToolCall{
				ID:    tu.ID,
				Name:  tu.Name,
				Input: json.RawMessage(tu.Input),
			})
		}
	}

	return resp, nil
}

func convertMessagesToClaude(msgs []Message) []anthropic.MessageParam {
	var params []anthropic.MessageParam
	for _, m := range msgs {
		switch m.Role {
		case "user":
			params = append(params, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))
		case "assistant":
			var blocks []anthropic.ContentBlockParamUnion
			if m.Content != "" {
				blocks = append(blocks, anthropic.NewTextBlock(m.Content))
			}
			for _, tc := range m.ToolCalls {
				var input any
				_ = json.Unmarshal(tc.Input, &input)
				blocks = append(blocks, anthropic.NewToolUseBlock(tc.ID, input, tc.Name))
			}
			params = append(params, anthropic.NewAssistantMessage(blocks...))
		case "tool":
			if m.ToolResult != nil {
				params = append(params, anthropic.NewUserMessage(
					anthropic.NewToolResultBlock(m.ToolResult.ToolCallID, m.ToolResult.Content, m.ToolResult.IsError),
				))
			}
		}
	}
	return params
}

func convertToolsToClaude(tools []ToolDef) []anthropic.ToolUnionParam {
	var params []anthropic.ToolUnionParam
	for _, t := range tools {
		var props any
		var required []string
		var schema struct {
			Properties any      `json:"properties"`
			Required   []string `json:"required"`
		}
		if err := json.Unmarshal(t.InputSchema, &schema); err == nil {
			props = schema.Properties
			required = schema.Required
		}

		params = append(params, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        t.Name,
				Description: anthropic.Opt(t.Description),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: props,
					Required:   required,
				},
			},
		})
	}
	return params
}
