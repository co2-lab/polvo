package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/co2-lab/polvo/internal/provider"
	"github.com/co2-lab/polvo/internal/template"
)

// LLMAgent is an agent backed by an LLM provider.
type LLMAgent struct {
	name         string
	role         Role
	guide        string // resolved guide content
	promptTmpl   string // prompt template content
	providerInst provider.LLMProvider
	model        string
}

// NewLLMAgent creates a new LLM-backed agent.
func NewLLMAgent(name string, role Role, guide, promptTmpl string, p provider.LLMProvider, model string) *LLMAgent {
	return &LLMAgent{
		name:         name,
		role:         role,
		guide:        guide,
		promptTmpl:   promptTmpl,
		providerInst: p,
		model:        model,
	}
}

func (a *LLMAgent) Name() string { return a.name }
func (a *LLMAgent) Role() Role   { return a.role }

func (a *LLMAgent) Execute(ctx context.Context, input *Input) (*Result, error) {
	// Build template data
	data := &template.Data{
		File:            input.File,
		Content:         input.Content,
		Diff:            input.Diff,
		Guide:           a.guide,
		Event:           input.Event,
		ProjectRoot:     input.ProjectRoot,
		PreviousReports: input.PreviousReports,
		FileHistory:     input.FileHistory,
		Interface:       input.Interface,
		Spec:            input.Spec,
		Feature:         input.Feature,
		PRDiff:          input.PRDiff,
		PRComments:      input.PRComments,
	}

	// Render prompt
	prompt, err := template.Render(a.promptTmpl, data)
	if err != nil {
		return nil, fmt.Errorf("rendering prompt for agent %s: %w", a.name, err)
	}

	// Call provider
	resp, err := a.providerInst.Complete(ctx, provider.Request{
		Model:     a.model,
		System:    "You are the " + a.name + " agent for Polvo, an AI agent orchestrator.",
		Prompt:    prompt,
		MaxTokens: 4096,
	})
	if err != nil {
		return nil, fmt.Errorf("agent %s completion: %w", a.name, err)
	}

	return parseResult(resp.Content, a.role), nil
}

func parseResult(content string, role Role) *Result {
	result := &Result{
		Content: content,
		Summary: content,
	}

	if role == RoleReviewer {
		upper := strings.ToUpper(content)
		if strings.Contains(upper, "\"APPROVE\"") || strings.Contains(upper, "APPROVE") {
			result.Decision = "APPROVE"
		} else {
			result.Decision = "REJECT"
		}
	}

	return result
}
