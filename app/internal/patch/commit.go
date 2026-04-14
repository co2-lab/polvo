package patch

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/co2-lab/polvo/internal/provider"
)

const commitPrompt = `You are an expert software engineer that generates concise, one-line Git commit messages based on the provided diffs.

Review the provided diff and generate a single commit message.
The commit message must be structured as: <type>(<scope>): <description>
Types: feat, fix, refactor, docs, test, chore, build, ci, style, perf

Rules:
- Use imperative mood (e.g. "add feature" not "added feature")
- Do not exceed 72 characters
- Scope is the agent name or file/package affected (optional but recommended)

Reply ONLY with the one-line commit message. No explanation, no quotes, no extra text.

Diff:
`

// CommitMessageGenerator generates conventional commit messages from diffs.
type CommitMessageGenerator struct {
	Provider    provider.ChatProvider
	Model       string
	AgentName   string
	WatcherName string    // name of the watcher that triggered execution (optional)
	Timestamp   time.Time // when the commit was generated; zero value uses time.Now()
}

// Generate produces a commit message for the given diff using the summary model.
// Falls back to a default message if the LLM call fails.
//
// The returned string is a full git commit message: a subject line followed by
// metadata trailers (Triggered-by and Generated-at) in the body.
func (g *CommitMessageGenerator) Generate(ctx context.Context, diff string) string {
	subject := g.generateSubject(ctx, diff)
	return g.attachMetadata(subject)
}

// generateSubject returns only the first-line commit subject, either from the
// LLM or from the fallback.
func (g *CommitMessageGenerator) generateSubject(ctx context.Context, diff string) string {
	if g.Provider == nil || diff == "" {
		return g.fallback()
	}

	req := provider.ChatRequest{
		Model: g.Model,
		Messages: []provider.Message{
			{Role: "user", Content: commitPrompt + diff},
		},
		MaxTokens: 128,
	}

	resp, err := g.Provider.Chat(ctx, req)
	if err != nil {
		return g.fallback()
	}

	msg := strings.TrimSpace(resp.Message.Content)
	msg = strings.Trim(msg, `"'`) // strip surrounding quotes
	if msg == "" || len(msg) > 72 {
		return g.fallback()
	}
	return msg
}

// attachMetadata appends Triggered-by and Generated-at trailers to the subject.
func (g *CommitMessageGenerator) attachMetadata(subject string) string {
	ts := g.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	var sb strings.Builder
	sb.WriteString(subject)

	if g.WatcherName != "" {
		sb.WriteString("\n\nTriggered-by: ")
		sb.WriteString(g.WatcherName)
	}
	sb.WriteString("\nGenerated-at: ")
	sb.WriteString(ts.UTC().Format(time.RFC3339))

	return sb.String()
}

func (g *CommitMessageGenerator) fallback() string {
	if g.AgentName != "" {
		return fmt.Sprintf("chore(%s): apply agent changes", g.AgentName)
	}
	return "chore: apply polvo agent changes"
}
