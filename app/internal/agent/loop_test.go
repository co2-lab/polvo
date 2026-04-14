package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/co2-lab/polvo/internal/memory"
	"github.com/co2-lab/polvo/internal/provider"
	"github.com/co2-lab/polvo/internal/tool"
)

// ---- scriptedChatProvider ---------------------------------------------------

// scriptedChatProvider implementa provider.ChatProvider com respostas pré-definidas.
// Permite testar loop.go de ponta a ponta sem LLM real.
type scriptedChatProvider struct {
	turns []provider.ChatResponse
	calls int
	reqs  []provider.ChatRequest // grava tudo que foi enviado ao LLM para inspeção
}

func (s *scriptedChatProvider) Chat(_ context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.calls >= len(s.turns) {
		panic(fmt.Sprintf("scriptedChatProvider: no more turns (call %d, have %d)", s.calls, len(s.turns)))
	}
	resp := s.turns[s.calls]
	s.calls++
	return &resp, nil
}

func (s *scriptedChatProvider) Name() string                      { return "scripted" }
func (s *scriptedChatProvider) Available(_ context.Context) error { return nil }
func (s *scriptedChatProvider) Complete(_ context.Context, _ provider.Request) (*provider.Response, error) {
	return &provider.Response{}, nil
}

// scriptedTurn cria um ChatResponse de tool_use com uma tool call.
func scriptedTurn(toolName, toolID string, input json.RawMessage) provider.ChatResponse {
	return provider.ChatResponse{
		Message: provider.Message{
			Role: "assistant",
			ToolCalls: []provider.ToolCall{
				{ID: toolID, Name: toolName, Input: input},
			},
		},
		StopReason: "tool_use",
	}
}

// scriptedEndTurn cria um ChatResponse de end_turn (sem tool calls).
func scriptedEndTurn(content string) provider.ChatResponse {
	return provider.ChatResponse{
		Message:    provider.Message{Role: "assistant", Content: content},
		StopReason: "end_turn",
	}
}

// ---- noopToolImpl -----------------------------------------------------------

// noopToolImpl implementa tool.Tool retornando sempre sucesso vazio.
type noopToolImpl struct {
	name string
}

func (n *noopToolImpl) Name() string                                                        { return n.name }
func (n *noopToolImpl) Description() string                                                 { return "noop" }
func (n *noopToolImpl) InputSchema() json.RawMessage                                        { return json.RawMessage(`{}`) }
func (n *noopToolImpl) Execute(_ context.Context, _ json.RawMessage) (*tool.Result, error) {
	return &tool.Result{Content: "ok"}, nil
}

func makeNoopTool(name string) tool.Tool { return &noopToolImpl{name: name} }

// ---- blockingChatProvider ---------------------------------------------------

// blockingChatProvider bloqueia até o ctx ser cancelado.
type blockingChatProvider struct {
	block chan struct{}
}

func (b *blockingChatProvider) Chat(ctx context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-b.block:
		return nil, nil
	}
}

func (b *blockingChatProvider) Name() string                      { return "blocking" }
func (b *blockingChatProvider) Available(_ context.Context) error { return nil }
func (b *blockingChatProvider) Complete(_ context.Context, _ provider.Request) (*provider.Response, error) {
	return &provider.Response{}, nil
}

// ---- TestLoop_MaxTurns ------------------------------------------------------

func TestLoop_MaxTurns(t *testing.T) {
	script := &scriptedChatProvider{
		turns: []provider.ChatResponse{
			scriptedTurn("read", "id1", json.RawMessage(`{"path":"a.go"}`)),
			scriptedTurn("read", "id2", json.RawMessage(`{"path":"b.go"}`)), // different path to avoid stuck
			scriptedEndTurn("done"), // nunca alcançado
		},
	}

	reg := tool.NewRegistry()
	reg.Register(makeNoopTool("read"))

	loop := NewLoop(LoopConfig{
		Provider: script,
		Tools:    reg,
		MaxTurns: 2,
	})

	_, err := loop.Run(context.Background(), "task")
	if err == nil {
		t.Fatal("expected error when max turns exceeded")
	}
	if !containsStr(err.Error(), "max turns") {
		t.Errorf("expected error to contain 'max turns', got: %v", err)
	}
	if script.calls != 2 {
		t.Errorf("expected exactly 2 LLM calls, got %d", script.calls)
	}
}

// ---- TestLoop_StuckDetectionIntegration -------------------------------------

func TestLoop_StuckDetectionIntegration(t *testing.T) {
	// Análogo ao test_is_stuck_repeating_action_observation do OpenHands:
	// mesma tool com mesmo input chamada threshold=3 vezes → loop encerra.
	sameInput := json.RawMessage(`{"path":"file.go"}`)
	script := &scriptedChatProvider{
		turns: []provider.ChatResponse{
			scriptedTurn("read", "id1", sameInput),
			scriptedTurn("read", "id2", sameInput),
			scriptedTurn("read", "id3", sameInput),
			scriptedEndTurn("done"), // nunca alcançado
		},
	}

	reg := tool.NewRegistry()
	reg.Register(makeNoopTool("read"))

	loop := NewLoop(LoopConfig{
		Provider:        script,
		Tools:           reg,
		MaxTurns:        50,
		StuckThreshold:  3,
		StuckWindowSize: 6,
	})

	_, err := loop.Run(context.Background(), "task")
	if err == nil {
		t.Fatal("expected stuck error")
	}
	if !containsStr(err.Error(), "agent stuck") {
		t.Errorf("expected error to contain 'agent stuck', got: %v", err)
	}
	if !containsStr(err.Error(), "same tool+input repeated") {
		t.Errorf("expected error to contain 'same tool+input repeated', got: %v", err)
	}
}

// ---- TestLoop_StuckDetection_PersistsToMemory -------------------------------

// TestLoop_StuckDetection_PersistsToMemory verifies that when the loop detects
// a stuck condition and a Memory store is configured, an entry is written to it.
func TestLoop_StuckDetection_PersistsToMemory(t *testing.T) {
	sameInput := json.RawMessage(`{"path":"file.go"}`)
	script := &scriptedChatProvider{
		turns: []provider.ChatResponse{
			scriptedTurn("read", "id1", sameInput),
			scriptedTurn("read", "id2", sameInput),
			scriptedTurn("read", "id3", sameInput),
			scriptedEndTurn("done"), // never reached
		},
	}

	reg := tool.NewRegistry()
	reg.Register(makeNoopTool("read"))

	mem, err := memory.Open(t.TempDir())
	if err != nil {
		t.Fatalf("memory.Open: %v", err)
	}
	t.Cleanup(func() { mem.Close() })

	loop := NewLoop(LoopConfig{
		Provider:        script,
		Tools:           reg,
		MaxTurns:        50,
		StuckThreshold:  3,
		StuckWindowSize: 6,
		Memory:          mem,
	})

	_, loopErr := loop.Run(context.Background(), "task")
	if loopErr == nil || !containsStr(loopErr.Error(), "agent stuck") {
		t.Fatalf("expected stuck error, got: %v", loopErr)
	}

	entries, readErr := mem.Read(memory.Filter{Type: "issue"})
	if readErr != nil {
		t.Fatalf("memory.Read: %v", readErr)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one memory entry for stuck detection, got none")
	}
	if !containsStr(entries[0].Content, "stuck detected") {
		t.Errorf("expected memory entry to contain 'stuck detected', got: %q", entries[0].Content)
	}
}

// ---- TestLoop_ContextCancel -------------------------------------------------

func TestLoop_ContextCancel(t *testing.T) {
	script := &scriptedChatProvider{
		turns: []provider.ChatResponse{scriptedEndTurn("never")},
	}
	loop := NewLoop(LoopConfig{Provider: script, MaxTurns: 10})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancela antes de Run

	_, err := loop.Run(ctx, "task")
	if err == nil {
		t.Fatal("expected error when context already cancelled")
	}
	if err != context.Canceled && !containsStr(err.Error(), "cancelled") && !containsStr(err.Error(), "canceled") {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
	if script.calls != 0 {
		t.Errorf("LLM should not be called when context is pre-cancelled, got %d calls", script.calls)
	}
}

// ---- TestLoop_RunTimeout ----------------------------------------------------

func TestLoop_RunTimeout(t *testing.T) {
	blocking := &blockingChatProvider{block: make(chan struct{})}
	loop := NewLoop(LoopConfig{
		Provider:   blocking,
		MaxTurns:   10,
		RunTimeout: 20 * time.Millisecond,
	})

	_, err := loop.Run(context.Background(), "task")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	// Context deadline exceeded is wrapped by loop
	if !containsStr(err.Error(), "deadline exceeded") && !containsStr(err.Error(), "context deadline") {
		t.Errorf("expected deadline error, got: %v", err)
	}
}

// ---- TestLoop_ToolCallbacks -------------------------------------------------

func TestLoop_ToolCallbacks(t *testing.T) {
	script := &scriptedChatProvider{
		turns: []provider.ChatResponse{
			scriptedTurn("read", "call-1", json.RawMessage(`{"path":"x"}`)),
			scriptedEndTurn("done"),
		},
	}
	reg := tool.NewRegistry()
	reg.Register(makeNoopTool("read"))

	var calledTools []string
	var resultIDs []string

	loop := NewLoop(LoopConfig{
		Provider:     script,
		Tools:        reg,
		MaxTurns:     10,
		OnToolCall:   func(tc provider.ToolCall) { calledTools = append(calledTools, tc.Name) },
		OnToolResult: func(id, name, result string, isError bool) { resultIDs = append(resultIDs, id) },
	})

	_, err := loop.Run(context.Background(), "task")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(calledTools) != 1 || calledTools[0] != "read" {
		t.Errorf("expected calledTools=[read], got %v", calledTools)
	}
	if len(resultIDs) != 1 || resultIDs[0] != "call-1" {
		t.Errorf("expected resultIDs=[call-1], got %v", resultIDs)
	}
}

// ---- helpers ----------------------------------------------------------------

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
