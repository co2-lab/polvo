package agent

import (
	"context"
	"fmt"

	"github.com/co2-lab/polvo/internal/provider"
	"github.com/co2-lab/polvo/internal/session"
)

// LLMSummarizer implements session.Summarizer using a ChatProvider.
// Used when settings.summary_model is configured.
type LLMSummarizer struct {
	Provider provider.ChatProvider
	Model    string
}

var _ session.Summarizer = LLMSummarizer{}

// Summarize generates a short summary of the work item prompt using the LLM.
func (s LLMSummarizer) Summarize(ctx context.Context, kind session.Kind, prompt string) (string, error) {
	kindLabel := string(kind)
	userMsg := fmt.Sprintf("Summarize this %s in 1-2 sentences (be concise, start with a verb):\n\n%s", kindLabel, prompt)

	resp, err := s.Provider.Chat(ctx, provider.ChatRequest{
		Model: s.Model,
		Messages: []provider.Message{
			{Role: "user", Content: userMsg},
		},
		MaxTokens: 128,
	})
	if err != nil {
		return "", fmt.Errorf("llm summarizer: %w", err)
	}
	return resp.Message.Content, nil
}

// SummarizeTurn generates a short summary of a completed conversation turn.
// userText is the user's message; assistantText is the assistant's response.
func SummarizeTurn(ctx context.Context, p provider.ChatProvider, model, userText, assistantText string) (string, error) {
	prompt := fmt.Sprintf(
		"Summarize this conversation turn in 1-2 sentences (be concise, start with a verb):\n\nUser: %s\n\nAssistant: %s",
		userText, assistantText,
	)
	resp, err := p.Chat(ctx, provider.ChatRequest{
		Model:     model,
		Messages:  []provider.Message{{Role: "user", Content: prompt}},
		MaxTokens: 128,
	})
	if err != nil {
		return "", fmt.Errorf("summarize turn: %w", err)
	}
	return resp.Message.Content, nil
}
