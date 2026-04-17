package repomap

import (
	"context"
	"testing"

	"github.com/co2-lab/polvo/internal/provider"
)

// mockRerankerProvider returns a fixed response.
type mockRerankerProvider struct {
	response string
	err      error
}

func (m *mockRerankerProvider) Chat(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &provider.ChatResponse{Message: provider.Message{Role: "assistant", Content: m.response}}, nil
}

func (m *mockRerankerProvider) Name() string                               { return "mock" }
func (m *mockRerankerProvider) Complete(_ context.Context, _ provider.Request) (*provider.Response, error) {
	return &provider.Response{}, nil
}
func (m *mockRerankerProvider) Available(_ context.Context) error { return nil }

func TestReranker_MockScoresTopK(t *testing.T) {
	// Chunks in order: A, B, C. LLM says B is most relevant (score 9), then A (6), then C (2).
	chunks := []Chunk{
		{Path: "a.go", Content: "func A() {}"},
		{Path: "b.go", Content: "func B() {}"},
		{Path: "c.go", Content: "func C() {}"},
	}
	rr := &Reranker{
		Provider: &mockRerankerProvider{response: "[6, 9, 2]"},
		Model:    "test-model",
	}

	got, err := rr.Rerank(context.Background(), "B function", chunks, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}
	if got[0].Path != "b.go" {
		t.Errorf("expected b.go first (score 9), got %s", got[0].Path)
	}
	if got[1].Path != "a.go" {
		t.Errorf("expected a.go second (score 6), got %s", got[1].Path)
	}
}

func TestReranker_FallbackOnError(t *testing.T) {
	chunks := []Chunk{
		{Path: "x.go", Content: "x"},
		{Path: "y.go", Content: "y"},
	}
	import_err := context.DeadlineExceeded
	rr := &Reranker{
		Provider: &mockRerankerProvider{err: import_err},
		Model:    "test-model",
	}

	got, _ := rr.Rerank(context.Background(), "query", chunks, 2)
	// Should fall back to original order.
	if len(got) != 2 {
		t.Fatalf("expected 2 results on fallback, got %d", len(got))
	}
}

func TestReranker_FallbackOnInvalidJSON(t *testing.T) {
	chunks := []Chunk{
		{Path: "a.go", Content: "a"},
		{Path: "b.go", Content: "b"},
	}
	rr := &Reranker{
		Provider: &mockRerankerProvider{response: "I cannot score these"},
		Model:    "test-model",
	}

	got, _ := rr.Rerank(context.Background(), "query", chunks, 2)
	if len(got) != 2 {
		t.Fatalf("expected 2 results on invalid JSON fallback, got %d", len(got))
	}
}

func TestParseScoreArray(t *testing.T) {
	tests := []struct {
		input string
		n     int
		want  []float64
	}{
		{"[7, 3, 9]", 3, []float64{7, 3, 9}},
		{"Here are scores: [5, 8]", 2, []float64{5, 8}},
		{"not a json array", 2, []float64{5, 5}}, // fallback
		{"[10]", 3, []float64{10, 5, 5}},          // partial
	}

	for _, tc := range tests {
		got, _ := parseScoreArray(tc.input, tc.n)
		if len(got) != len(tc.want) {
			t.Errorf("input=%q: len=%d, want %d", tc.input, len(got), len(tc.want))
			continue
		}
		for i, v := range tc.want {
			if got[i] != v {
				t.Errorf("input=%q [%d]: got %v, want %v", tc.input, i, got[i], v)
			}
		}
	}
}
