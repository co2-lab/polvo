package skill

import (
	"context"
	"fmt"
	"strings"

	"github.com/co2-lab/polvo/internal/memory"
	"github.com/co2-lab/polvo/internal/provider"
)

// Extractor calls an LLM to extract reusable project-specific procedures from a
// completed agent session, then persists them in the memory store as "decision"
// type entries for injection in future sessions.
type Extractor struct {
	Provider provider.ChatProvider
	Model    string
	Store    *memory.Store
}

// Extract analyses turnHistory + summary and writes 0–3 skills to the store.
// Returns the number of skills written.
func (e *Extractor) Extract(ctx context.Context, turnHistory, summary, workDir string) (int, error) {
	prompt := buildExtractionPrompt(turnHistory, summary, workDir)
	resp, err := e.Provider.Chat(ctx, provider.ChatRequest{
		Model:     e.Model,
		Messages:  []provider.Message{{Role: "user", Content: prompt}},
		MaxTokens: 1024,
	})
	if err != nil {
		return 0, fmt.Errorf("skill extraction: %w", err)
	}
	skills := parseSkills(resp.Message.Content)
	for _, s := range skills {
		if err := e.Store.Write(memory.Entry{
			Agent:   "skill-extractor",
			Type:    "decision",
			Content: s,
		}); err != nil {
			return 0, fmt.Errorf("writing skill: %w", err)
		}
	}
	return len(skills), nil
}

func buildExtractionPrompt(history, summary, workDir string) string {
	return fmt.Sprintf(`You are analyzing a completed coding agent session to extract reusable procedures.

Project directory: %s
Session summary: %s

Session history (last 3000 chars):
%s

Extract 0-3 reusable procedures learned in this session. A procedure is worth extracting only if:
- It is specific to this project (not general programming knowledge)
- It would save time if remembered in a future session
- It describes HOW to do something (a command, a pattern, a workflow)

Format each procedure as a single line starting with "SKILL: ".
If nothing is worth extracting, output only "NONE".

Examples of good skills:
SKILL: To run tests in this project: cd app && go test ./... -count=1
SKILL: The deploy script requires AWS_PROFILE=prod to be set before running make deploy
SKILL: Migration files go in app/internal/db/migrations/ and must use goose format

Examples of bad skills (too generic, don't extract):
- "Use go test to run tests" (generic Go knowledge)
- "Check the README for instructions" (not actionable)`, workDir, summary, last3000(history))
}

func parseSkills(response string) []string {
	var skills []string
	for line := range strings.SplitSeq(response, "\n") {
		line = strings.TrimSpace(line)
		if s, ok := strings.CutPrefix(line, "SKILL: "); ok && s != "" {
			skills = append(skills, s)
		}
	}
	return skills
}

func last3000(s string) string {
	if len(s) <= 3000 {
		return s
	}
	return "...(truncated)...\n" + s[len(s)-3000:]
}
