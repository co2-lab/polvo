package agent

import (
	"context"
	"fmt"

	"github.com/co2-lab/polvo/internal/provider"
	"github.com/co2-lab/polvo/internal/template"
	"github.com/co2-lab/polvo/internal/tool"
)

// ToolLLMAgent is an agent that uses the agentic loop with read-only tools
// to explore the codebase before generating content.
type ToolLLMAgent struct {
	name            string
	role            Role
	guide           string
	promptTmpl      string
	providerInst    provider.ChatProvider
	model           string
	tools           *tool.Registry
	architectEditor ArchitectEditorConfig
}

// NewToolLLMAgent creates an agent that uses tools during execution.
func NewToolLLMAgent(name string, role Role, guide, promptTmpl string, p provider.ChatProvider, model string, tools *tool.Registry) *ToolLLMAgent {
	return &ToolLLMAgent{
		name:         name,
		role:         role,
		guide:        guide,
		promptTmpl:   promptTmpl,
		providerInst: p,
		model:        model,
		tools:        tools,
	}
}

// WithArchitectEditor configures the two-phase architect/editor loop for this agent.
func (a *ToolLLMAgent) WithArchitectEditor(cfg ArchitectEditorConfig) *ToolLLMAgent {
	a.architectEditor = cfg
	return a
}

func (a *ToolLLMAgent) Name() string { return a.name }
func (a *ToolLLMAgent) Role() Role   { return a.role }

func (a *ToolLLMAgent) Execute(ctx context.Context, input *Input) (*Result, error) {
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

	prompt, err := template.Render(a.promptTmpl, data)
	if err != nil {
		return nil, fmt.Errorf("rendering prompt for agent %s: %w", a.name, err)
	}

	// When architect/editor is enabled use the full tool registry (the two-phase
	// loop itself restricts the architect to read-only tools internally).
	// Otherwise fall back to the classic read-only exploration registry.
	var loopTools *tool.Registry
	if a.architectEditor.Enabled {
		loopTools = a.tools
	} else {
		readOnlyReg := tool.NewRegistry()
		for _, t := range a.tools.All() {
			switch t.Name() {
			case "read", "glob", "grep", "ls":
				readOnlyReg.Register(t)
			}
		}
		loopTools = readOnlyReg
	}

	loop := NewLoop(LoopConfig{
		Provider:        a.providerInst,
		Tools:           loopTools,
		System:          "You are the " + a.name + " agent for Polvo, an AI agent orchestrator.",
		Model:           a.model,
		MaxTurns:        20,
		MaxTokens:       8192,
		ArchitectEditor: a.architectEditor,
	})

	result, err := loop.Run(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("agent %s loop: %w", a.name, err)
	}

	return parseResult(result.FinalText, a.role), nil
}
