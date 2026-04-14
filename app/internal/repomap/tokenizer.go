// Package repomap builds a token-budgeted map of the repository.
package repomap

// EstimateTokens returns a quick token count estimate for text.
// Approximation: 1 token ≈ 4 characters (good for English/code).
func EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return (len(text) + 3) / 4 // round up
}
