package git

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/co2-lab/polvo/internal/provider"
)

var conventionalCommitRe = regexp.MustCompile(`^[a-z]+\([^)]+\): .+$`)

// LLMMessageGenerator generates conventional commit messages via an LLM.
// It caches the last result so repeated calls with the same diff are free.
type LLMMessageGenerator struct {
	provider    provider.ChatProvider
	model       string
	agentName   string
	taskDesc    string
	lastDiff    string
	lastMessage string
}

// NewLLMMessageGenerator creates a new LLMMessageGenerator.
func NewLLMMessageGenerator(p provider.ChatProvider, model, agentName, taskDesc string) *LLMMessageGenerator {
	return &LLMMessageGenerator{
		provider:  p,
		model:     model,
		agentName: agentName,
		taskDesc:  taskDesc,
	}
}

// Generate returns a conventional commit message for the given diff.
// If the diff is unchanged since the last call, the cached message is returned.
// If the LLM fails or returns an invalid message, a fallback is used.
func (g *LLMMessageGenerator) Generate(ctx context.Context, diff string) (string, error) {
	// Truncate diff to avoid bloating the prompt.
	if len(diff) > 4000 {
		diff = diff[:4000]
	}

	// Cache hit — skip LLM call.
	if diff == g.lastDiff && g.lastMessage != "" {
		return g.lastMessage, nil
	}

	resp, err := g.provider.Chat(ctx, provider.ChatRequest{
		Model: g.model,
		Messages: []provider.Message{
			{
				Role:    "system",
				Content: "Output only a single conventional commit message line in the format: `type(scope): description`. No explanation, no markdown, no newlines.",
			},
			{
				Role:    "user",
				Content: fmt.Sprintf("Agent: %s\nTask: %s\nDiff:\n%s", g.agentName, g.taskDesc, diff),
			},
		},
		MaxTokens: 80,
	})

	var msg string
	if err == nil {
		msg = resp.Message.Content
	}

	if !conventionalCommitRe.MatchString(msg) {
		msg = g.fallback()
	}

	g.lastDiff = diff
	g.lastMessage = msg
	return msg, nil
}

func (g *LLMMessageGenerator) fallback() string {
	return fmt.Sprintf("chore(%s): automated changes at %s",
		sanitizeBranchComponent(g.agentName),
		time.Now().Format("2006-01-02T15:04:05"),
	)
}
