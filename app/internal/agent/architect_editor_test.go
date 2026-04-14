package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/co2-lab/polvo/internal/provider"
	"github.com/co2-lab/polvo/internal/tool"
)

// validWorkPlanJSON is a minimal valid work plan JSON for use in tests.
const validWorkPlanJSON = `{
  "summary": "add a comment",
  "files_to_edit": ["foo.go"],
  "steps": [
    {
      "file": "foo.go",
      "description": "add a comment at the top"
    }
  ]
}`

// architectResponse wraps the work plan JSON in the expected XML tags.
func architectResponse(planJSON string) string {
	return "I have analyzed the code.\n\n<work_plan>\n" + planJSON + "\n</work_plan>\n"
}

// TestTwoPhase_ArchitectProducesWorkPlan verifies that when the architect
// returns a valid work plan the editor phase runs and results are merged.
func TestTwoPhase_ArchitectProducesWorkPlan(t *testing.T) {
	// Architect: 1 turn ending with a work plan.
	// Editor:    1 turn to "implement" it.
	archProvider := &scriptedChatProvider{
		turns: []provider.ChatResponse{
			scriptedEndTurn(architectResponse(validWorkPlanJSON)),
		},
	}
	editorProvider := &scriptedChatProvider{
		turns: []provider.ChatResponse{
			scriptedEndTurn("Done. Added a comment to foo.go."),
		},
	}

	// We need both phases to use different providers; however the LoopConfig
	// uses a single Provider field.  To support independent scripting we wire a
	// switchingProvider that dispatches to arch/editor based on a flag.
	sp := &switchingProvider{current: archProvider}

	reg := tool.NewRegistry()
	reg.Register(makeNoopTool("read"))
	reg.Register(makeNoopTool("glob"))
	reg.Register(makeNoopTool("grep"))
	reg.Register(makeNoopTool("write"))
	reg.Register(makeNoopTool("edit"))

	loop := NewLoop(LoopConfig{
		Provider: sp,
		Tools:    reg,
		System:   "you are a test agent",
		Model:    "test-model",
		MaxTurns: 20,
		MaxTokens: 4096,
		ArchitectEditor: ArchitectEditorConfig{
			Enabled:           true,
			MaxArchitectTurns: 3,
			MaxEditorTurns:    10,
		},
	})

	// Switch to editor provider after the architect phase completes.
	// We intercept this via the overrideAfter mechanism on switchingProvider.
	sp.switchAfterCalls = 1
	sp.next = editorProvider

	result, err := loop.Run(context.Background(), "add a comment to foo.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Editor's final text should be the loop result.
	if !strings.Contains(result.FinalText, "Done") {
		t.Errorf("expected FinalText to contain 'Done', got: %q", result.FinalText)
	}

	// Turn count should include both phases (1 arch + 1 editor = 2).
	if result.TurnCount != 2 {
		t.Errorf("expected TurnCount=2 (arch+editor), got %d", result.TurnCount)
	}
}

// TestTwoPhase_FallsBackIfNoPlan verifies that when the architect returns
// a response without a work plan the single-phase loop is executed instead.
func TestTwoPhase_FallsBackIfNoPlan(t *testing.T) {
	// Architect returns a response WITHOUT a <work_plan> block.
	// The fallback single-phase will then call the provider again,
	// so we pre-load two end-turn responses.
	script := &scriptedChatProvider{
		turns: []provider.ChatResponse{
			// architect turn (no plan)
			scriptedEndTurn("I could not produce a plan."),
			// single-phase fallback turn
			scriptedEndTurn("fallback completed"),
		},
	}

	reg := tool.NewRegistry()
	reg.Register(makeNoopTool("read"))

	loop := NewLoop(LoopConfig{
		Provider:  script,
		Tools:     reg,
		System:    "test",
		Model:     "test-model",
		MaxTurns:  20,
		MaxTokens: 4096,
		ArchitectEditor: ArchitectEditorConfig{
			Enabled:           true,
			MaxArchitectTurns: 1,
			MaxEditorTurns:    5,
		},
	})

	result, err := loop.Run(context.Background(), "do something")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Single-phase fallback should have run; result is from the second call.
	if !strings.Contains(result.FinalText, "fallback completed") {
		t.Errorf("expected fallback result, got: %q", result.FinalText)
	}
}

// TestTwoPhase_ArchitectReadOnlyTools verifies that the architect sub-loop
// does NOT have write/edit/bash tools and DOES have read/glob/grep.
func TestTwoPhase_ArchitectReadOnlyTools(t *testing.T) {
	var architectToolNames []string

	// The architect calls a tool so we can capture what tools were offered.
	// We capture this via the ChatRequest recorded by scriptedChatProvider.
	archProvider := &scriptedChatProvider{
		turns: []provider.ChatResponse{
			// Architect ends without a plan so we only need one turn.
			scriptedEndTurn("no plan here"),
		},
	}

	reg := tool.NewRegistry()
	for _, name := range []string{"read", "glob", "grep", "ls", "think", "write", "edit", "bash"} {
		reg.Register(makeNoopTool(name))
	}

	loop := NewLoop(LoopConfig{
		Provider:  archProvider,
		Tools:     reg,
		System:    "test",
		Model:     "test-model",
		MaxTurns:  20,
		MaxTokens: 4096,
		ArchitectEditor: ArchitectEditorConfig{
			Enabled:           true,
			MaxArchitectTurns: 1,
			MaxEditorTurns:    1,
		},
	})

	// Run: architect will end without a plan; the fallback single-phase
	// will also need a response — reuse archProvider with an extra turn.
	archProvider.turns = append(archProvider.turns,
		scriptedEndTurn("fallback done"),
	)

	_, err := loop.Run(context.Background(), "task")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The first ChatRequest was for the architect phase.
	if len(archProvider.reqs) == 0 {
		t.Fatal("no requests recorded")
	}
	archReq := archProvider.reqs[0]
	for _, td := range archReq.Tools {
		architectToolNames = append(architectToolNames, td.Name)
	}

	// Must contain read-only tools.
	for _, want := range []string{"read", "glob", "grep"} {
		found := false
		for _, n := range architectToolNames {
			if n == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("architect tool list missing %q; got %v", want, architectToolNames)
		}
	}

	// Must NOT contain write, edit, or bash.
	for _, forbidden := range []string{"write", "edit", "bash"} {
		for _, n := range architectToolNames {
			if n == forbidden {
				t.Errorf("architect tool list must not contain %q; got %v", forbidden, architectToolNames)
			}
		}
	}
}

// switchingProvider is a ChatProvider that delegates to `current` until
// `switchAfterCalls` calls have been made, then switches to `next`.
type switchingProvider struct {
	current          *scriptedChatProvider
	next             *scriptedChatProvider
	switchAfterCalls int
	totalCalls       int
}

func (s *switchingProvider) Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	s.totalCalls++
	if s.next != nil && s.totalCalls > s.switchAfterCalls {
		return s.next.Chat(ctx, req)
	}
	return s.current.Chat(ctx, req)
}

func (s *switchingProvider) Name() string                      { return "switching" }
func (s *switchingProvider) Available(_ context.Context) error { return nil }
func (s *switchingProvider) Complete(_ context.Context, _ provider.Request) (*provider.Response, error) {
	return &provider.Response{}, nil
}
