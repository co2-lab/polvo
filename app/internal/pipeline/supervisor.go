package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/co2-lab/polvo/internal/provider"
)

// RoutingDecision is the LLM supervisor's routing response.
type RoutingDecision struct {
	Agent       string  `json:"agent"`       // specialist agent name
	Confidence  float64 `json:"confidence"`  // 0.0–1.0
	Rationale   string  `json:"rationale"`   // reasoning for audit log
	Fallthrough bool    `json:"fallthrough"` // true = use DefaultChain
}

// Supervisor uses an LLM to dynamically route FileEvents to specialist agents.
// When nil or when confidence is below threshold, the Scheduler uses DefaultChain.
type Supervisor struct {
	provider            provider.ChatProvider
	model               string
	specialists         map[string]Step
	confidenceThreshold float64
}

// NewSupervisor creates a Supervisor with the specialist registry built from steps.
// If confidenceThreshold is 0, defaults to 0.7.
func NewSupervisor(p provider.ChatProvider, model string, steps []Step, confidenceThreshold float64) *Supervisor {
	if confidenceThreshold <= 0 {
		confidenceThreshold = 0.7
	}
	specialists := make(map[string]Step, len(steps))
	for _, step := range steps {
		specialists[step.Agent] = step
	}
	return &Supervisor{
		provider:            p,
		model:               model,
		specialists:         specialists,
		confidenceThreshold: confidenceThreshold,
	}
}

var jsonExtractRe = regexp.MustCompile(`\{[^}]+\}`)

// extractJSON extracts the first JSON object from a string.
func extractJSON(s string) string {
	if m := jsonExtractRe.FindString(s); m != "" {
		return m
	}
	return s
}

// Route returns the routing decision for the given event.
// On error or when confidence is below threshold, returns Fallthrough=true.
func (s *Supervisor) Route(ctx context.Context, event *FileEvent) (RoutingDecision, error) {
	var agentDescs []string
	for name, step := range s.specialists {
		agentDescs = append(agentDescs, fmt.Sprintf("- %s: handles %s events", name, step.Trigger))
	}

	systemPrompt := `You are a routing agent. Given a file event, decide which specialist agent should handle it. Respond with ONLY valid JSON matching exactly:
{"agent":"<name>","confidence":<0-1>,"rationale":"<why>","fallthrough":<bool>}
Use fallthrough:true if no agent is a good fit or you are unsure.`

	userMsg := fmt.Sprintf("Available agents:\n%s\n\nEvent: type=%s file=%s",
		strings.Join(agentDescs, "\n"), event.Type, event.File)

	resp, err := s.provider.Chat(ctx, provider.ChatRequest{
		Model:  s.model,
		System: systemPrompt,
		Messages: []provider.Message{
			{Role: "user", Content: userMsg},
		},
		MaxTokens: 256,
	})
	if err != nil {
		slog.Warn("supervisor routing error", "err", err)
		return RoutingDecision{Fallthrough: true}, err
	}

	var decision RoutingDecision
	if err := json.Unmarshal([]byte(extractJSON(resp.Message.Content)), &decision); err != nil {
		slog.Warn("supervisor: invalid JSON response, falling through", "content", resp.Message.Content)
		return RoutingDecision{Fallthrough: true}, nil
	}
	if decision.Confidence < s.confidenceThreshold {
		decision.Fallthrough = true
	}
	return decision, nil
}
