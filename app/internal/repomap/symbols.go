package repomap

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Symbol represents an exported symbol extracted from a source file.
type Symbol struct {
	Line      int    // 1-based line number
	Kind      string // "func" | "type" | "var" | "const" | "interface" | "class" | "method"
	Signature string // full signature line
}

// ExtractSymbols extracts exported symbols from content by file type.
// Supports Go, TypeScript/JavaScript, Python. Returns nil for other types.
func ExtractSymbols(path string, content []byte) []Symbol {
	ext := strings.ToLower(filepath.Ext(path))
	lines := splitLines(content)
	switch ext {
	case ".go":
		return extractGoSymbols(lines)
	case ".ts", ".tsx", ".js", ".jsx":
		return extractJSSymbols(lines)
	case ".py":
		return extractPySymbols(lines)
	}
	return nil
}

func extractGoSymbols(lines []string) []Symbol {
	var syms []Symbol
	for i, line := range lines {
		if !isTopLevel(line) {
			continue
		}
		kind, sig := parseGoLine(line)
		if kind != "" && sig != "" && isExportedSig(sig) {
			syms = append(syms, Symbol{Line: i + 1, Kind: kind, Signature: strings.TrimSpace(line)})
		}
	}
	return syms
}

func parseGoLine(line string) (kind, sig string) {
	for _, k := range []string{"func", "type", "var", "const"} {
		if strings.HasPrefix(line, k+" ") {
			rest := strings.TrimPrefix(line, k+" ")
			name := firstIdent(rest)
			if name != "" {
				return k, name
			}
		}
	}
	return "", ""
}

func isExportedSig(name string) bool {
	return len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z'
}

func extractJSSymbols(lines []string) []Symbol {
	var syms []Symbol
	exportPrefixes := []struct {
		prefix string
		kind   string
	}{
		{"export async function ", "func"},
		{"export function ", "func"},
		{"export default async function ", "func"},
		{"export default function ", "func"},
		{"export class ", "class"},
		{"export interface ", "interface"},
		{"export type ", "type"},
		{"export enum ", "type"},
		{"export const ", "func"}, // arrow functions
	}
	for i, line := range lines {
		if !isTopLevel(line) {
			continue
		}
		for _, ep := range exportPrefixes {
			if strings.HasPrefix(line, ep.prefix) {
				rest := strings.TrimPrefix(line, ep.prefix)
				name := firstIdent(rest)
				if name != "" {
					sig := strings.TrimSpace(line)
					if len(sig) > 120 {
						sig = sig[:120] + "…"
					}
					syms = append(syms, Symbol{Line: i + 1, Kind: ep.kind, Signature: sig})
					break
				}
			}
		}
	}
	return syms
}

func extractPySymbols(lines []string) []Symbol {
	var syms []Symbol
	for i, line := range lines {
		if !isTopLevel(line) {
			continue
		}
		var kind string
		var rest string
		switch {
		case strings.HasPrefix(line, "async def "):
			kind = "func"
			rest = strings.TrimPrefix(line, "async def ")
		case strings.HasPrefix(line, "def "):
			kind = "func"
			rest = strings.TrimPrefix(line, "def ")
		case strings.HasPrefix(line, "class "):
			kind = "class"
			rest = strings.TrimPrefix(line, "class ")
		default:
			continue
		}
		name := firstIdent(rest)
		// skip private (leading underscore)
		if name == "" || strings.HasPrefix(name, "_") {
			continue
		}
		sig := strings.TrimSpace(line)
		if len(sig) > 120 {
			sig = sig[:120] + "…"
		}
		syms = append(syms, Symbol{Line: i + 1, Kind: kind, Signature: sig})
	}
	return syms
}

// writeSidecar writes a .symbols sidecar file next to path.
// Format: first line = hash, subsequent lines = <line>\t<kind>\t<signature>
func writeSidecar(path, hash string, symbols []Symbol) error {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s\n", hash)
	for _, s := range symbols {
		fmt.Fprintf(&sb, "%d\t%s\t%s\n", s.Line, s.Kind, s.Signature)
	}
	return os.WriteFile(sidecarPath(path), []byte(sb.String()), 0o644)
}

// loadSidecar reads and validates a .symbols sidecar for path.
// Returns an error if the sidecar is absent, malformed, or stale (hash mismatch).
func loadSidecar(path string) ([]Symbol, error) {
	data, err := os.ReadFile(sidecarPath(path))
	if err != nil {
		return nil, err
	}
	parts := strings.SplitN(string(data), "\n", 2)
	if len(parts) < 2 {
		return nil, errors.New("invalid sidecar: missing hash line")
	}
	storedHash := strings.TrimSpace(parts[0])

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading source for hash check: %w", err)
	}
	if storedHash != fileHash(content) {
		return nil, errors.New("sidecar stale")
	}
	return parseSidecar(parts[1]), nil
}

// parseSidecar parses the body (everything after the hash line) of a sidecar file.
func parseSidecar(body string) []Symbol {
	var syms []Symbol
	scanner := bufio.NewScanner(bytes.NewBufferString(body))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		lineNum, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		syms = append(syms, Symbol{
			Line:      lineNum,
			Kind:      parts[1],
			Signature: parts[2],
		})
	}
	return syms
}

// sidecarPath returns the .symbols path for a given source file path.
func sidecarPath(path string) string {
	return path + ".symbols"
}
