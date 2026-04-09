package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/co2-lab/polvo/internal/provider"
	"github.com/co2-lab/polvo/internal/tool"
)

const defaultMaxTurns = 50

// LoopConfig configures the agentic loop.
type LoopConfig struct {
	Provider     provider.ChatProvider
	Tools        *tool.Registry
	GuardedTools *tool.GuardedRegistry // optional: permission-checked execution
	System       string
	Model        string
	MaxTurns     int
	MaxTokens    int
	OnText       func(text string)
	OnTextDelta  func(delta string)
	OnToolCall   func(call provider.ToolCall)
	OnToolResult func(id, name, result string, isError bool)
}

// LoopResult is the outcome of a loop execution.
type LoopResult struct {
	FinalText  string
	TurnCount  int
	TokensUsed provider.TokenUsage
}

// Loop implements the agentic prompt→LLM→tools→LLM cycle.
type Loop struct {
	cfg  LoopConfig
	conv *Conversation
}

// NewLoop creates a new agentic loop.
func NewLoop(cfg LoopConfig) *Loop {
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = defaultMaxTurns
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 16384
	}
	return &Loop{
		cfg:  cfg,
		conv: NewConversation(),
	}
}

// Run executes the loop with a user prompt. It blocks until the LLM
// finishes (end_turn) or the turn limit is reached.
func (l *Loop) Run(ctx context.Context, userPrompt string) (*LoopResult, error) {
	l.conv.AddUser(userPrompt)

	var totalTokens provider.TokenUsage
	turnCount := 0

	toolDefs := l.buildToolDefs()

	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("loop cancelled: %w", err)
		}

		turnCount++
		if turnCount > l.cfg.MaxTurns {
			return nil, fmt.Errorf("max turns (%d) exceeded", l.cfg.MaxTurns)
		}

		chatReq := provider.ChatRequest{
			Model:     l.cfg.Model,
			System:    l.cfg.System,
			Messages:  l.conv.Messages(),
			Tools:     toolDefs,
			MaxTokens: l.cfg.MaxTokens,
		}

		var resp *provider.ChatResponse
		var err error

		// Use streaming when available and delta callback is set
		if sp, ok := l.cfg.Provider.(provider.StreamProvider); ok && l.cfg.OnTextDelta != nil {
			resp, err = sp.ChatStream(ctx, chatReq, func(event provider.StreamEvent) {
				if event.Type == "text_delta" && l.cfg.OnTextDelta != nil {
					l.cfg.OnTextDelta(event.TextDelta)
				}
			})
		} else {
			resp, err = l.cfg.Provider.Chat(ctx, chatReq)
		}
		if err != nil {
			return nil, fmt.Errorf("chat turn %d: %w", turnCount, err)
		}

		totalTokens.PromptTokens += resp.TokensUsed.PromptTokens
		totalTokens.CompletionTokens += resp.TokensUsed.CompletionTokens
		totalTokens.TotalTokens += resp.TokensUsed.TotalTokens

		// Add assistant message to history
		l.conv.AddAssistant(resp.Message)

		// Fire text callback for any text content
		if resp.Message.Content != "" && l.cfg.OnText != nil {
			l.cfg.OnText(resp.Message.Content)
		}

		if resp.StopReason != "tool_use" || len(resp.Message.ToolCalls) == 0 {
			return &LoopResult{
				FinalText:  resp.Message.Content,
				TurnCount:  turnCount,
				TokensUsed: totalTokens,
			}, nil
		}

		// Execute tools
		for _, tc := range resp.Message.ToolCalls {
			if l.cfg.OnToolCall != nil {
				l.cfg.OnToolCall(tc)
			}

			result := l.executeTool(ctx, tc)
			l.conv.AddToolResult(tc.ID, result.Content, result.IsError)

			if l.cfg.OnToolResult != nil {
				l.cfg.OnToolResult(tc.ID, tc.Name, result.Content, result.IsError)
			}
		}
	}
}

func (l *Loop) buildToolDefs() []provider.ToolDef {
	if l.cfg.Tools == nil {
		return nil
	}
	tools := l.cfg.Tools.All()
	defs := make([]provider.ToolDef, len(tools))
	for i, t := range tools {
		defs[i] = provider.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: t.InputSchema(),
		}
	}
	return defs
}

func (l *Loop) executeTool(ctx context.Context, tc provider.ToolCall) *tool.Result {
	if l.cfg.Tools == nil {
		return tool.ErrorResult("no tools available")
	}

	input := tc.Input
	if input == nil {
		input = json.RawMessage("{}")
	}

	// Use guarded registry if available (permission checks)
	if l.cfg.GuardedTools != nil {
		result, err := l.cfg.GuardedTools.Execute(ctx, tc.Name, input)
		if err != nil {
			return tool.ErrorResult(fmt.Sprintf("tool error: %v", err))
		}
		return result
	}

	t, ok := l.cfg.Tools.Get(tc.Name)
	if !ok {
		return tool.ErrorResult(fmt.Sprintf("unknown tool: %s", tc.Name))
	}

	result, err := t.Execute(ctx, input)
	if err != nil {
		return tool.ErrorResult(fmt.Sprintf("tool error: %v", err))
	}
	return result
}
