package agent

import (
	"fmt"
	"os"
	"strings"

	"github.com/co2-lab/polvo/internal/assets"
	"github.com/co2-lab/polvo/internal/config"
	"github.com/co2-lab/polvo/internal/guide"
	"github.com/co2-lab/polvo/internal/provider"
	"github.com/co2-lab/polvo/internal/tool"
)

// Executor builds and manages agents from config.
type Executor struct {
	resolver *guide.Resolver
	registry *provider.Registry
	cfg      *config.Config
	agents   map[string]Agent
}

// NewExecutor creates a new agent executor.
func NewExecutor(resolver *guide.Resolver, registry *provider.Registry, cfg *config.Config) *Executor {
	return &Executor{
		resolver: resolver,
		registry: registry,
		cfg:      cfg,
		agents:   make(map[string]Agent),
	}
}

// GetAgent returns (or creates) an agent for the given guide name and interface group.
func (e *Executor) GetAgent(guideName string, group *config.InterfaceGroupConfig) (Agent, error) {
	key := guideName
	if group != nil {
		// We use the group pattern matching as a key prefix to cache per group
		key = strings.Join(group.Patterns, ",") + ":" + guideName
	}

	if a, ok := e.agents[key]; ok {
		return a, nil
	}

	a, err := e.buildAgent(guideName, group)
	if err != nil {
		return nil, err
	}
	e.agents[key] = a
	return a, nil
}

func (e *Executor) buildAgent(guideName string, group *config.InterfaceGroupConfig) (Agent, error) {
	// Resolve the correct guide configuration (Group overrides Global)
	var gcfg config.GuideConfig
	if group != nil {
		gcfg = group.GetGuideConfig(guideName, e.cfg.Guides)
	} else {
		gcfg = e.cfg.Guides[guideName]
	}

	// Resolve guide content
	g, err := e.resolver.Resolve(guideName, gcfg)
	if err != nil {
		return nil, fmt.Errorf("resolving guide %q: %w", guideName, err)
	}

	// Load prompt template
	var promptContent string
	if gcfg.Prompt != "" {
		promptContent = gcfg.Prompt
	} else {
		data, err := assets.ReadPrompt(guideName)
		if err != nil {
			return nil, fmt.Errorf("reading prompt for %q: %w", guideName, err)
		}
		promptContent = string(data)
	}

	// Resolve provider
	providerName := gcfg.Provider
	var p provider.LLMProvider
	if providerName != "" {
		p, err = e.registry.Get(providerName)
	} else {
		p, err = e.registry.Default()
	}
	if err != nil {
		return nil, fmt.Errorf("resolving provider for %q: %w", guideName, err)
	}

	role := RoleAuthor
	if g.Role == "reviewer" {
		role = RoleReviewer
	}

	// Use ToolLLMAgent if tools are enabled and provider supports Chat
	if gcfg.UseTools {
		if cp, ok := p.(provider.ChatProvider); ok {
			cwd, _ := os.Getwd()
			toolReg := tool.DefaultRegistry(cwd)
			return NewToolLLMAgent(guideName, role, g.Content, promptContent, cp, gcfg.Model, toolReg), nil
		}
	}

	return NewLLMAgent(guideName, role, g.Content, promptContent, p, gcfg.Model), nil
}
