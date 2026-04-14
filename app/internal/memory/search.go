package memory

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// Searcher ranks memory entries by relevance to a query.
type Searcher interface {
	Search(entries []Entry, query string, topK int) []Entry
}

// TFIDFSearcher ranks entries using TF-IDF similarity.
// Pure Go, no external dependencies. Suitable for < 10k entries.
type TFIDFSearcher struct{}

// Search returns the topK most relevant entries for the given query.
// Returns nil if query is empty or entries is empty.
func (s TFIDFSearcher) Search(entries []Entry, query string, topK int) []Entry {
	queryTerms := tokenize(query)
	if len(queryTerms) == 0 || len(entries) == 0 {
		return nil
	}

	// Build document term-frequency maps and document-frequency counts.
	df := make(map[string]int)
	docs := make([]map[string]int, len(entries))
	for i, e := range entries {
		terms := tokenize(e.Content)
		tf := make(map[string]int)
		for _, t := range terms {
			tf[t]++
		}
		docs[i] = tf
		for t := range tf {
			df[t]++
		}
	}

	N := float64(len(entries))
	scores := make([]float64, len(entries))
	for i, tf := range docs {
		for _, qt := range queryTerms {
			if cnt := tf[qt]; cnt > 0 {
				// Use smoothed IDF: log(1 + N/df) to ensure positive values
				// even when the term appears in every document.
				idf := math.Log(1 + N/float64(df[qt]))
				scores[i] += float64(cnt) * idf
			}
		}
	}

	// Build index slice and sort by score descending.
	indices := make([]int, len(entries))
	for i := range indices {
		indices[i] = i
	}
	sort.Slice(indices, func(a, b int) bool {
		return scores[indices[a]] > scores[indices[b]]
	})

	// Collect topK results — only entries with positive score are relevant.
	k := topK
	if k <= 0 || k > len(entries) {
		k = len(entries)
	}
	result := make([]Entry, 0, k)
	for _, idx := range indices {
		if len(result) >= k {
			break
		}
		if scores[idx] > 0 {
			result = append(result, entries[idx])
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// tokenize lowercases s and splits on non-alphanumeric runes.
func tokenize(s string) []string {
	s = strings.ToLower(s)
	return strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}
