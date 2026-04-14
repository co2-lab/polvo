package repomap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// goSrc is a synthetic Go file with 3 top-level functions.
const goSrc = `package example

import "fmt"

func Alpha() {
	fmt.Println("alpha")
}

func Beta(x int) int {
	return x * 2
}

func Gamma(a, b string) string {
	return a + b
}
`

func TestChunkFile_Go_ThreeFunctions(t *testing.T) {
	chunks, err := ChunkFile("example.go", []byte(goSrc))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expect at least 3 chunks (one per function; there may be an import chunk too)
	funcChunks := 0
	for _, c := range chunks {
		if strings.HasPrefix(c.Symbol, "func ") {
			funcChunks++
		}
	}
	if funcChunks < 3 {
		t.Errorf("expected at least 3 func chunks, got %d (total chunks: %d)", funcChunks, len(chunks))
		for _, c := range chunks {
			t.Logf("  chunk %s: symbol=%q lines=%d-%d", c.ID, c.Symbol, c.StartLine, c.EndLine)
		}
	}
}

func TestChunkFile_Go_FunctionsNotCutInMiddle(t *testing.T) {
	chunks, err := ChunkFile("example.go", []byte(goSrc))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(goSrc, "\n")
	// Remove trailing empty entry if goSrc ends with newline
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	// Every function chunk that contains "func " must also contain its closing brace.
	for _, c := range chunks {
		if !strings.HasPrefix(c.Symbol, "func ") {
			continue
		}
		// The chunk content should contain at least one closing brace
		if !strings.Contains(c.Content, "}") {
			t.Errorf("function chunk %s (%s) is missing closing brace; content:\n%s",
				c.ID, c.Symbol, c.Content)
		}
		// StartLine should be on a "func " line
		startIdx := c.StartLine - 1
		if startIdx < 0 || startIdx >= len(lines) {
			t.Errorf("chunk %s has out-of-bounds StartLine %d", c.ID, c.StartLine)
			continue
		}
		if !strings.HasPrefix(lines[startIdx], "func ") {
			t.Errorf("chunk %s StartLine %d is not a func declaration: %q",
				c.ID, c.StartLine, lines[startIdx])
		}
	}
}

func TestChunkFile_FallbackSlidingWindow(t *testing.T) {
	// Generate a 120-line unknown-extension file
	var sb strings.Builder
	for i := 0; i < 120; i++ {
		sb.WriteString("line content here\n")
	}
	content := []byte(sb.String())

	chunks, err := ChunkFile("file.unknown", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(chunks) == 0 {
		t.Fatal("expected sliding window chunks for unknown extension")
	}

	// Each chunk should be at most slidingWindowSize lines
	for _, c := range chunks {
		size := c.EndLine - c.StartLine + 1
		if size > slidingWindowSize {
			t.Errorf("chunk %s has %d lines, exceeds sliding window size %d", c.ID, size, slidingWindowSize)
		}
	}

	// Verify overlap: next chunk start should be before previous chunk end
	for i := 1; i < len(chunks); i++ {
		if chunks[i].StartLine >= chunks[i-1].EndLine {
			t.Errorf("no overlap between chunk %d (end %d) and chunk %d (start %d)",
				i-1, chunks[i-1].EndLine, i, chunks[i].StartLine)
		}
	}
}

func TestChunkFile_EmptyFile(t *testing.T) {
	chunks, err := ChunkFile("empty.go", []byte(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty file, got %d", len(chunks))
	}
}

func TestChunkFile_Go_MinChunkSize(t *testing.T) {
	// A file where chunks smaller than minChunkLines should be skipped
	src := `package tiny

func Tiny() {}
`
	chunks, err := ChunkFile("tiny.go", []byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// "func Tiny() {}" is 1 line — should be skipped (< minChunkLines=3)
	// The package declaration is not a declaration we track, so 0 or only
	// chunks >= 3 lines should appear.
	for _, c := range chunks {
		size := c.EndLine - c.StartLine + 1
		if size < minChunkLines {
			t.Errorf("chunk %s has %d lines, below minimum %d", c.ID, size, minChunkLines)
		}
	}
}

func TestChunkFile_ParseErrorDoesNotCrash(t *testing.T) {
	// Invalid Go syntax — ChunkFile must not panic.
	// The line-scanner (findGoDecls) does not invoke go/parser, so this falls through
	// to buildDeclChunks or buildSlidingChunks without panicking.
	src := []byte("func foo( { syntax error\n")
	chunks, err := ChunkFile("bad.go", src)
	// No panic is the primary assertion; error is acceptable too.
	_ = err
	_ = chunks
}

func TestChunkFile_TypesAndVars(t *testing.T) {
	// Each declaration needs at least minChunkLines (3) lines to produce a chunk.
	// var/const blocks use parenthesized form to span multiple lines.
	src := `package example

type Foo struct {
	X int
	Y string
}

var (
	bar = 1
	baz = 2
)

const (
	Alpha = "a"
	Beta  = "b"
)
`
	chunks, err := ChunkFile("typevars.go", []byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// We expect chunks for the type, var, and const declarations.
	// Each declaration starts at column 0 and is picked up by findGoDecls.
	found := map[string]bool{}
	for _, c := range chunks {
		if strings.HasPrefix(c.Symbol, "type ") {
			found["type"] = true
		}
		if strings.HasPrefix(c.Symbol, "var ") {
			found["var"] = true
		}
		if strings.HasPrefix(c.Symbol, "const ") {
			found["const"] = true
		}
	}

	for _, want := range []string{"type", "var", "const"} {
		if !found[want] {
			t.Errorf("expected a chunk with %s declaration, got chunks: %v", want, chunks)
		}
	}
}

func TestChunkFile_Python_MultiSymbol(t *testing.T) {
	// Each top-level declaration must span at least minChunkLines (3) lines.
	// We give each one a multi-line body to ensure it passes the threshold.
	src := `class Alpha:
    def method(self):
        pass

class Beta:
    x = 1
    y = 2

class Gamma:
    a = 3
    b = 4

def func_one():
    x = 1
    return x

def func_two(x, y):
    z = x + y
    return z
`
	chunks, err := ChunkFile("multi.py", []byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expect 5 chunks: 3 classes + 2 functions.
	// Each top-level def/class starts at column 0.
	if len(chunks) < 5 {
		t.Errorf("expected at least 5 chunks for 3 classes + 2 functions, got %d", len(chunks))
		for _, c := range chunks {
			t.Logf("  chunk %s: symbol=%q lines=%d-%d", c.ID, c.Symbol, c.StartLine, c.EndLine)
		}
		return
	}

	names := map[string]bool{
		"class Alpha": false,
		"class Beta":  false,
		"class Gamma": false,
		"def func_one": false,
		"def func_two": false,
	}
	for _, c := range chunks {
		if _, ok := names[c.Symbol]; ok {
			names[c.Symbol] = true
		}
	}
	for sym, found := range names {
		if !found {
			t.Errorf("expected chunk with symbol %q, not found", sym)
		}
	}
}

// TestChunkFile_MinChunkSizeRespected verifies that 1-line functions are excluded.
// This duplicates the intent of TestChunkFile_Go_MinChunkSize with an explicit assertion.
func TestChunkFile_MinChunkSizeRespected(t *testing.T) {
	// current behavior: single-line functions are NOT included (< minChunkLines=3)
	src := `package tiny

func f() {}
`
	chunks, err := ChunkFile("tiny2.go", []byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// "func f() {}" is 1 line — below minChunkLines (3), so should be skipped.
	for _, c := range chunks {
		size := c.EndLine - c.StartLine + 1
		if size < minChunkLines {
			t.Errorf("chunk %s (%q) has %d lines, below minChunkLines=%d",
				c.ID, c.Symbol, size, minChunkLines)
		}
	}
}

func TestChunkDir_ExcludesKnownDirs(t *testing.T) {
	root := t.TempDir()

	writeAt := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	writeAt("main.go", "package main\n\nfunc Main() {}\n")
	writeAt("vendor/dep.go", "package dep\n\nfunc VendorFunc() {}\n")
	writeAt("node_modules/lib.js", "export function libFunc() {}\n")
	writeAt(".git/config", "[core]\n\trepositoryformatversion = 0\n")

	chunks, err := ChunkDir(root)
	if err != nil {
		t.Fatalf("ChunkDir: %v", err)
	}

	for _, c := range chunks {
		if strings.Contains(c.Path, "vendor") {
			t.Errorf("vendor/ file should be excluded, but got chunk: path=%s symbol=%s", c.Path, c.Symbol)
		}
		if strings.Contains(c.Path, "node_modules") {
			t.Errorf("node_modules/ file should be excluded, but got chunk: path=%s symbol=%s", c.Path, c.Symbol)
		}
		if strings.Contains(c.Path, ".git") {
			t.Errorf(".git/ file should be excluded, but got chunk: path=%s symbol=%s", c.Path, c.Symbol)
		}
	}

	// main.go should produce at least one chunk (func Main — 1 line, may be skipped by minChunkLines;
	// but at least no panic and the excluded dirs are absent).
	hasMain := false
	for _, c := range chunks {
		if strings.HasSuffix(c.Path, "main.go") {
			hasMain = true
			break
		}
	}
	// If func Main() {} is only 1 line it will be below minChunkLines=3 and won't produce a chunk.
	// Verify by checking chunk count only if we have chunks.
	if len(chunks) > 0 && !hasMain {
		// All chunks should come from main.go only.
		for _, c := range chunks {
			if !strings.HasSuffix(c.Path, "main.go") {
				t.Errorf("unexpected chunk from %s — expected only main.go", c.Path)
			}
		}
	}
}

func TestChunkFile_GoLargeFunction(t *testing.T) {
	// A function longer than maxChunkLines should be split into sub-chunks
	var sb strings.Builder
	sb.WriteString("package large\n\n")
	sb.WriteString("func BigFunc() {\n")
	for i := 0; i < 250; i++ {
		sb.WriteString("\t_ = 0\n")
	}
	sb.WriteString("}\n")

	chunks, err := ChunkFile("large.go", []byte(sb.String()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have been split into multiple chunks
	if len(chunks) < 2 {
		t.Errorf("expected BigFunc to be split into 2+ chunks, got %d", len(chunks))
	}
	for _, c := range chunks {
		size := c.EndLine - c.StartLine + 1
		if size > maxChunkLines {
			t.Errorf("chunk %s has %d lines, exceeds maxChunkLines %d", c.ID, size, maxChunkLines)
		}
	}
}
