package repomap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// RepoMap builds a token-budgeted symbol map of the repository.
// It uses go/ast for Go files and improved regex for TS/JS/Python.
// A PageRank graph over cross-file references ranks files by semantic importance.
type RepoMap struct {
	Root         string
	MaxTokens    int      // default 2000
	Refresh      string   // "auto" | "always" | "manual"
	ExcludeFiles []string // files already in chat context — skip to avoid duplication
	SymCache     *SymCache // optional SQLite symbol cache; nil = no caching
}

// New creates a RepoMap with defaults.
func New(root string, maxTokens int) *RepoMap {
	if maxTokens <= 0 {
		maxTokens = 2000
	}
	return &RepoMap{Root: root, MaxTokens: maxTokens, Refresh: "auto"}
}

// fileSymbol represents a file and its extracted symbols.
type fileSymbol struct {
	Path    string
	Symbols []Symbol // rich symbols (line+kind+signature)
	Names   []string // plain names for score boosting
	Score   float64  // PageRank-like score
}

// rankedTag is a (file, symbol-name) pair with a PageRank-derived score.
type rankedTag struct {
	RelPath string
	Name    string
	Score   float64
}

// Build generates the repo map as a formatted string within MaxTokens.
// focusFiles are files relevant to the current task.
// Uses go/ast extraction + PageRank cross-file ranking + binary-search budget fit.
func (r *RepoMap) Build(_ context.Context, focusFiles []string) (string, error) {
	focusSet := make(map[string]bool, len(focusFiles))
	for _, f := range focusFiles {
		focusSet[filepath.Clean(f)] = true
	}

	excludeSet := make(map[string]bool, len(r.ExcludeFiles))
	for _, f := range r.ExcludeFiles {
		excludeSet[filepath.Clean(f)] = true
	}

	// Phase 1: collect rich symbols per file.
	fileSyms := make(map[string][]RichSymbol)  // relPath → symbols
	legacySyms := make(map[string][]Symbol)     // relPath → legacy symbols (for rendering)

	walkErr := filepath.WalkDir(r.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipRepoDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isSourceFile(d.Name()) {
			return nil
		}
		if excludeSet[filepath.Clean(path)] {
			return nil
		}

		rel, _ := filepath.Rel(r.Root, path)

		// Try SymCache.
		var richSyms []RichSymbol
		if r.SymCache != nil {
			info, statErr := os.Stat(path)
			if statErr == nil {
				mtime := info.ModTime().UnixNano()
				cached, cacheErr := r.SymCache.Get(rel, mtime)
				if cacheErr == nil && cached != nil {
					richSyms = cached
				}
			}
		}

		if richSyms == nil {
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			richSyms = ExtractRichSymbols(rel, content)
			if r.SymCache != nil {
				info, statErr := os.Stat(path)
				if statErr == nil {
					_ = r.SymCache.Put(rel, info.ModTime().UnixNano(), richSyms)
				}
			}
			// Also keep legacy symbol extraction for rendering.
			legacySyms[rel] = extractLegacyFallback(rel, content)
		}

		if richSyms != nil {
			fileSyms[rel] = richSyms
		}
		return nil
	})
	if walkErr != nil {
		return "", fmt.Errorf("walking repo: %w", walkErr)
	}

	// Phase 2: build PageRank graph.
	mentionedIdents := extractMentionedIdents(focusFiles)
	g, pers := buildGraph(fileSyms, focusSet, mentionedIdents)
	rank := pageRank(g, pers)

	// Update cache with computed ranks (fire-and-forget).
	if r.SymCache != nil {
		go func() {
			for path, r2 := range rank {
				_ = r.SymCache.SetRank(path, r2)
			}
		}()
	}

	// Phase 3: distribute rank to (file, symbol) pairs.
	scores := distributeRank(g, rank, fileSyms)

	// Phase 4: collect ranked tags.
	var tags []rankedTag
	for key, score := range scores {
		tags = append(tags, rankedTag{RelPath: key[0], Name: key[1], Score: score})
	}
	// Add files with legacy symbols that may not have rich defs (non-Go files etc).
	for rel, syms := range legacySyms {
		if _, ok := fileSyms[rel]; ok {
			continue // already covered
		}
		fileRank := rank[rel]
		for _, s := range syms {
			tags = append(tags, rankedTag{RelPath: rel, Name: firstIdent(s.Signature), Score: fileRank})
		}
	}

	// Sort: score desc, then relPath asc, then name asc (deterministic).
	sort.Slice(tags, func(i, j int) bool {
		if tags[i].Score != tags[j].Score {
			return tags[i].Score > tags[j].Score
		}
		if tags[i].RelPath != tags[j].RelPath {
			return tags[i].RelPath < tags[j].RelPath
		}
		return tags[i].Name < tags[j].Name
	})

	// Phase 5: binary-search token budget.
	header := "# Repository Map\n\n"
	result := r.fitToBudget(tags, fileSyms, legacySyms, focusSet, rank, header)
	return result, nil
}

// fitToBudget uses binary search to find the largest prefix of rankedTags
// that fits within MaxTokens (±15% tolerance matches Aider behaviour).
func (r *RepoMap) fitToBudget(
	tags []rankedTag,
	fileSyms map[string][]RichSymbol,
	legacySyms map[string][]Symbol,
	focusSet map[string]bool,
	rank map[string]float64,
	header string,
) string {
	if len(tags) == 0 {
		return header
	}

	budget := r.MaxTokens
	tolerance := int(float64(budget) * 0.15)

	// Group tags by file for rendering.
	renderGroup := func(n int) string {
		var sb strings.Builder
		sb.WriteString(header)
		// Collect files in order.
		seen := make(map[string]bool)
		fileOrder := make([]string, 0)
		for i := 0; i < n && i < len(tags); i++ {
			rel := tags[i].RelPath
			if !seen[rel] {
				seen[rel] = true
				fileOrder = append(fileOrder, rel)
			}
		}
		for _, rel := range fileOrder {
			// Prefer legacy symbols for rendering (have full signature lines).
			if syms, ok := legacySyms[rel]; ok {
				sb.WriteString(formatRepoMapBlock(rel, syms))
				continue
			}
			// Fallback: render from rich symbols.
			if richSyms, ok := fileSyms[rel]; ok {
				var legacyFallback []Symbol
				for _, s := range richSyms {
					if s.IsDef {
						legacyFallback = append(legacyFallback, Symbol{
							Line:      s.Line,
							Kind:      s.Kind,
							Signature: s.Name,
						})
					}
				}
				if len(legacyFallback) > 0 {
					sb.WriteString(formatRepoMapBlock(rel, legacyFallback))
				}
			}
		}
		return sb.String()
	}

	// Binary search for the largest n where tokens <= budget+tolerance.
	lo, hi := 0, len(tags)
	result := header
	for lo <= hi {
		mid := (lo + hi) / 2
		candidate := renderGroup(mid)
		cost := EstimateTokens(candidate)
		if cost <= budget+tolerance {
			result = candidate
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	return result
}

// extractLegacyFallback extracts legacy symbols for files that have no rich symbols yet.
func extractLegacyFallback(path string, content []byte) []Symbol {
	syms, err := loadSidecar(path)
	if err == nil {
		return syms
	}
	return ExtractSymbols(path, content)
}

// extractMentionedIdents collects base names from focus file paths.
func extractMentionedIdents(focusFiles []string) map[string]bool {
	m := make(map[string]bool, len(focusFiles))
	for _, f := range focusFiles {
		base := strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))
		if base != "" {
			m[base] = true
		}
		// Also tokenise the base name (e.g. "userHandler" → "user", "handler").
		for _, tok := range splitCamel(base) {
			m[tok] = true
		}
	}
	return m
}

// splitCamel splits a camelCase or snake_case identifier into lowercase tokens.
func splitCamel(s string) []string {
	var tokens []string
	var cur strings.Builder
	for _, r := range s {
		if r == '_' || r == '-' {
			if cur.Len() > 0 {
				tokens = append(tokens, strings.ToLower(cur.String()))
				cur.Reset()
			}
			continue
		}
		if unicode.IsUpper(r) && cur.Len() > 0 {
			tokens = append(tokens, strings.ToLower(cur.String()))
			cur.Reset()
		}
		cur.WriteRune(r)
	}
	if cur.Len() > 0 {
		tokens = append(tokens, strings.ToLower(cur.String()))
	}
	return tokens
}

// formatRepoMapBlock formats a file's symbols in ctags-style for the repo map output.
func formatRepoMapBlock(relPath string, syms []Symbol) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s\n", relPath)
	for _, s := range syms {
		fmt.Fprintf(&sb, "  %d\t%s\t%s\n", s.Line, s.Kind, s.Signature)
	}
	return sb.String()
}

func firstIdent(s string) string {
	end := 0
	for end < len(s) {
		c := s[end]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			end++
		} else {
			break
		}
	}
	return s[:end]
}

func isSourceFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".rs", ".java":
		return true
	}
	return false
}

func skipRepoDir(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch name {
	case "node_modules", "vendor", "__pycache__", "dist", "build", "target", "out", "bin":
		return true
	}
	return false
}
