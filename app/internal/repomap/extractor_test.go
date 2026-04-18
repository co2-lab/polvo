package repomap

import (
	"testing"
)

func defsOf(syms []RichSymbol) []RichSymbol {
	var out []RichSymbol
	for _, s := range syms {
		if s.IsDef {
			out = append(out, s)
		}
	}
	return out
}

func refsOf(syms []RichSymbol) []RichSymbol {
	var out []RichSymbol
	for _, s := range syms {
		if !s.IsDef {
			out = append(out, s)
		}
	}
	return out
}

func hasName(syms []RichSymbol, name, kind string) bool {
	for _, s := range syms {
		if s.Name == name && (kind == "" || s.Kind == kind) {
			return true
		}
	}
	return false
}

func TestGoExtractor_ExportedFuncAndMethod(t *testing.T) {
	src := `package p
func Foo() {}
func (r *T) Bar() string { return "" }
`
	syms := ExtractRichSymbols("foo.go", []byte(src))
	defs := defsOf(syms)
	if !hasName(defs, "Foo", "function") {
		t.Error("missing Foo(function)")
	}
	if !hasName(defs, "Bar", "method") {
		t.Error("missing Bar(method)")
	}
}

func TestGoExtractor_Generics(t *testing.T) {
	src := `package p
func Map[T any, R any](in []T, fn func(T) R) []R { return nil }
`
	syms := ExtractRichSymbols("foo.go", []byte(src))
	defs := defsOf(syms)
	if !hasName(defs, "Map", "function") {
		t.Error("missing Map(function)")
	}
	// Type params T and R should NOT appear as defs.
	for _, s := range defs {
		if s.Name == "T" || s.Name == "R" {
			t.Errorf("type param %q should not appear as def", s.Name)
		}
	}
}

func TestGoExtractor_InterfaceMethod(t *testing.T) {
	src := `package p
type Writer interface {
	Write(p []byte) (n int, err error)
}
`
	syms := ExtractRichSymbols("foo.go", []byte(src))
	defs := defsOf(syms)
	if !hasName(defs, "Writer", "interface") {
		t.Error("missing Writer(interface)")
	}
}

func TestGoExtractor_ConstBlock(t *testing.T) {
	src := `package p
const (
	Alpha = 1
	Beta  = 2
)
`
	syms := ExtractRichSymbols("foo.go", []byte(src))
	defs := defsOf(syms)
	if !hasName(defs, "Alpha", "const") {
		t.Error("missing Alpha(const)")
	}
	if !hasName(defs, "Beta", "const") {
		t.Error("missing Beta(const)")
	}
}

func TestGoExtractor_VarBlock(t *testing.T) {
	src := `package p
var (
	ErrNotFound = errors.New("not found")
	ErrTimeout  = errors.New("timeout")
)
`
	syms := ExtractRichSymbols("foo.go", []byte(src))
	defs := defsOf(syms)
	if !hasName(defs, "ErrNotFound", "var") {
		t.Error("missing ErrNotFound(var)")
	}
	if !hasName(defs, "ErrTimeout", "var") {
		t.Error("missing ErrTimeout(var)")
	}
}

func TestGoExtractor_References(t *testing.T) {
	src := `package p
func main() {
	bar()
	baz.Qux()
}
`
	syms := ExtractRichSymbols("foo.go", []byte(src))
	refs := refsOf(syms)
	if !hasName(refs, "bar", "") {
		t.Error("missing ref to bar")
	}
	if !hasName(refs, "Qux", "") {
		t.Error("missing ref to Qux")
	}
}

func TestTSExtractor_AsyncArrow(t *testing.T) {
	src := `export const fetchUser = async (id: string) => { return null; };`
	syms := ExtractRichSymbols("foo.ts", []byte(src))
	defs := defsOf(syms)
	if !hasName(defs, "fetchUser", "function") {
		t.Errorf("missing fetchUser(function); got %v", defs)
	}
}

func TestTSExtractor_TypeAlias(t *testing.T) {
	src := `export type UserId = string;`
	syms := ExtractRichSymbols("foo.ts", []byte(src))
	defs := defsOf(syms)
	if !hasName(defs, "UserId", "type") {
		t.Errorf("missing UserId(type); got %v", defs)
	}
}

func TestTSExtractor_ImportRefs(t *testing.T) {
	src := `import { createUser, UserRole } from "./user";`
	syms := ExtractRichSymbols("foo.ts", []byte(src))
	refs := refsOf(syms)
	if !hasName(refs, "createUser", "") {
		t.Error("missing ref to createUser")
	}
	if !hasName(refs, "UserRole", "") {
		t.Error("missing ref to UserRole")
	}
}

func TestPYExtractor_ClassMethod(t *testing.T) {
	src := `class Greeter:
    def greet(self, name):
        return "hello " + name
`
	syms := ExtractRichSymbols("foo.py", []byte(src))
	defs := defsOf(syms)
	if !hasName(defs, "Greeter", "class") {
		t.Error("missing Greeter(class)")
	}
	if !hasName(defs, "greet", "method") {
		t.Error("missing greet(method)")
	}
}

func TestExtractor_UnknownExtFallback(t *testing.T) {
	// Should not panic, may return nil.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panic on unknown extension: %v", r)
		}
	}()
	_ = ExtractRichSymbols("test.xyz", []byte("some content"))
}

func TestExtractor_ParseErrorGraceful(t *testing.T) {
	// Malformed Go: should not panic.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panic on malformed Go: %v", r)
		}
	}()
	src := `func ( {`
	_ = ExtractRichSymbols("bad.go", []byte(src))
}
