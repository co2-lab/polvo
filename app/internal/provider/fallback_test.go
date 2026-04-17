package provider

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// --- mockChatProvider --------------------------------------------------------

type mockFallbackProvider struct {
	response string
	err      error
	calls    int
	lastReq  ChatRequest
}

func (m *mockFallbackProvider) Chat(_ context.Context, req ChatRequest) (*ChatResponse, error) {
	m.calls++
	m.lastReq = req
	if m.err != nil {
		return nil, m.err
	}
	return &ChatResponse{
		Message:    Message{Role: "assistant", Content: m.response},
		StopReason: "end_turn",
	}, nil
}

func (m *mockFallbackProvider) Name() string                       { return "mock" }
func (m *mockFallbackProvider) Available(_ context.Context) error  { return nil }
func (m *mockFallbackProvider) Complete(_ context.Context, _ Request) (*Response, error) {
	return &Response{Content: m.response}, nil
}

// --- mockSummarizer ----------------------------------------------------------

type mockSummarizer struct {
	result []Message
	err    error
	calls  int
}

func (s *mockSummarizer) Summarize(_ context.Context, msgs []Message, _ int) ([]Message, error) {
	s.calls++
	if s.err != nil {
		return msgs, s.err
	}
	if s.result != nil {
		return s.result, nil
	}
	// Default: return first + last 2
	if len(msgs) > 3 {
		return msgs[:1], nil
	}
	return msgs, nil
}

// --- helpers -----------------------------------------------------------------

func makeMsgs(n int) []Message {
	msgs := make([]Message, n)
	for i := range msgs {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		msgs[i] = Message{Role: role, Content: strings.Repeat("x", 40)}
	}
	return msgs
}

// --- ContextFallbackManager --------------------------------------------------

func TestContextFallbackManager_NormalPath(t *testing.T) {
	mgr := &ContextFallbackManager{}
	mock := &mockFallbackProvider{response: "ok"}

	msgs := makeMsgs(3)
	req := ChatRequest{Model: "model-a", Messages: msgs}

	// Small messages, no context window — should go through normally.
	resp, err := mgr.WrapChat(context.Background(), mock, req, 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message.Content != "ok" {
		t.Errorf("expected 'ok', got %q", resp.Message.Content)
	}
	if mock.calls != 1 {
		t.Errorf("expected 1 call, got %d", mock.calls)
	}
}

func TestContextFallbackManager_PreCallCheck_OverflowToPrune(t *testing.T) {
	// Use contextWindow=2000 but 20 messages × ~14 tokens = ~280 tokens.
	// SafetyBuffer(2000) = max(1000, 40) = 1000. Available = 2000 - 1000 - 200 = 800.
	// 280 tokens fits in 800 — so we need more messages to force overflow.
	// Use 200 messages × ~14 tokens = ~2800 tokens. Available = 800. Forces overflow.
	mgr := &ContextFallbackManager{MinOutputTokens: 200}
	mock := &mockFallbackProvider{response: "ok"}

	msgs := makeMsgs(200)
	req := ChatRequest{Model: "model-a", Messages: msgs}

	resp, err := mgr.WrapChat(context.Background(), mock, req, 2000, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected response")
	}
	// Provider was called with pruned messages (fewer than original).
	if len(mock.lastReq.Messages) >= len(msgs) {
		t.Errorf("expected pruned messages, got %d (original %d)", len(mock.lastReq.Messages), len(msgs))
	}
}

func TestContextFallbackManager_Layer1_ModelFallback(t *testing.T) {
	mgr := &ContextFallbackManager{
		ContextWindowFallbacks: map[string][]string{
			"small-model": {"large-model"},
		},
		MinOutputTokens: 200,
	}
	mock := &mockFallbackProvider{response: "ok"}

	msgs := makeMsgs(200) // ~2800 tokens, forces overflow at contextWindow=2000
	req := ChatRequest{Model: "small-model", Messages: msgs}

	_, err := mgr.WrapChat(context.Background(), mock, req, 2000, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have switched to large-model before pruning (layer 1 fires first).
	if mock.lastReq.Model != "large-model" {
		t.Errorf("expected large-model after fallback, got %q", mock.lastReq.Model)
	}
}

func TestContextFallbackManager_Layer2_Summarizer(t *testing.T) {
	// Summarizer returns a single small message that will fit.
	summaryResult := []Message{{Role: "assistant", Content: "brief summary"}}
	summ := &mockSummarizer{result: summaryResult}

	mgr := &ContextFallbackManager{
		Summarizer:      summ,
		MinOutputTokens: 200,
	}
	mock := &mockFallbackProvider{response: "ok"}

	msgs := makeMsgs(200) // ~2800 tokens, forces overflow at contextWindow=2000
	req := ChatRequest{Model: "model-a", Messages: msgs}

	resp, err := mgr.WrapChat(context.Background(), mock, req, 2000, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected response")
	}
	if summ.calls == 0 {
		t.Error("expected summarizer to be called")
	}
}

func TestContextFallbackManager_MaxDepthExhausted(t *testing.T) {
	// MaxDepth=1, so the first recursive call (depth=1) will fail immediately.
	mgr := &ContextFallbackManager{
		MaxDepth:        1,
		MinOutputTokens: 100,
	}
	// Provider always returns a context window error to keep triggering overflow.
	mock := &mockFallbackProvider{err: &ProviderError{
		Kind:    ErrKindContextTooLong,
		Message: "prompt is too long",
	}}

	msgs := makeMsgs(5)
	req := ChatRequest{Model: "model-a", Messages: msgs}

	_, err := mgr.WrapChat(context.Background(), mock, req, 200, 0)
	if !errors.Is(err, ErrContextWindowExhausted) {
		t.Errorf("expected ErrContextWindowExhausted, got: %v", err)
	}
}

func TestContextFallbackManager_ContextWindowErrorPostSend(t *testing.T) {
	// Provider always returns a context window error → cascade fires → exhausted.
	mgr := &ContextFallbackManager{
		MaxDepth:        2,
		MinOutputTokens: 100,
	}
	mock := &mockFallbackProvider{err: &ProviderError{
		Kind:    ErrKindContextTooLong,
		Message: "prompt is too long",
	}}

	// Messages that appear to fit (contextWindow=0 skips pre-call check),
	// so the provider is actually called and returns an error.
	msgs := makeMsgs(3)
	req := ChatRequest{Model: "model-a", Messages: msgs}

	_, err := mgr.WrapChat(context.Background(), mock, req, 0, 0)
	// Provider always returns context error → cascade fires → exhausted after MaxDepth.
	if !errors.Is(err, ErrContextWindowExhausted) {
		t.Errorf("expected ErrContextWindowExhausted, got: %v", err)
	}
	if mock.calls == 0 {
		t.Error("expected at least one provider call")
	}
}

func TestContextFallbackManager_NonContextError_Propagated(t *testing.T) {
	mgr := &ContextFallbackManager{}
	someErr := errors.New("internal server error")
	mock := &mockFallbackProvider{err: someErr}

	msgs := makeMsgs(3)
	req := ChatRequest{Model: "model-a", Messages: msgs}

	_, err := mgr.WrapChat(context.Background(), mock, req, 0, 0)
	if !errors.Is(err, someErr) {
		t.Errorf("expected original error to propagate, got: %v", err)
	}
}

func TestContextFallbackManager_NextLargerModel(t *testing.T) {
	mgr := &ContextFallbackManager{
		ContextWindowFallbacks: map[string][]string{
			"a": {"b", "c"},
			"b": {"c"},
		},
	}
	if got := mgr.nextLargerModel("a"); got != "b" {
		t.Errorf("expected 'b', got %q", got)
	}
	if got := mgr.nextLargerModel("b"); got != "c" {
		t.Errorf("expected 'c', got %q", got)
	}
	if got := mgr.nextLargerModel("c"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
	if got := mgr.nextLargerModel("unknown"); got != "" {
		t.Errorf("expected empty for unknown model, got %q", got)
	}
}

func TestContextFallbackManager_Defaults(t *testing.T) {
	mgr := &ContextFallbackManager{}
	if mgr.maxDepth() != 3 {
		t.Errorf("expected default maxDepth=3, got %d", mgr.maxDepth())
	}
	if mgr.minOutput() != 1000 {
		t.Errorf("expected default minOutput=1000, got %d", mgr.minOutput())
	}
}
