package repomap

import (
	"fmt"
	"path/filepath"
	"testing"
)

func openTestIndex(t *testing.T) *ChunkIndex {
	t.Helper()
	dir := t.TempDir()
	idx, err := OpenChunkIndex(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenChunkIndex: %v", err)
	}
	t.Cleanup(func() { idx.Close() })
	return idx
}

func makeTestChunk(id, path, symbol, content string, start, end int) Chunk {
	return Chunk{
		ID:        id,
		Path:      path,
		StartLine: start,
		EndLine:   end,
		Symbol:    symbol,
		Content:   content,
		FileHash:  "deadbeef",
	}
}

func TestIndex_UpsertAndSearchBM25(t *testing.T) {
	idx := openTestIndex(t)

	c := makeTestChunk("abc123", "/src/foo.go", "func Foo", "func Foo() { return frobnicator() }", 1, 3)
	if err := idx.Upsert(c); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// Search for a word that appears in content
	results, err := idx.SearchBM25("frobnicator", 10)
	if err != nil {
		t.Fatalf("SearchBM25: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result for 'frobnicator'")
	}
	if results[0].ID != "abc123" {
		t.Errorf("expected chunk abc123, got %s", results[0].ID)
	}
}

func TestIndex_DeleteByPath(t *testing.T) {
	idx := openTestIndex(t)

	c1 := makeTestChunk("c1", "/src/bar.go", "func Bar", "func Bar() { quuxalicious() }", 1, 3)
	c2 := makeTestChunk("c2", "/src/bar.go", "func Baz", "func Baz() { quuxalicious() }", 5, 7)
	if err := idx.UpsertBatch([]Chunk{c1, c2}); err != nil {
		t.Fatalf("UpsertBatch: %v", err)
	}

	// Verify both are searchable
	results, err := idx.SearchBM25("quuxalicious", 10)
	if err != nil {
		t.Fatalf("SearchBM25: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results before delete, got %d", len(results))
	}

	// Delete the file
	if err := idx.DeleteByPath("/src/bar.go"); err != nil {
		t.Fatalf("DeleteByPath: %v", err)
	}

	// Chunks should be gone
	results, err = idx.SearchBM25("quuxalicious", 10)
	if err != nil {
		t.Fatalf("SearchBM25 after delete: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results after delete, got %d", len(results))
	}
}

func TestIndex_UpsertBatchAndGetByPath(t *testing.T) {
	idx := openTestIndex(t)

	chunks := []Chunk{
		makeTestChunk("x1", "/src/multi.go", "func One", "func One() {}", 1, 3),
		makeTestChunk("x2", "/src/multi.go", "func Two", "func Two() {}", 5, 7),
		makeTestChunk("x3", "/src/multi.go", "func Three", "func Three() {}", 9, 11),
	}
	if err := idx.UpsertBatch(chunks); err != nil {
		t.Fatalf("UpsertBatch: %v", err)
	}

	got, err := idx.GetByPath("/src/multi.go")
	if err != nil {
		t.Fatalf("GetByPath: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 chunks, got %d", len(got))
	}

	// Verify IDs
	ids := map[string]bool{"x1": true, "x2": true, "x3": true}
	for _, c := range got {
		if !ids[c.ID] {
			t.Errorf("unexpected chunk ID: %s", c.ID)
		}
	}
}

func TestIndex_UpsertIdempotent(t *testing.T) {
	idx := openTestIndex(t)

	c := makeTestChunk("idem1", "/src/idem.go", "func Idem", "func Idem() { uniqueword() }", 1, 3)
	// Insert twice — should not error or duplicate
	if err := idx.Upsert(c); err != nil {
		t.Fatalf("first Upsert: %v", err)
	}
	if err := idx.Upsert(c); err != nil {
		t.Fatalf("second Upsert: %v", err)
	}

	results, err := idx.SearchBM25("uniqueword", 10)
	if err != nil {
		t.Fatalf("SearchBM25: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected exactly 1 result after idempotent upsert, got %d", len(results))
	}
}

func TestIndex_GetByPath_Empty(t *testing.T) {
	idx := openTestIndex(t)

	chunks, err := idx.GetByPath("/nonexistent/path.go")
	if err != nil {
		t.Fatalf("GetByPath: %v", err)
	}
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for unknown path, got %d", len(chunks))
	}
}

func TestChunkIndex_ConcurrentUpsert(t *testing.T) {
	t.Parallel()

	idx := openTestIndex(t)

	const workers = 10
	errCh := make(chan error, workers)

	for i := 0; i < workers; i++ {
		go func(n int) {
			id := fmt.Sprintf("concurrent%d", n)
			word := fmt.Sprintf("uniqueterm%d", n)
			c := makeTestChunk(id, fmt.Sprintf("/src/file%d.go", n), "func F",
				fmt.Sprintf("func F() { return %s() }", word), 1, 3)
			errCh <- idx.Upsert(c)
		}(i)
	}

	for i := 0; i < workers; i++ {
		if err := <-errCh; err != nil {
			t.Errorf("concurrent Upsert: %v", err)
		}
	}

	// Verify all 10 chunks are searchable.
	for i := 0; i < workers; i++ {
		word := fmt.Sprintf("uniqueterm%d", i)
		results, err := idx.SearchBM25(word, 5)
		if err != nil {
			t.Fatalf("SearchBM25(%q): %v", word, err)
		}
		if len(results) == 0 {
			t.Errorf("expected result for %q, got none", word)
		}
	}
}

func TestChunkIndex_SearchReturnsTopK(t *testing.T) {
	idx := openTestIndex(t)

	// Insert 10 chunks all containing the same query term.
	chunks := make([]Chunk, 10)
	for i := 0; i < 10; i++ {
		chunks[i] = makeTestChunk(
			fmt.Sprintf("topk%d", i),
			fmt.Sprintf("/src/topk%d.go", i),
			"func F",
			fmt.Sprintf("func F%d() { return sharedterm() }", i),
			i*10+1, i*10+3,
		)
	}
	if err := idx.UpsertBatch(chunks); err != nil {
		t.Fatalf("UpsertBatch: %v", err)
	}

	results, err := idx.SearchBM25("sharedterm", 3)
	if err != nil {
		t.Fatalf("SearchBM25: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected exactly 3 results for topK=3, got %d", len(results))
	}
}

func TestChunkIndex_UpdateExistingChunk(t *testing.T) {
	idx := openTestIndex(t)

	path := "/src/update_test.go"
	startLine := 1

	// Build the ID the same way the implementation does.
	id := chunkID(path, startLine)

	original := Chunk{
		ID:        id,
		Path:      path,
		StartLine: startLine,
		EndLine:   3,
		Symbol:    "func Updated",
		Content:   "func Updated() { return oldcontent() }",
		FileHash:  "hash1",
	}
	if err := idx.Upsert(original); err != nil {
		t.Fatalf("first Upsert: %v", err)
	}

	updated := Chunk{
		ID:        id,
		Path:      path,
		StartLine: startLine,
		EndLine:   3,
		Symbol:    "func Updated",
		Content:   "func Updated() { return newcontent() }",
		FileHash:  "hash2",
	}
	if err := idx.Upsert(updated); err != nil {
		t.Fatalf("second Upsert (update): %v", err)
	}

	// Should return the NEW content, not the old.
	results, err := idx.SearchBM25("newcontent", 10)
	if err != nil {
		t.Fatalf("SearchBM25 new: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result after update, got %d", len(results))
	}
	if results[0].Content != updated.Content {
		t.Errorf("expected updated content %q, got %q", updated.Content, results[0].Content)
	}

	// Old content must not be present.
	old, err := idx.SearchBM25("oldcontent", 10)
	if err != nil {
		t.Fatalf("SearchBM25 old: %v", err)
	}
	if len(old) != 0 {
		t.Errorf("expected 0 results for old content, got %d", len(old))
	}
}

func TestIndex_SearchBM25_NoMatch(t *testing.T) {
	idx := openTestIndex(t)

	c := makeTestChunk("nm1", "/src/nm.go", "func NM", "func NM() { hello() }", 1, 3)
	if err := idx.Upsert(c); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	results, err := idx.SearchBM25("xyzzy_not_present", 10)
	if err != nil {
		t.Fatalf("SearchBM25: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for missing term, got %d", len(results))
	}
}
