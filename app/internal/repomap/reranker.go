package repomap

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/co2-lab/polvo/internal/provider"
)

// Reranker uses a cheap LLM to score chunk relevance for a given query.
// It batches up to 5 chunks per LLM call for efficiency and falls back
// to the original order on any error.
type Reranker struct {
	Provider provider.ChatProvider
	Model    string
}

// Rerank scores the given chunks for the query and returns the top-K most
// relevant ones in descending relevance order.
// Falls back to the original order if the LLM fails or returns unusable output.
func (r *Reranker) Rerank(ctx context.Context, query string, chunks []Chunk, topK int) ([]Chunk, error) {
	if len(chunks) == 0 || topK <= 0 {
		return chunks, nil
	}
	if topK > len(chunks) {
		topK = len(chunks)
	}

	type scored struct {
		idx   int
		score float64
	}
	scores := make([]scored, len(chunks))
	for i := range scores {
		scores[i] = scored{idx: i, score: float64(len(chunks) - i)} // default: preserve order
	}

	const batchSize = 5
	for start := 0; start < len(chunks); start += batchSize {
		end := start + batchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		batch := chunks[start:end]

		batchScores, err := r.scoreBatch(ctx, query, batch)
		if err != nil {
			// Fallback: keep default order for this batch.
			continue
		}
		for i, s := range batchScores {
			if start+i < len(scores) {
				scores[start+i].score = s
			}
		}
	}

	sort.SliceStable(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	result := make([]Chunk, topK)
	for i := 0; i < topK; i++ {
		result[i] = chunks[scores[i].idx]
	}
	return result, nil
}

// scoreBatch asks the LLM to rate relevance 0-10 for each chunk in the batch.
// Returns a float64 score per chunk.
func (r *Reranker) scoreBatch(ctx context.Context, query string, batch []Chunk) ([]float64, error) {
	var sb strings.Builder
	sb.WriteString("You are a relevance scoring assistant.\n")
	sb.WriteString("Rate the relevance of each code chunk to the given query. ")
	sb.WriteString("Respond ONLY with a JSON array of numbers (0-10, one per chunk).\n\n")
	fmt.Fprintf(&sb, "Query: %s\n\n", query)
	for i, c := range batch {
		fmt.Fprintf(&sb, "Chunk %d (%s):\n%s\n\n", i+1, c.Path, truncate(c.Content, 300))
	}
	sb.WriteString("JSON array response (example: [7, 3, 9]):")

	resp, err := r.Provider.Chat(ctx, provider.ChatRequest{
		Model:  r.Model,
		System: "Respond only with a JSON array of numbers.",
		Messages: []provider.Message{
			{Role: "user", Content: sb.String()},
		},
		MaxTokens: 50,
	})
	if err != nil {
		return nil, err
	}

	text := strings.TrimSpace(resp.Message.Content) //nolint:govet // resp is *ChatResponse
	return parseScoreArray(text, len(batch))
}

// parseScoreArray parses a JSON array like "[7, 3, 9]" into float64 scores.
// Falls back to equal scores if parsing fails.
func parseScoreArray(text string, n int) ([]float64, error) {
	// Find the first '[' and last ']'.
	start := strings.Index(text, "[")
	end := strings.LastIndex(text, "]")
	if start < 0 || end <= start {
		return equalScores(n), nil
	}
	text = text[start : end+1]

	var raw []json.RawMessage
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return equalScores(n), nil
	}

	scores := equalScores(n) // start at 5.0 so unprovided entries get neutral score
	for i := 0; i < n && i < len(raw); i++ {
		s, err := strconv.ParseFloat(strings.TrimSpace(string(raw[i])), 64)
		if err != nil {
			scores[i] = 5.0 // neutral fallback
			continue
		}
		scores[i] = s
	}
	return scores, nil
}

func equalScores(n int) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = 5.0
	}
	return s
}

func truncate(s string, maxChars int) string {
	if len(s) <= maxChars {
		return s
	}
	return s[:maxChars] + "…"
}

// SearchOptions configures the hybrid BM25 + PageRank + re-rank search pipeline.
type SearchOptions struct {
	Query       string
	TopK        int
	UsePageRank bool     // apply PageRank scores as a boost factor
	UseRerank   bool     // apply LLM re-ranking on top candidates
	SeedFiles   []string // files to use as PageRank seeds
}

// Search runs the hybrid retrieval pipeline:
//  1. BM25 full-text search via ChunkIndex
//  2. Optional PageRank boost using SeedFiles
//  3. Optional LLM re-ranking
func (ix *Indexer) Search(ctx context.Context, opts SearchOptions, reranker *Reranker) ([]Chunk, error) {
	topK := opts.TopK
	if topK <= 0 {
		topK = 10
	}

	// Phase 1: BM25 retrieval — fetch more candidates if re-ranking is enabled.
	candidateN := topK
	if opts.UseRerank {
		candidateN = topK * 4
		if candidateN < 20 {
			candidateN = 20
		}
	}

	chunks, err := ix.index.SearchBM25(opts.Query, candidateN)
	if err != nil {
		return nil, fmt.Errorf("repomap search BM25: %w", err)
	}
	if len(chunks) == 0 {
		return nil, nil
	}

	// Phase 2: PageRank boost.
	if opts.UsePageRank && len(opts.SeedFiles) > 0 {
		chunks = applyPageRankBoost(chunks, opts.SeedFiles, ix.root)
	}

	// Phase 3: LLM re-ranking.
	if opts.UseRerank && reranker != nil {
		chunks, err = reranker.Rerank(ctx, opts.Query, chunks, topK)
		if err != nil {
			// Fallback: truncate to topK.
			if len(chunks) > topK {
				chunks = chunks[:topK]
			}
		}
		return chunks, nil
	}

	if len(chunks) > topK {
		chunks = chunks[:topK]
	}
	return chunks, nil
}

// applyPageRankBoost reorders chunks by using a simple seed-proximity heuristic.
// Files whose paths share a directory with a seed file are boosted.
func applyPageRankBoost(chunks []Chunk, seedFiles []string, root string) []Chunk {
	seedDirs := make(map[string]bool, len(seedFiles))
	for _, f := range seedFiles {
		rel := strings.TrimPrefix(f, root+"/")
		dir := pathDir(rel)
		if dir != "" && dir != "." {
			seedDirs[dir] = true
		}
	}
	if len(seedDirs) == 0 {
		return chunks
	}

	type scored struct {
		c     Chunk
		boost float64
	}
	sc := make([]scored, len(chunks))
	for i, c := range chunks {
		boost := 1.0
		dir := pathDir(c.Path)
		if seedDirs[dir] {
			boost = 2.0
		}
		sc[i] = scored{c, boost}
	}
	sort.SliceStable(sc, func(i, j int) bool {
		return sc[i].boost > sc[j].boost
	})
	result := make([]Chunk, len(chunks))
	for i, s := range sc {
		result[i] = s.c
	}
	return result
}

func pathDir(path string) string {
	idx := strings.LastIndexAny(path, "/\\")
	if idx < 0 {
		return ""
	}
	return path[:idx]
}
