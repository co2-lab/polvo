package memory

import (
	"testing"
)

func TestTFIDFSearcher_FindsRelevantEntries(t *testing.T) {
	entries := []Entry{
		{ID: 1, Type: "decision", Content: "deploy kubernetes cluster on production server"},
		{ID: 2, Type: "context", Content: "the weather is nice today"},
		{ID: 3, Type: "observation", Content: "kubernetes pod crashed due to OOM"},
	}

	results := TFIDFSearcher{}.Search(entries, "kubernetes deployment", 5)
	if len(results) == 0 {
		t.Fatal("expected at least one result for 'kubernetes deployment'")
	}
	// Both kubernetes-related entries should appear.
	found := make(map[int64]bool)
	for _, r := range results {
		found[r.ID] = true
	}
	if !found[1] && !found[3] {
		t.Error("expected kubernetes-related entries to be returned")
	}
}

func TestTFIDFSearcher_RanksMoreRelevantFirst(t *testing.T) {
	entries := []Entry{
		{ID: 1, Content: "database migration failed on production"},
		{ID: 2, Content: "database database migration migration migration"},
		{ID: 3, Content: "unrelated topic about birds and trees"},
	}

	results := TFIDFSearcher{}.Search(entries, "database migration", 3)
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}
	// Entry with more term frequency should rank first.
	if results[0].ID != 2 {
		t.Errorf("expected entry 2 (higher TF) to rank first, got ID=%d", results[0].ID)
	}
}

func TestTFIDFSearcher_TopKLimit(t *testing.T) {
	entries := []Entry{
		{ID: 1, Content: "go programming language"},
		{ID: 2, Content: "go build toolchain"},
		{ID: 3, Content: "go test runner"},
		{ID: 4, Content: "go module system"},
		{ID: 5, Content: "go concurrency patterns"},
	}

	results := TFIDFSearcher{}.Search(entries, "go", 3)
	if len(results) > 3 {
		t.Errorf("expected at most 3 results with topK=3, got %d", len(results))
	}
}

func TestTFIDFSearcher_EmptyQuery(t *testing.T) {
	entries := []Entry{
		{ID: 1, Content: "some content"},
	}

	results := TFIDFSearcher{}.Search(entries, "", 5)
	if results != nil {
		t.Errorf("expected nil for empty query, got %v", results)
	}

	// Empty entries should also not panic.
	results2 := TFIDFSearcher{}.Search(nil, "query", 5)
	if results2 != nil {
		t.Errorf("expected nil for empty entries, got %v", results2)
	}
}

func TestTokenize_Normalization(t *testing.T) {
	cases := []struct {
		input    string
		expected []string
	}{
		{"Hello, World!", []string{"hello", "world"}},
		{"foo-bar_baz", []string{"foo", "bar", "baz"}},
		{"  spaces   between  ", []string{"spaces", "between"}},
		{"CamelCase123", []string{"camelcase123"}},
		{"", nil},
	}

	for _, tc := range cases {
		got := tokenize(tc.input)
		if len(got) != len(tc.expected) {
			t.Errorf("tokenize(%q): expected %v, got %v", tc.input, tc.expected, got)
			continue
		}
		for i, tok := range got {
			if tok != tc.expected[i] {
				t.Errorf("tokenize(%q)[%d]: expected %q, got %q", tc.input, i, tc.expected[i], tok)
			}
		}
	}
}
