package repomap

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
)

// RichSymbol represents a symbol extracted from source with def/ref distinction.
// The existing Symbol type is kept for backward compat; this is used by the
// PageRank pipeline.
type RichSymbol struct {
	Name     string // e.g. "ServeHTTP"
	Kind     string // "function" | "method" | "type" | "interface" | "const" | "var" | "class" | "ref"
	IsDef    bool   // true = declaration; false = reference/call
	Line     int    // 1-based
	FilePath string // relative to repo root
}

// ExtractRichSymbols dispatches to the language-specific extractor.
// Returns an empty slice (not nil) on unknown file type or parse error.
func ExtractRichSymbols(path string, src []byte) []RichSymbol {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		syms, _ := extractGoAST(path, src)
		return syms
	case ".ts", ".tsx":
		return extractTSSymbols(path, src)
	case ".js", ".jsx":
		return extractJSRichSymbols(path, src)
	case ".py":
		return extractPyRichSymbols(path, src)
	}
	return nil
}

// extractGoAST uses go/ast to extract definitions and references from Go source.
func extractGoAST(path string, src []byte) ([]RichSymbol, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		// Return what we got (partial AST on error).
		if f == nil {
			return nil, err
		}
	}

	var syms []RichSymbol

	// Walk top-level declarations for definitions.
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name == nil {
				continue
			}
			kind := "function"
			if d.Recv != nil && len(d.Recv.List) > 0 {
				kind = "method"
			}
			syms = append(syms, RichSymbol{
				Name:     d.Name.Name,
				Kind:     kind,
				IsDef:    true,
				Line:     fset.Position(d.Name.Pos()).Line,
				FilePath: path,
			})

		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if s.Name == nil {
						continue
					}
					kind := "type"
					if _, ok := s.Type.(*ast.InterfaceType); ok {
						kind = "interface"
					}
					syms = append(syms, RichSymbol{
						Name:     s.Name.Name,
						Kind:     kind,
						IsDef:    true,
						Line:     fset.Position(s.Name.Pos()).Line,
						FilePath: path,
					})
				case *ast.ValueSpec:
					kind := "var"
					if d.Tok == token.CONST {
						kind = "const"
					}
					for _, name := range s.Names {
						if name == nil {
							continue
						}
						syms = append(syms, RichSymbol{
							Name:     name.Name,
							Kind:     kind,
							IsDef:    true,
							Line:     fset.Position(name.Pos()).Line,
							FilePath: path,
						})
					}
				}
			}
		}
	}

	// Walk entire file for references (call expressions and selector expressions).
	ast.Inspect(f, func(n ast.Node) bool {
		switch expr := n.(type) {
		case *ast.CallExpr:
			// Direct call: foo()
			if ident, ok := expr.Fun.(*ast.Ident); ok {
				syms = append(syms, RichSymbol{
					Name:     ident.Name,
					Kind:     "ref",
					IsDef:    false,
					Line:     fset.Position(ident.Pos()).Line,
					FilePath: path,
				})
			}
			// Selector call: pkg.Foo()
			if sel, ok := expr.Fun.(*ast.SelectorExpr); ok {
				syms = append(syms, RichSymbol{
					Name:     sel.Sel.Name,
					Kind:     "ref",
					IsDef:    false,
					Line:     fset.Position(sel.Sel.Pos()).Line,
					FilePath: path,
				})
			}
		case *ast.SelectorExpr:
			// Selector access: x.Field (non-call)
			syms = append(syms, RichSymbol{
				Name:     expr.Sel.Name,
				Kind:     "ref",
				IsDef:    false,
				Line:     fset.Position(expr.Sel.Pos()).Line,
				FilePath: path,
			})
		}
		return true
	})

	return syms, nil
}

// TS/JS regex patterns
var (
	reTSExportFunc  = regexp.MustCompile(`^export\s+(?:async\s+)?function\s+(\w+)`)
	reTSExportClass = regexp.MustCompile(`^export\s+(?:abstract\s+)?class\s+(\w+)`)
	reTSTypeAlias   = regexp.MustCompile(`^export\s+type\s+(\w+)\s*=`)
	reTSInterface   = regexp.MustCompile(`^export\s+interface\s+(\w+)`)
	reTSArrowConst  = regexp.MustCompile(`^export\s+(?:const|let)\s+(\w+)\s*(?::[^=]+)?\s*=\s*(?:async\s+)?(?:function|\()`)
	reTSImportRef   = regexp.MustCompile(`import\s*\{([^}]+)\}\s+from`)
)

func extractTSSymbols(path string, src []byte) []RichSymbol {
	lines := strings.Split(string(src), "\n")
	var syms []RichSymbol

	for i, line := range lines {
		lineNo := i + 1
		if m := reTSExportFunc.FindStringSubmatch(line); m != nil {
			syms = append(syms, RichSymbol{Name: m[1], Kind: "function", IsDef: true, Line: lineNo, FilePath: path})
			continue
		}
		if m := reTSExportClass.FindStringSubmatch(line); m != nil {
			syms = append(syms, RichSymbol{Name: m[1], Kind: "class", IsDef: true, Line: lineNo, FilePath: path})
			continue
		}
		if m := reTSTypeAlias.FindStringSubmatch(line); m != nil {
			syms = append(syms, RichSymbol{Name: m[1], Kind: "type", IsDef: true, Line: lineNo, FilePath: path})
			continue
		}
		if m := reTSInterface.FindStringSubmatch(line); m != nil {
			syms = append(syms, RichSymbol{Name: m[1], Kind: "interface", IsDef: true, Line: lineNo, FilePath: path})
			continue
		}
		if m := reTSArrowConst.FindStringSubmatch(line); m != nil {
			syms = append(syms, RichSymbol{Name: m[1], Kind: "function", IsDef: true, Line: lineNo, FilePath: path})
			continue
		}
		// Import references create cross-file edges.
		if m := reTSImportRef.FindStringSubmatch(line); m != nil {
			for _, name := range strings.Split(m[1], ",") {
				name = strings.TrimSpace(name)
				if name != "" {
					syms = append(syms, RichSymbol{Name: name, Kind: "ref", IsDef: false, Line: lineNo, FilePath: path})
				}
			}
		}
	}
	return syms
}

// JS uses the same regexes as TS (subset of patterns).
func extractJSRichSymbols(path string, src []byte) []RichSymbol {
	return extractTSSymbols(path, src)
}

// Python patterns
var (
	rePyClassDef   = regexp.MustCompile(`^class\s+(\w+)`)
	rePyTopDef     = regexp.MustCompile(`^def\s+(\w+)`)
	rePyTopAsync   = regexp.MustCompile(`^async\s+def\s+(\w+)`)
	rePyMethodDef  = regexp.MustCompile(`^\s{4}def\s+(\w+)`)
	rePyDecorator  = regexp.MustCompile(`^@(\w+)`)
)

func extractPyRichSymbols(path string, src []byte) []RichSymbol {
	lines := strings.Split(string(src), "\n")
	var syms []RichSymbol

	for i, line := range lines {
		lineNo := i + 1
		if m := rePyClassDef.FindStringSubmatch(line); m != nil {
			syms = append(syms, RichSymbol{Name: m[1], Kind: "class", IsDef: true, Line: lineNo, FilePath: path})
			continue
		}
		if m := rePyTopAsync.FindStringSubmatch(line); m != nil {
			if !strings.HasPrefix(m[1], "_") {
				syms = append(syms, RichSymbol{Name: m[1], Kind: "function", IsDef: true, Line: lineNo, FilePath: path})
			}
			continue
		}
		if m := rePyTopDef.FindStringSubmatch(line); m != nil {
			if !strings.HasPrefix(m[1], "_") {
				syms = append(syms, RichSymbol{Name: m[1], Kind: "function", IsDef: true, Line: lineNo, FilePath: path})
			}
			continue
		}
		if m := rePyMethodDef.FindStringSubmatch(line); m != nil {
			if !strings.HasPrefix(m[1], "_") {
				syms = append(syms, RichSymbol{Name: m[1], Kind: "method", IsDef: true, Line: lineNo, FilePath: path})
			}
			continue
		}
		if m := rePyDecorator.FindStringSubmatch(line); m != nil {
			syms = append(syms, RichSymbol{Name: m[1], Kind: "ref", IsDef: false, Line: lineNo, FilePath: path})
		}
	}
	return syms
}
