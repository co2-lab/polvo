package repomap

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	maxChunkLines     = 200
	minChunkLines     = 3
	slidingWindowSize = 50
	slidingWindowStep = 40 // overlap of 10 lines
)

// Chunk is a segment of a source file.
type Chunk struct {
	ID        string // sha256(path + ":" + strconv.Itoa(startLine))[:16]
	Path      string
	StartLine int
	EndLine   int
	Symbol    string // "func Foo", "type Bar", "" for generic chunks
	Content   string
	FileHash  string // sha256 of full file content
}

// chunkID derives the chunk identifier from path and start line.
func chunkID(path string, startLine int) string {
	h := sha256.Sum256([]byte(path + ":" + strconv.Itoa(startLine)))
	return fmt.Sprintf("%x", h[:8]) // 16 hex chars
}

// ChunkFile splits a file into semantic chunks.
//
// Strategy:
//  1. .go: top-level "func ", "type ", "var ", "const " at column 0
//  2. .ts/.tsx/.js/.jsx: "export function", "export class", "export const", "function " at column 0
//  3. .py: "def ", "class " at column 0 (no indent)
//  4. Fallback: sliding window of 50 lines with 10-line overlap
//
// Max chunk size: 200 lines. Min chunk size: 3 lines (smaller chunks skipped).
func ChunkFile(path string, content []byte) ([]Chunk, error) {
	fh := fileHash(content)
	lines := splitLines(content)

	ext := strings.ToLower(filepath.Ext(path))

	var declStarts []int // 0-based indices of declaration-starting lines
	var declSymbols []string

	switch ext {
	case ".go":
		declStarts, declSymbols = findGoDecls(lines)
	case ".ts", ".tsx", ".js", ".jsx":
		declStarts, declSymbols = findJSDecls(lines)
	case ".py":
		declStarts, declSymbols = findPyDecls(lines)
	}

	if len(declStarts) > 0 {
		return buildDeclChunks(path, lines, declStarts, declSymbols, fh), nil
	}

	// Fallback: sliding window
	return buildSlidingChunks(path, lines, fh), nil
}

// splitLines splits content into individual lines (without trailing newline).
func splitLines(content []byte) []string {
	if len(content) == 0 {
		return nil
	}
	var lines []string
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}

// isTopLevel returns true if the line starts at column 0 (no indentation).
func isTopLevel(line string) bool {
	return len(line) > 0 && line[0] != ' ' && line[0] != '\t'
}

// findGoDecls finds top-level Go declarations.
func findGoDecls(lines []string) (starts []int, symbols []string) {
	prefixes := []string{"func ", "type ", "var ", "const "}
	for i, line := range lines {
		if !isTopLevel(line) {
			continue
		}
		for _, p := range prefixes {
			if strings.HasPrefix(line, p) {
				rest := strings.TrimPrefix(line, p)
				name := firstIdent(rest)
				sym := strings.TrimSuffix(p, " ") + " " + name
				starts = append(starts, i)
				symbols = append(symbols, sym)
				break
			}
		}
	}
	return
}

// findJSDecls finds top-level JS/TS declarations.
func findJSDecls(lines []string) (starts []int, symbols []string) {
	prefixes := []string{
		"export function ", "export class ", "export const ",
		"export default function ", "function ",
	}
	for i, line := range lines {
		if !isTopLevel(line) {
			continue
		}
		for _, p := range prefixes {
			if strings.HasPrefix(line, p) {
				rest := strings.TrimPrefix(line, p)
				name := firstIdent(rest)
				sym := strings.TrimSuffix(p, " ") + " " + name
				starts = append(starts, i)
				symbols = append(symbols, sym)
				break
			}
		}
	}
	return
}

// findPyDecls finds top-level Python declarations.
func findPyDecls(lines []string) (starts []int, symbols []string) {
	for i, line := range lines {
		if !isTopLevel(line) {
			continue
		}
		for _, p := range []string{"def ", "class "} {
			if strings.HasPrefix(line, p) {
				rest := strings.TrimPrefix(line, p)
				name := firstIdent(rest)
				sym := strings.TrimSuffix(p, " ") + " " + name
				starts = append(starts, i)
				symbols = append(symbols, sym)
				break
			}
		}
	}
	return
}

// buildDeclChunks creates chunks from declaration boundaries.
// Each chunk spans from one declaration to the line before the next declaration.
// Chunks larger than maxChunkLines are split at that boundary.
func buildDeclChunks(path string, lines []string, starts []int, symbols []string, fh string) []Chunk {
	var chunks []Chunk
	total := len(lines)

	for idx, start := range starts {
		var end int
		if idx+1 < len(starts) {
			end = starts[idx+1] - 1
		} else {
			end = total - 1
		}

		// Trim trailing blank lines
		for end > start && strings.TrimSpace(lines[end]) == "" {
			end--
		}

		sym := symbols[idx]
		size := end - start + 1

		if size < minChunkLines {
			continue
		}

		if size <= maxChunkLines {
			c := makeChunk(path, lines, start, end, sym, fh)
			chunks = append(chunks, c)
		} else {
			// Split into sub-chunks of maxChunkLines, first keeps symbol
			for sub := start; sub <= end; sub += maxChunkLines {
				subEnd := sub + maxChunkLines - 1
				if subEnd > end {
					subEnd = end
				}
				subSym := sym
				if sub != start {
					subSym = ""
				}
				if subEnd-sub+1 < minChunkLines {
					continue
				}
				c := makeChunk(path, lines, sub, subEnd, subSym, fh)
				chunks = append(chunks, c)
			}
		}
	}
	return chunks
}

// buildSlidingChunks creates chunks via sliding window for unsupported file types.
func buildSlidingChunks(path string, lines []string, fh string) []Chunk {
	total := len(lines)
	if total < minChunkLines {
		return nil
	}

	var chunks []Chunk
	for start := 0; start < total; start += slidingWindowStep {
		end := start + slidingWindowSize - 1
		if end >= total {
			end = total - 1
		}
		if end-start+1 < minChunkLines {
			break
		}
		c := makeChunk(path, lines, start, end, "", fh)
		chunks = append(chunks, c)
		if end == total-1 {
			break
		}
	}
	return chunks
}

// makeChunk constructs a Chunk from line slice boundaries (0-based indices → 1-based line numbers).
func makeChunk(path string, lines []string, start, end int, sym, fh string) Chunk {
	startLine := start + 1 // convert to 1-based
	endLine := end + 1
	content := strings.Join(lines[start:end+1], "\n")
	return Chunk{
		ID:        chunkID(path, startLine),
		Path:      path,
		StartLine: startLine,
		EndLine:   endLine,
		Symbol:    sym,
		Content:   content,
		FileHash:  fh,
	}
}

// skipIndexDir returns true for directories that should be skipped during indexing.
func skipIndexDir(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch name {
	case "node_modules", "vendor", "bin", "dist", "build", "target", "out":
		return true
	}
	return false
}

// isSupportedSource returns true for file extensions we can chunk.
func isSupportedSource(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".go", ".ts", ".tsx", ".js", ".jsx", ".py",
		".rs", ".java", ".c", ".cpp", ".h", ".rb", ".md", ".txt", ".yaml", ".yml", ".json":
		return true
	}
	return false
}

// ChunkDir walks a directory and chunks all supported files.
// Skips node_modules, vendor, .git, .polvo, bin, dist, build, target, out.
// Skips files > 500KB.
func ChunkDir(root string) ([]Chunk, error) {
	const maxFileSize = 500 * 1024

	var all []Chunk
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipIndexDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isSupportedSource(d.Name()) {
			return nil
		}

		info, err := d.Info()
		if err != nil || info.Size() > maxFileSize {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		chunks, err := ChunkFile(path, content)
		if err != nil {
			return nil
		}
		all = append(all, chunks...)
		return nil
	})
	return all, err
}

// fileHash returns the sha256 hex digest of content.
func fileHash(content []byte) string {
	h := sha256.Sum256(content)
	return fmt.Sprintf("%x", h[:])
}
