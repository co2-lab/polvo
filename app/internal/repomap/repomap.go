package repomap

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RepoMap builds a token-budgeted symbol map of the repository.
// It uses a lightweight grep-based extractor (no tree-sitter dependency)
// that works for Go, TypeScript, Python, and JavaScript.
// A full tree-sitter integration can replace the extractor in a later phase.
type RepoMap struct {
	Root         string
	MaxTokens    int      // default 2000
	Refresh      string   // "auto" | "always" | "manual"
	ExcludeFiles []string // files already in chat context — skip to avoid duplication
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
	Symbols []string
	Score   float64 // PageRank-like score
}

// Build generates the repo map as a formatted string within MaxTokens.
// focusFiles are files relevant to the current task — they receive a score boost.
func (r *RepoMap) Build(_ context.Context, focusFiles []string) (string, error) {
	focusSet := make(map[string]bool)
	for _, f := range focusFiles {
		focusSet[filepath.Clean(f)] = true
		// Also boost files in the same directory as focus files
		focusSet[filepath.Dir(f)] = true
	}

	excludeSet := make(map[string]bool, len(r.ExcludeFiles))
	for _, f := range r.ExcludeFiles {
		excludeSet[filepath.Clean(f)] = true
	}

	var files []fileSymbol
	err := filepath.WalkDir(r.Root, func(path string, d os.DirEntry, err error) error {
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

		// Skip files already in the active chat context.
		if excludeSet[filepath.Clean(path)] {
			return nil
		}

		syms, err := extractSymbols(path)
		if err != nil || len(syms) == 0 {
			return nil
		}

		score := 1.0
		rel, _ := filepath.Rel(r.Root, path)
		if focusSet[filepath.Clean(path)] || focusSet[filepath.Dir(path)] {
			score = 10.0 // boost focus files
		}
		// Boost files that import/reference focus file names
		for _, f := range focusFiles {
			base := strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))
			if strings.Contains(strings.Join(syms, " "), base) {
				score += 2.0
			}
		}

		files = append(files, fileSymbol{Path: rel, Symbols: syms, Score: score})
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walking repo: %w", err)
	}

	// Sort by score descending, then path
	sort.Slice(files, func(i, j int) bool {
		if files[i].Score != files[j].Score {
			return files[i].Score > files[j].Score
		}
		return files[i].Path < files[j].Path
	})

	// Build output within token budget
	var sb strings.Builder
	sb.WriteString("# Repository Map\n\n")
	tokensSoFar := EstimateTokens(sb.String())

	for _, f := range files {
		line := fmt.Sprintf("%s: %s\n", f.Path, strings.Join(f.Symbols, ", "))
		cost := EstimateTokens(line)
		if tokensSoFar+cost > r.MaxTokens {
			break
		}
		sb.WriteString(line)
		tokensSoFar += cost
	}

	return sb.String(), nil
}

// extractSymbols pulls exported symbol names from a source file using
// a simple line-scanner (no AST parsing needed for the map).
func extractSymbols(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	ext := strings.ToLower(filepath.Ext(path))
	var syms []string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		sym := extractSymbolLine(line, ext)
		if sym != "" {
			syms = append(syms, sym)
		}
	}
	return syms, scanner.Err()
}

// extractSymbolLine extracts a symbol name from a source line based on file extension.
func extractSymbolLine(line, ext string) string {
	switch ext {
	case ".go":
		for _, prefix := range []string{"func ", "type ", "var ", "const "} {
			if strings.HasPrefix(line, prefix) {
				rest := strings.TrimPrefix(line, prefix)
				name := firstIdent(rest)
				if name != "" && isExported(name) {
					return name
				}
			}
		}
	case ".ts", ".tsx", ".js", ".jsx":
		for _, prefix := range []string{"export function ", "export class ", "export interface ", "export type ", "export const ", "export default function "} {
			if strings.HasPrefix(line, prefix) {
				rest := strings.TrimPrefix(line, prefix)
				return firstIdent(rest)
			}
		}
	case ".py":
		if strings.HasPrefix(line, "def ") || strings.HasPrefix(line, "class ") {
			rest := strings.TrimPrefix(strings.TrimPrefix(line, "def "), "class ")
			return firstIdent(rest)
		}
	}
	return ""
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

func isExported(name string) bool {
	return len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z'
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
