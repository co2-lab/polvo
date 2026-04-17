package repomap

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "update golden files")

// ---------------------------------------------------------------------------
// EstimateTokens
// ---------------------------------------------------------------------------

func TestEstimateTokens(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"abcd", 1},        // 4 chars → 1
		{"abc", 1},         // 3 chars → 1
		{"abcdefgh", 2},    // 8 chars → 2
		{strings.Repeat("x", 100), 25}, // 100 chars → 25
	}
	for _, tc := range cases {
		got := EstimateTokens(tc.input)
		if got != tc.want {
			t.Errorf("EstimateTokens(%q) = %d, want %d", tc.input, got, tc.want)
		}
		// Verify exact formula
		formula := (len(tc.input) + 3) / 4
		if got != formula {
			t.Errorf("EstimateTokens(%q) = %d, but formula gives %d", tc.input, got, formula)
		}
	}
}

// ---------------------------------------------------------------------------
// ExtractSymbols (replaces extractSymbolLine tests)
// ---------------------------------------------------------------------------

func TestExtractSymbols_Go(t *testing.T) {
	src := []byte("package p\nfunc MyFunc() {}\nfunc myPrivate() {}\ntype MyType struct{}\nvar MyVar = 1\nconst MyConst = 2\n")
	syms := ExtractSymbols("foo.go", src)
	want := []string{"MyFunc", "MyType", "MyVar", "MyConst"}
	if len(syms) != len(want) {
		t.Fatalf("want %d symbols, got %d: %+v", len(want), len(syms), syms)
	}
	for i, s := range syms {
		name := firstIdent(s.Signature[len(s.Kind)+1:])
		if name != want[i] {
			t.Errorf("[%d] want %q got %q", i, want[i], name)
		}
	}
}

func TestExtractSymbols_TypeScript(t *testing.T) {
	src := []byte("export function MyFn(x: number) {}\nexport class MyClass {}\nexport interface IFoo {}\nexport const bar = () => {}\nfunction local() {}\n")
	syms := ExtractSymbols("foo.ts", src)
	if len(syms) < 4 {
		t.Fatalf("want at least 4 symbols, got %d: %+v", len(syms), syms)
	}
	// "local" should NOT be in syms
	for _, s := range syms {
		if strings.Contains(s.Signature, "local") {
			t.Errorf("non-exported 'local' should not appear: %+v", s)
		}
	}
}

func TestExtractSymbols_Python(t *testing.T) {
	src := []byte("def my_function(x):\n    pass\nclass MyClass:\n    pass\ndef _private():\n    pass\n")
	syms := ExtractSymbols("foo.py", src)
	if len(syms) != 2 {
		t.Fatalf("want 2 symbols (my_function, MyClass), got %d: %+v", len(syms), syms)
	}
}

// ---------------------------------------------------------------------------
// testLanguageRepoMap helper
// ---------------------------------------------------------------------------

func testLanguageRepoMap(t *testing.T, ext, wantSymbol string, content string) {
	t.Helper()
	dir := t.TempDir()
	filename := "test." + ext
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	rm := New(dir, 2000)
	result, err := rm.Build(context.Background(), nil)
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if !strings.Contains(result, filename) {
		t.Errorf("expected %q in result, got:\n%s", filename, result)
	}
	if !strings.Contains(result, wantSymbol) {
		t.Errorf("expected symbol %q in result, got:\n%s", wantSymbol, result)
	}
}

func TestLanguageGo(t *testing.T) {
	testLanguageRepoMap(t, "go", "Greeter",
		"package p\n\ntype Greeter interface {\n\tGreet() string\n}\n\nfunc NewGreeter() Greeter { return nil }\n")
}

func TestLanguagePython(t *testing.T) {
	testLanguageRepoMap(t, "py", "Person",
		"class Person:\n    def __init__(self, name):\n        self.name = name\n")
}

func TestLanguageTypeScript(t *testing.T) {
	testLanguageRepoMap(t, "ts", "greet",
		"export function greet(name: string): string {\n    return `Hello, ${name}`;\n}\n")
}

func TestLanguageJS(t *testing.T) {
	testLanguageRepoMap(t, "js", "myFunc",
		"export function myFunc() { return 42; }\n")
}

// ---------------------------------------------------------------------------
// isSourceFile
// ---------------------------------------------------------------------------

func TestIsSourceFile(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"main.go", true},
		{"app.ts", true},
		{"comp.tsx", true},
		{"util.js", true},
		{"mod.jsx", true},
		{"script.py", true},
		{"lib.rs", true},
		{"App.java", true},
		{"README.md", false},
		{"config.json", false},
		{"data.yaml", false},
		// Note: filepath.Ext("main.GO") → ".GO" → strings.ToLower → ".go" → matches → true
		// The implementation lowercases the extension so "main.GO" IS recognized as a Go source file.
		{"main.GO", true},
	}
	for _, tc := range cases {
		got := isSourceFile(tc.name)
		if got != tc.want {
			t.Errorf("isSourceFile(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// skipRepoDir
// ---------------------------------------------------------------------------

func TestSkipRepoDir(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"node_modules", true},
		{"vendor", true},
		{"__pycache__", true},
		{".git", true},
		{".hidden", true},
		{"dist", true},
		{"build", true},
		{"target", true},
		{"out", true},
		{"bin", true},
		{"src", false},
		{"internal", false},
		{"cmd", false},
	}
	for _, tc := range cases {
		got := skipRepoDir(tc.name)
		if got != tc.want {
			t.Errorf("skipRepoDir(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// RepoMap.Build — integration tests
// ---------------------------------------------------------------------------

func TestRepomapBuild(t *testing.T) {
	dir := t.TempDir()

	writeFile := func(rel, content string) {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	writeFile("main.go", "package main\nfunc Main() {}\n")
	writeFile("internal/a.go", "package a\nfunc Helper() {}\n")
	writeFile("vendor/x.go", "package x\nfunc Skip() {}\n")
	writeFile(".hidden/secret.go", "package h\nfunc Secret() {}\n")

	t.Run("no focus files", func(t *testing.T) {
		rm := New(dir, 2000)
		result, err := rm.Build(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(result, "# Repository Map") {
			t.Errorf("result should start with '# Repository Map', got: %q", result[:min(50, len(result))])
		}
		if !strings.Contains(result, "Main") {
			t.Errorf("expected 'Main' in result:\n%s", result)
		}
		if !strings.Contains(result, "Helper") {
			t.Errorf("expected 'Helper' in result:\n%s", result)
		}
		if strings.Contains(result, "Skip") {
			t.Errorf("vendor/ should be excluded, but 'Skip' found in result:\n%s", result)
		}
		if strings.Contains(result, "Secret") {
			t.Errorf(".hidden/ should be excluded, but 'Secret' found in result:\n%s", result)
		}
	})

	t.Run("focus files boost", func(t *testing.T) {
		rm := New(dir, 2000)
		// Pass the absolute path so focusSet matches the WalkDir absolute paths
		absA := filepath.Join(dir, "internal", "a.go")
		result, err := rm.Build(context.Background(), []string{absA})
		if err != nil {
			t.Fatal(err)
		}
		// internal/a.go should appear before main.go (higher score)
		idxA := strings.Index(result, "internal/a.go")
		idxMain := strings.Index(result, "main.go")
		if idxA < 0 {
			t.Fatal("internal/a.go not found in result")
		}
		if idxMain < 0 {
			t.Fatal("main.go not found in result")
		}
		if idxA >= idxMain {
			t.Errorf("expected internal/a.go (idx %d) before main.go (idx %d) in result:\n%s", idxA, idxMain, result)
		}
	})

	t.Run("MaxTokens cap", func(t *testing.T) {
		rm := New(dir, 10) // very low budget
		result, err := rm.Build(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		// Header alone costs some tokens; with MaxTokens=10 only header or ≤1 file fits
		// Count lines that match "file: symbols" pattern
		lines := strings.Split(strings.TrimSpace(result), "\n")
		fileLines := 0
		for _, l := range lines {
			if strings.Contains(l, ":") && !strings.HasPrefix(l, "#") {
				fileLines++
			}
		}
		if fileLines > 1 {
			t.Errorf("MaxTokens=10 should limit output, got %d file lines:\n%s", fileLines, result)
		}
	})
}

func TestRepomapBuild_MultiLanguage(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "sample.go"), []byte("package p\nfunc ExportedFunc() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "sample.ts"), []byte("export function tsFunc() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "sample.py"), []byte("def py_func():\n    pass\n"), 0644)

	rm := New(dir, 2000)
	out, err := rm.Build(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, sym := range []string{"ExportedFunc", "tsFunc", "py_func"} {
		if !strings.Contains(out, sym) {
			t.Errorf("expected %q in output:\n%s", sym, out)
		}
	}
}

func TestRepomapBuild_Determinism(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		"main.go":          "package main\nfunc Main() {}\n",
		"api/handler.go":   "package api\nfunc Handle() {}\ntype Request struct{}\n",
		"ui/app.ts":        "export function initApp() {}\nexport class AppComponent {}\n",
		"scripts/build.py": "def build():\n    pass\nclass Builder:\n    pass\n",
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(path), 0755)
		os.WriteFile(path, []byte(content), 0644)
	}

	rm := New(dir, 2000)
	got1, err := rm.Build(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	got2, err := rm.Build(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got1 != got2 {
		t.Errorf("Build is not deterministic:\nfirst:\n%s\nsecond:\n%s", got1, got2)
	}
}

func TestRepomapSampleCodeBase(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"main.go":           "package main\nfunc Main() {}\n",
		"api/handler.go":    "package api\nfunc Handle() {}\ntype Request struct{}\n",
		"ui/app.ts":         "export function initApp() {}\nexport class AppComponent {}\n",
		"scripts/build.py":  "def build():\n    pass\nclass Builder:\n    pass\n",
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(path), 0755)
		os.WriteFile(path, []byte(content), 0644)
	}
	rm := New(dir, 2000)
	got, err := rm.Build(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	golden := "testdata/expected_map.txt"
	if *update {
		os.MkdirAll("testdata", 0755)
		if err := os.WriteFile(golden, []byte(got), 0644); err != nil {
			t.Fatalf("writing golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Skipf("golden file not found — run with -update to create: %v", err)
	}
	if got != string(want) {
		t.Errorf("mismatch (-want +got):\nwant:\n%s\ngot:\n%s", string(want), got)
	}
}

// ---------------------------------------------------------------------------
// Token budget and context exclusion
// ---------------------------------------------------------------------------

func TestBuild_TokenBudgetRespected(t *testing.T) {
	dir := t.TempDir()

	// Write several files with symbols so the unconstrained map would exceed 50 tokens.
	for _, name := range []string{"a.go", "b.go", "c.go", "d.go"} {
		content := "package p\nfunc " + name[:1] + "Func() {}\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	const budget = 50
	rm := New(dir, budget)
	result, err := rm.Build(context.Background(), nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	tokens := EstimateTokens(result)
	if tokens > budget {
		t.Errorf("EstimateTokens(result)=%d exceeds MaxTokens=%d; output:\n%s", tokens, budget, result)
	}
}

func TestBuild_ExcludesFilesAlreadyInContext(t *testing.T) {
	dir := t.TempDir()

	absExcluded := filepath.Join(dir, "excluded.go")
	absKept := filepath.Join(dir, "kept.go")

	if err := os.WriteFile(absExcluded, []byte("package p\nfunc ExcludedSymbol() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absKept, []byte("package p\nfunc KeptSymbol() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	rm := New(dir, 2000)
	rm.ExcludeFiles = []string{absExcluded}

	result, err := rm.Build(context.Background(), nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if strings.Contains(result, "ExcludedSymbol") {
		t.Errorf("ExcludedSymbol should not appear in repo map when file is in ExcludeFiles:\n%s", result)
	}
	if !strings.Contains(result, "KeptSymbol") {
		t.Errorf("KeptSymbol should appear in repo map:\n%s", result)
	}
}

// GAP: repomap usa line-scanner simples, não tree-sitter + PageRank real.
// O campo Refresh existe na struct mas é inerte — sem cache implementado.
// Estes testes cobrem o comportamento real implementado, não o design do draft.

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
