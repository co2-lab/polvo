package repomap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractGoSymbols(t *testing.T) {
	src := []byte(`package foo

func ExportedFunc(x int) string { return "" }
func unexportedFunc() {}
type ExportedType struct{}
type unexportedType struct{}
var ExportedVar = 1
const ExportedConst = "x"
`)
	syms := ExtractSymbols("foo.go", src)
	want := map[string]string{
		"ExportedFunc":  "func",
		"ExportedType":  "type",
		"ExportedVar":   "var",
		"ExportedConst": "const",
	}
	if len(syms) != len(want) {
		t.Fatalf("want %d symbols, got %d: %+v", len(want), len(syms), syms)
	}
	for _, s := range syms {
		name := firstIdent(s.Signature[len(s.Kind)+1:])
		if want[name] != s.Kind {
			t.Errorf("symbol %q: want kind %q, got %q", name, want[name], s.Kind)
		}
	}
}

func TestExtractGoSymbols_LineNumbers(t *testing.T) {
	src := []byte(`package foo

func Foo() {}
type Bar struct{}
`)
	syms := ExtractSymbols("foo.go", src)
	if len(syms) != 2 {
		t.Fatalf("want 2 symbols, got %d", len(syms))
	}
	if syms[0].Line != 3 {
		t.Errorf("Foo: want line 3, got %d", syms[0].Line)
	}
	if syms[1].Line != 4 {
		t.Errorf("Bar: want line 4, got %d", syms[1].Line)
	}
}

func TestExtractJSSymbols(t *testing.T) {
	src := []byte(`import foo from 'bar'

export function MyFunc(x: number): string { return '' }
export class MyClass {}
export interface MyInterface {}
export type MyType = string
export const MyConst = () => {}
function notExported() {}
`)
	syms := ExtractSymbols("foo.ts", src)
	if len(syms) < 5 {
		t.Fatalf("want at least 5 symbols, got %d: %+v", len(syms), syms)
	}
}

func TestExtractPySymbols(t *testing.T) {
	src := []byte(`def public_func(x):
    pass

async def public_async(x):
    pass

def _private(x):
    pass

class MyClass:
    pass
`)
	syms := ExtractSymbols("foo.py", src)
	if len(syms) != 3 {
		t.Fatalf("want 3 symbols (public_func, public_async, MyClass), got %d: %+v", len(syms), syms)
	}
}

func TestExtractSymbols_Unsupported(t *testing.T) {
	src := []byte(`some content`)
	syms := ExtractSymbols("file.rs", src)
	if syms != nil {
		t.Errorf("expected nil for unsupported ext, got %+v", syms)
	}
}

func TestSidecarRoundtrip(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "foo.go")
	src := []byte(`package foo

func Exported(x int) string { return "" }
type MyType struct{}
`)
	if err := os.WriteFile(srcPath, src, 0o644); err != nil {
		t.Fatal(err)
	}

	hash := fileHash(src)
	syms := ExtractSymbols(srcPath, src)
	if len(syms) == 0 {
		t.Fatal("expected symbols")
	}

	if err := writeSidecar(srcPath, hash, syms); err != nil {
		t.Fatalf("writeSidecar: %v", err)
	}

	loaded, err := loadSidecar(srcPath)
	if err != nil {
		t.Fatalf("loadSidecar: %v", err)
	}
	if len(loaded) != len(syms) {
		t.Fatalf("want %d symbols, got %d", len(syms), len(loaded))
	}
	for i, s := range syms {
		if loaded[i].Line != s.Line || loaded[i].Kind != s.Kind || loaded[i].Signature != s.Signature {
			t.Errorf("symbol %d mismatch: want %+v, got %+v", i, s, loaded[i])
		}
	}
}

func TestLoadSidecar_StaleHash(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "foo.go")

	// Write initial source and sidecar
	src := []byte(`package foo

func Foo() {}
`)
	if err := os.WriteFile(srcPath, src, 0o644); err != nil {
		t.Fatal(err)
	}
	hash := fileHash(src)
	syms := ExtractSymbols(srcPath, src)
	if err := writeSidecar(srcPath, hash, syms); err != nil {
		t.Fatal(err)
	}

	// Modify source without updating sidecar
	if err := os.WriteFile(srcPath, []byte(`package foo

func Foo() {}
func Bar() {}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := loadSidecar(srcPath)
	if err == nil {
		t.Error("expected error for stale sidecar, got nil")
	}
}

func TestLoadSidecar_Missing(t *testing.T) {
	_, err := loadSidecar("/nonexistent/path/foo.go")
	if err == nil {
		t.Error("expected error for missing sidecar, got nil")
	}
}
