package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/co2-lab/polvo/internal/provider"
)

// ---- Helpers ----------------------------------------------------------------

func msgs(n int) []provider.Message {
	result := make([]provider.Message, n)
	for i := range result {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		result[i] = provider.Message{Role: role, Content: strings.Repeat("x", 40)} // ~10 tokens
	}
	return result
}

func toolMsgs(n int) []provider.Message {
	var out []provider.Message
	for i := 0; i < n; i++ {
		out = append(out, provider.Message{Role: "user", Content: "task"})
		out = append(out, provider.Message{Role: "tool", Content: strings.Repeat("y", 1000)}) // ~250 tokens
	}
	return out
}

// ---- mockChatProvider -------------------------------------------------------

type mockChatProvider struct {
	response string
	err      error
	calls    int
}

func (m *mockChatProvider) Chat(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	return &provider.ChatResponse{
		Message:    provider.Message{Role: "assistant", Content: m.response},
		StopReason: "end_turn",
	}, nil
}

func (m *mockChatProvider) Name() string                       { return "mock" }
func (m *mockChatProvider) Available(_ context.Context) error  { return nil }
func (m *mockChatProvider) Complete(_ context.Context, _ provider.Request) (*provider.Response, error) {
	return &provider.Response{Content: m.response}, nil
}

// ---- estimateHistoryTokens --------------------------------------------------

func TestEstimateHistoryTokens(t *testing.T) {
	// Fórmula: sum(EstimateTokens(content)) + 4*numMessages
	// EstimateTokens(s) = (len(s)+3)/4

	t.Run("lista vazia → 0", func(t *testing.T) {
		if got := estimateHistoryTokens(nil); got != 0 {
			t.Errorf("got %d; want 0", got)
		}
	})

	t.Run("1 msg com content 'abcd' (4 chars → 1 token) → 1+4=5", func(t *testing.T) {
		ms := []provider.Message{{Role: "user", Content: "abcd"}}
		// EstimateTokens("abcd") = (4+3)/4 = 1
		if got := estimateHistoryTokens(ms); got != 5 {
			t.Errorf("got %d; want 5", got)
		}
	})

	t.Run("1 msg com content '' → 0+4=4", func(t *testing.T) {
		ms := []provider.Message{{Role: "user", Content: ""}}
		// EstimateTokens("") = 0 (len==0 returns 0)
		if got := estimateHistoryTokens(ms); got != 4 {
			t.Errorf("got %d; want 4", got)
		}
	})

	t.Run("3 msgs com 40 chars cada (~10 tokens) → (10+4)*3=42", func(t *testing.T) {
		ms := msgs(3)
		// each has 40 chars: EstimateTokens = (40+3)/4 = 10
		// total = (10+4)*3 = 42
		if got := estimateHistoryTokens(ms); got != 42 {
			t.Errorf("got %d; want 42", got)
		}
	})
}

// ---- SlidingWindowCondenser -------------------------------------------------

func TestSlidingWindowCondenser(t *testing.T) {
	t.Run("len <= keep+1 não modifica", func(t *testing.T) {
		c := &SlidingWindowCondenser{KeepLast: 3}
		input := msgs(4) // 4 msgs, keep+1 = 4 → não modifica
		got, err := c.Condense(context.Background(), input, 1000)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != len(input) {
			t.Errorf("expected len=%d, got %d", len(input), len(got))
		}
	})

	t.Run("len > keep+1 retorna first+notice+últimas keep", func(t *testing.T) {
		c := &SlidingWindowCondenser{KeepLast: 3}
		input := msgs(10) // 10 msgs, keep+1=4 → deve condensar
		got, err := c.Condense(context.Background(), input, 1000)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Expected: 1(first) + 1(notice) + 3(last) = 5
		if len(got) != 5 {
			t.Errorf("expected len=5, got %d", len(got))
		}
		// First message preserved
		if got[0].Content != input[0].Content {
			t.Error("first message should be preserved")
		}
		// Notice is second
		if got[1].Role != "user" || !strings.Contains(got[1].Content, "omitted") {
			t.Errorf("expected notice message at index 1, got: %v", got[1])
		}
		// Last 3 preserved
		for i := 0; i < 3; i++ {
			if got[2+i].Content != input[len(input)-3+i].Content {
				t.Errorf("last messages not preserved correctly at position %d", i)
			}
		}
	})

	t.Run("notice contém contagem correta de msgs omitidas", func(t *testing.T) {
		c := &SlidingWindowCondenser{KeepLast: 3}
		input := msgs(10) // 10 msgs, 1 first, 3 last = 6 omitted
		got, _ := c.Condense(context.Background(), input, 1000)
		// elided = len - 1 - keep = 10 - 1 - 3 = 6
		if !strings.Contains(got[1].Content, "6") {
			t.Errorf("notice should mention 6 omitted messages, got: %q", got[1].Content)
		}
	})

	t.Run("keep=0 usa default 20", func(t *testing.T) {
		c := &SlidingWindowCondenser{KeepLast: 0}
		input := msgs(25) // 25 msgs, default keep=20, keep+1=21 → deve condensar
		got, err := c.Condense(context.Background(), input, 1000)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// 1+1+20 = 22
		if len(got) != 22 {
			t.Errorf("expected 22 with default keep=20, got %d", len(got))
		}
	})

	t.Run("primeira mensagem sempre preservada como result[0]", func(t *testing.T) {
		c := &SlidingWindowCondenser{KeepLast: 2}
		input := msgs(10)
		input[0].Content = "SYSTEM_MESSAGE"
		got, _ := c.Condense(context.Background(), input, 1000)
		if got[0].Content != "SYSTEM_MESSAGE" {
			t.Error("first message (system) not preserved")
		}
	})
}

// ---- ObservationMaskingCondenser --------------------------------------------

func TestObservationMaskingCondenser(t *testing.T) {
	t.Run("tool results antigos com >200 tokens mascarados", func(t *testing.T) {
		c := &ObservationMaskingCondenser{KeepRecentResults: 1}
		// 3 tool results: 2 antigos (>200 tokens), 1 recente
		messages := []provider.Message{
			{Role: "user", Content: "task"},
			{Role: "tool", Content: strings.Repeat("y", 1000)}, // antigo, >200 tokens
			{Role: "user", Content: "task2"},
			{Role: "tool", Content: strings.Repeat("y", 1000)}, // antigo, >200 tokens
			{Role: "user", Content: "task3"},
			{Role: "tool", Content: strings.Repeat("y", 1000)}, // recente (keepRecent=1)
		}
		got, err := c.Condense(context.Background(), messages, 10000)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Primeiros 2 tool results devem ser mascarados
		if !strings.Contains(got[1].Content, "omitted") {
			t.Errorf("expected old tool result to be masked, got: %q", got[1].Content)
		}
		if !strings.Contains(got[3].Content, "omitted") {
			t.Errorf("expected old tool result to be masked, got: %q", got[3].Content)
		}
		// Último tool result deve ser preservado
		if strings.Contains(got[5].Content, "omitted") {
			t.Error("expected recent tool result to be preserved")
		}
	})

	t.Run("tool results recentes preservados verbatim", func(t *testing.T) {
		c := &ObservationMaskingCondenser{KeepRecentResults: 2}
		orig := strings.Repeat("y", 1000)
		messages := []provider.Message{
			{Role: "user", Content: "task"},
			{Role: "tool", Content: strings.Repeat("z", 1000)}, // antigo
			{Role: "user", Content: "task2"},
			{Role: "tool", Content: orig}, // recente
			{Role: "user", Content: "task3"},
			{Role: "tool", Content: orig}, // recente
		}
		got, _ := c.Condense(context.Background(), messages, 10000)
		if got[3].Content != orig {
			t.Error("second-to-last tool result should be preserved verbatim")
		}
		if got[5].Content != orig {
			t.Error("last tool result should be preserved verbatim")
		}
	})

	t.Run("tool results pequenos preservados mesmo sendo antigos", func(t *testing.T) {
		c := &ObservationMaskingCondenser{KeepRecentResults: 1}
		smallContent := "ok" // <200 tokens
		messages := []provider.Message{
			{Role: "user", Content: "task"},
			{Role: "tool", Content: smallContent}, // antigo mas pequeno
			{Role: "user", Content: "task2"},
			{Role: "tool", Content: smallContent}, // antigo mas pequeno
			{Role: "user", Content: "task3"},
			{Role: "tool", Content: strings.Repeat("y", 1000)}, // recente
		}
		got, _ := c.Condense(context.Background(), messages, 10000)
		if got[1].Content != smallContent {
			t.Errorf("small old tool result should not be masked, got: %q", got[1].Content)
		}
	})

	t.Run("mensagens não-tool nunca alteradas", func(t *testing.T) {
		c := &ObservationMaskingCondenser{KeepRecentResults: 1}
		messages := []provider.Message{
			{Role: "user", Content: strings.Repeat("x", 1000)},
			{Role: "assistant", Content: strings.Repeat("a", 1000)},
		}
		got, _ := c.Condense(context.Background(), messages, 10000)
		if got[0].Content != messages[0].Content {
			t.Error("user message should not be altered")
		}
		if got[1].Content != messages[1].Content {
			t.Error("assistant message should not be altered")
		}
	})

	t.Run("keepRecent=0 usa default 5", func(t *testing.T) {
		c := &ObservationMaskingCondenser{KeepRecentResults: 0}
		// 7 tool results (>200 tokens each), default keepRecent=5
		// First 2 should be masked, last 5 preserved
		var messages []provider.Message
		for i := 0; i < 7; i++ {
			messages = append(messages, provider.Message{Role: "user", Content: "task"})
			messages = append(messages, provider.Message{Role: "tool", Content: strings.Repeat("y", 1000)})
		}
		got, _ := c.Condense(context.Background(), messages, 10000)
		// indices of tool results: 1, 3, 5, 7, 9, 11, 13
		// First 2 (idx 1, 3) should be masked
		if !strings.Contains(got[1].Content, "omitted") {
			t.Errorf("first old tool result should be masked, got: %q", got[1].Content)
		}
		if !strings.Contains(got[3].Content, "omitted") {
			t.Errorf("second old tool result should be masked, got: %q", got[3].Content)
		}
		// Last 5 (idx 5, 7, 9, 11, 13) should be preserved
		for _, idx := range []int{5, 7, 9, 11, 13} {
			if strings.Contains(got[idx].Content, "omitted") {
				t.Errorf("recent tool result at idx %d should not be masked", idx)
			}
		}
	})

	t.Run("todos os tool results pequenos: nada mascarado", func(t *testing.T) {
		c := &ObservationMaskingCondenser{KeepRecentResults: 1}
		var messages []provider.Message
		for i := 0; i < 5; i++ {
			messages = append(messages, provider.Message{Role: "user", Content: "task"})
			messages = append(messages, provider.Message{Role: "tool", Content: "small"}) // <200 tokens
		}
		got, _ := c.Condense(context.Background(), messages, 10000)
		for i, m := range got {
			if m.Role == "tool" && strings.Contains(m.Content, "omitted") {
				t.Errorf("small tool result at index %d should not be masked", i)
			}
		}
	})
}

// ---- LLMSummaryCondenser ----------------------------------------------------

func TestLLMSummaryCondenser(t *testing.T) {
	t.Run("len <= keep+1 não modifica", func(t *testing.T) {
		mock := &mockChatProvider{response: "summary"}
		c := &LLMSummaryCondenser{Provider: mock, KeepLast: 3}
		input := msgs(4) // 4 msgs, keep+1=4 → não modifica
		got, err := c.Condense(context.Background(), input, 1000)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != len(input) {
			t.Errorf("expected len=%d, got %d", len(input), len(got))
		}
		if mock.calls != 0 {
			t.Errorf("expected 0 LLM calls, got %d", mock.calls)
		}
	})

	t.Run("provider retorna erro → fallback silencioso para SlidingWindow", func(t *testing.T) {
		mock := &mockChatProvider{err: errors.New("rate limited")}
		c := &LLMSummaryCondenser{Provider: mock, KeepLast: 3}
		input := msgs(20) // more than keep+1
		got, err := c.Condense(context.Background(), input, 1000)
		if err != nil {
			t.Fatalf("expected no error (graceful degradation), got: %v", err)
		}
		// Sliding window fallback: 1+1+keep = 5
		if len(got) > 3+2 {
			t.Errorf("fallback result too large: %d > %d", len(got), 3+2)
		}
	})

	t.Run("provider retorna sucesso → summary como 2a mensagem", func(t *testing.T) {
		mock := &mockChatProvider{response: "conversation summary here"}
		c := &LLMSummaryCondenser{Provider: mock, KeepLast: 3}
		input := msgs(10)
		got, err := c.Condense(context.Background(), input, 1000)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Expected: 1(first) + 1(summary) + 3(last) = 5
		if len(got) != 5 {
			t.Errorf("expected 5 messages, got %d", len(got))
		}
		// Summary is second message
		if !strings.Contains(got[1].Content, "[Summary of earlier conversation]:") {
			t.Errorf("expected summary prefix, got: %q", got[1].Content)
		}
		if !strings.Contains(got[1].Content, "conversation summary here") {
			t.Errorf("summary content missing from message: %q", got[1].Content)
		}
	})

	t.Run("primeira mensagem sempre preservada", func(t *testing.T) {
		mock := &mockChatProvider{response: "summary"}
		c := &LLMSummaryCondenser{Provider: mock, KeepLast: 3}
		input := msgs(10)
		input[0].Content = "SYSTEM_CONTEXT"
		got, _ := c.Condense(context.Background(), input, 1000)
		if got[0].Content != "SYSTEM_CONTEXT" {
			t.Error("first message should be preserved")
		}
	})

	t.Run("LLM invocado exatamente 1x por Condense call", func(t *testing.T) {
		mock := &mockChatProvider{response: "summary"}
		c := &LLMSummaryCondenser{Provider: mock, KeepLast: 3}
		input := msgs(10)
		_, err := c.Condense(context.Background(), input, 1000)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mock.calls != 1 {
			t.Errorf("expected exactly 1 LLM call, got %d", mock.calls)
		}
	})
}

// ---- ApplyCondenser ---------------------------------------------------------

func TestApplyCondenser(t *testing.T) {
	t.Run("abaixo do threshold não chama Condenser", func(t *testing.T) {
		called := false
		c := &testCondenser{fn: func(messages []provider.Message) ([]provider.Message, error) {
			called = true
			return messages, nil
		}}
		// 1 msg com content "abcd" = 5 tokens, maxTokens=100, threshold=0.85 → limit=85
		input := []provider.Message{{Role: "user", Content: "abcd"}}
		got, err := ApplyCondenser(context.Background(), input, 100, 0.85, c)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if called {
			t.Error("condenser should not be called when below threshold")
		}
		if len(got) != 1 {
			t.Errorf("expected original messages, got %d", len(got))
		}
	})

	t.Run("acima do threshold chama Condenser", func(t *testing.T) {
		called := false
		c := &testCondenser{fn: func(messages []provider.Message) ([]provider.Message, error) {
			called = true
			return messages[:1], nil
		}}
		// 3 msgs with 40 chars each = 42 tokens, maxTokens=40, threshold=0.85 → limit=34
		// 42 > 34 → should condense
		input := msgs(3)
		_, err := ApplyCondenser(context.Background(), input, 40, 0.85, c)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !called {
			t.Error("condenser should be called when above threshold")
		}
	})

	t.Run("threshold=0.85 com maxTokens=100 → limit=85", func(t *testing.T) {
		// 22 msgs with content "x" each = (22*(0+4)) = 88 tokens
		// Wait: "x" = 1 char, EstimateTokens = (1+3)/4 = 1, so 1+4 = 5 per msg
		// 22 * 5 = 110 > 85 → should condense
		called := false
		c := &testCondenser{fn: func(messages []provider.Message) ([]provider.Message, error) {
			called = true
			return messages, nil
		}}
		input := make([]provider.Message, 22)
		for i := range input {
			input[i] = provider.Message{Role: "user", Content: "x"}
		}
		_, _ = ApplyCondenser(context.Background(), input, 100, 0.85, c)
		if !called {
			t.Error("condenser should be called when above threshold=0.85 with maxTokens=100")
		}
	})
}

// ---- SummarizeKeepTail -------------------------------------------------------

func TestSummarizeKeepTail(t *testing.T) {
	t.Run("empty messages → returned as-is", func(t *testing.T) {
		mock := &mockChatProvider{response: "summary"}
		got, err := SummarizeKeepTail(context.Background(), nil, 10000, mock, "model", 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("expected empty result, got %d messages", len(got))
		}
	})

	t.Run("messages fit in tailBudget → returned as-is", func(t *testing.T) {
		mock := &mockChatProvider{response: "summary"}
		// 2 tiny messages fit easily in contextWindow/2 = 50000
		input := []provider.Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "world"},
		}
		got, err := SummarizeKeepTail(context.Background(), input, 100000, mock, "model", 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != len(input) {
			t.Errorf("expected %d messages, got %d", len(input), len(got))
		}
	})

	t.Run("tail preserved, head summarized", func(t *testing.T) {
		mock := &mockChatProvider{response: "conversation summary"}
		// Use a small contextWindow so head gets summarized.
		// contextWindow=200, tailBudget=100
		// Each msg: 40 chars = (40+3)/4=10 tokens + 4 = 14 tokens
		// 7 messages × 14 = 98 tokens ≤ tailBudget=100 — edge case
		// Let's use 30 messages to force head summarization.
		input := msgs(30)
		contextWindow := 200
		got, err := SummarizeKeepTail(context.Background(), input, contextWindow, mock, "model", 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// First message should be summary.
		if len(got) == 0 {
			t.Fatal("expected at least one message")
		}
		if !strings.Contains(got[0].Content, "[Summary of earlier conversation]:") {
			t.Errorf("expected summary as first message, got: %q", got[0].Content)
		}
		if mock.calls == 0 {
			t.Error("expected at least one LLM call for summarization")
		}
	})

	t.Run("depth >= maxSummarizeDepth → messages returned as-is", func(t *testing.T) {
		mock := &mockChatProvider{response: "summary"}
		input := msgs(10)
		got, err := SummarizeKeepTail(context.Background(), input, 100, mock, "model", maxSummarizeDepth)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != len(input) {
			t.Errorf("expected original messages returned at max depth, got %d", len(got))
		}
		if mock.calls != 0 {
			t.Errorf("expected 0 LLM calls at max depth, got %d", mock.calls)
		}
	})

	t.Run("LLM error → fallback to omission notice", func(t *testing.T) {
		mock := &mockChatProvider{err: errors.New("rate limited")}
		input := msgs(30)
		got, err := SummarizeKeepTail(context.Background(), input, 200, mock, "model", 0)
		if err != nil {
			t.Fatalf("unexpected error (should degrade gracefully): %v", err)
		}
		// First message should be an omission notice.
		if len(got) == 0 {
			t.Fatal("expected at least one message")
		}
		if !strings.Contains(got[0].Content, "omitted") {
			t.Errorf("expected omission notice as first message, got: %q", got[0].Content)
		}
	})
}

// testCondenser is a simple condenser for testing ApplyCondenser.
type testCondenser struct {
	fn func(messages []provider.Message) ([]provider.Message, error)
}

func (c *testCondenser) Condense(_ context.Context, messages []provider.Message, _ int) ([]provider.Message, error) {
	return c.fn(messages)
}

// ---- condenserHarness -------------------------------------------------------

type condenserHarness struct {
	messages  []provider.Message
	c         Condenser
	maxTokens int
	threshold float64
	steps     int
}

func (h *condenserHarness) step(msg provider.Message) []provider.Message {
	h.messages = append(h.messages, msg)
	h.steps++
	out, _ := ApplyCondenser(context.Background(), h.messages, h.maxTokens, h.threshold, h.c)
	h.messages = out
	return out
}

func TestCondenserHarness(t *testing.T) {
	keep := 3
	c := &SlidingWindowCondenser{KeepLast: keep}
	h := &condenserHarness{c: c, maxTokens: 50, threshold: 0.5}

	for i := 0; i < 20; i++ {
		out := h.step(provider.Message{Role: "user", Content: strings.Repeat("x", 60)})
		// After condensation (when triggered), len should never exceed keep+2 (first+notice+keep)
		if len(out) > keep+2 {
			t.Errorf("step %d: len(messages)=%d > keep+2=%d", i, len(out), keep+2)
		}
	}
}

func TestCondenserHarness_LLMCallCount(t *testing.T) {
	mock := &mockChatProvider{response: "summary"}
	c := &LLMSummaryCondenser{Provider: mock, KeepLast: 3}
	h := &condenserHarness{c: c, maxTokens: 50, threshold: 0.5}

	for i := 0; i < 20; i++ {
		h.step(provider.Message{Role: "user", Content: strings.Repeat("x", 60)})
	}

	// LLM should have been called at least once (whenever condensation triggered)
	if mock.calls == 0 {
		t.Error("expected at least 1 LLM call during 20 steps")
	}
	// Each condensation should call LLM exactly once
	t.Logf("Total LLM calls: %d over 20 steps", mock.calls)
}
