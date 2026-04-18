package repomap

import (
	"math"
	"strings"
)

// graph is a weighted directed multigraph where nodes are file paths and
// edges represent "file A references a symbol defined in file B".
type graph struct {
	nodes     map[string]struct{}
	edges     map[string]map[string]float64 // src → dst → total weight
	outWeight map[string]float64            // sum of outgoing edge weights per node
}

func newGraph() *graph {
	return &graph{
		nodes:     make(map[string]struct{}),
		edges:     make(map[string]map[string]float64),
		outWeight: make(map[string]float64),
	}
}

func (g *graph) addNode(path string) {
	g.nodes[path] = struct{}{}
	if _, ok := g.edges[path]; !ok {
		g.edges[path] = make(map[string]float64)
	}
}

func (g *graph) addEdge(src, dst string, weight float64) {
	if src == dst {
		return
	}
	g.addNode(src)
	g.addNode(dst)
	g.edges[src][dst] += weight
	g.outWeight[src] += weight
}

// buildGraph constructs the reference graph from per-file rich symbols.
//
//   - fileSyms: map from relative file path → []RichSymbol
//   - chatFiles: set of files currently in the chat context (personalization seeds)
//   - mentionedIdents: set of identifier names mentioned in the chat
//
// Returns the graph and a personalization vector (unnormalised).
func buildGraph(
	fileSyms map[string][]RichSymbol,
	chatFiles map[string]bool,
	mentionedIdents map[string]bool,
) (*graph, map[string]float64) {
	g := newGraph()

	// Register all files as nodes.
	for path := range fileSyms {
		g.addNode(path)
	}

	// Build a reverse lookup: name → set of files that define it.
	defines := make(map[string]map[string]int) // name → {defFile → count}
	for path, syms := range fileSyms {
		for _, s := range syms {
			if !s.IsDef {
				continue
			}
			if defines[s.Name] == nil {
				defines[s.Name] = make(map[string]int)
			}
			defines[s.Name][path]++
		}
	}

	// For each file, for each reference, add edges to defining files.
	for refFile, syms := range fileSyms {
		// Count how many times each name is referenced from this file.
		refCounts := make(map[string]int)
		for _, s := range syms {
			if !s.IsDef {
				refCounts[s.Name]++
			}
		}

		for name, count := range refCounts {
			defFiles, ok := defines[name]
			if !ok {
				continue
			}
			// Base weight = sqrt(callCount) mirroring Aider.
			w := math.Sqrt(float64(count))

			// camelCase/snake_case bonus for long, descriptive names.
			if isMeaningfulIdent(name) {
				w *= 10
			}
			// Private penalty.
			if strings.HasPrefix(name, "_") {
				w *= 0.1
			}
			// Over-defined penalty (generic names like "Error", "String").
			if len(defFiles) > 5 {
				w *= 0.1
			}
			// Chat bonus: mentioned name gets ×10.
			if mentionedIdents[name] {
				w *= 10
			}
			// Caller-is-chat: ×50 if the ref file itself is in chat.
			if chatFiles[refFile] {
				w *= 50
			}

			for defFile := range defFiles {
				g.addEdge(refFile, defFile, w)
			}
		}
	}

	// Personalization: chat files + files whose basename matches a mentioned ident.
	pers := make(map[string]float64)
	n := float64(len(g.nodes))
	if n == 0 {
		return g, pers
	}
	seed := 100.0 / n
	for path := range g.nodes {
		if chatFiles[path] {
			pers[path] += seed
			continue
		}
		base := strings.TrimSuffix(fileBase(path), fileExt(path))
		if mentionedIdents[base] {
			pers[path] += seed
		}
	}

	return g, pers
}

// distributeRank distributes each file's rank proportionally to its outgoing
// edges, returning a score per (file, symbolName) pair.
// Key: [2]string{filePath, symbolName}.
func distributeRank(g *graph, rank map[string]float64, fileSyms map[string][]RichSymbol) map[[2]string]float64 {
	scores := make(map[[2]string]float64)

	for src, dsts := range g.edges {
		srcRank := rank[src]
		if srcRank == 0 || g.outWeight[src] == 0 {
			continue
		}
		for dst, w := range dsts {
			contrib := srcRank * w / g.outWeight[src]
			// Distribute to all def symbols in dst file.
			for _, s := range fileSyms[dst] {
				if s.IsDef {
					key := [2]string{dst, s.Name}
					scores[key] += contrib
				}
			}
		}
	}

	// Also give every def symbol its own file's rank as a baseline.
	for path, syms := range fileSyms {
		fr := rank[path]
		for _, s := range syms {
			if s.IsDef {
				key := [2]string{path, s.Name}
				scores[key] += fr
			}
		}
	}

	return scores
}

// isMeaningfulIdent returns true for camelCase or snake_case names ≥ 8 chars.
func isMeaningfulIdent(name string) bool {
	if len(name) < 8 {
		return false
	}
	hasCamel := false
	hasUnderscore := strings.Contains(name, "_")
	for i := 1; i < len(name); i++ {
		if name[i] >= 'A' && name[i] <= 'Z' {
			hasCamel = true
			break
		}
	}
	return hasCamel || hasUnderscore
}

func fileBase(path string) string {
	idx := strings.LastIndexAny(path, "/\\")
	if idx < 0 {
		return path
	}
	return path[idx+1:]
}

func fileExt(path string) string {
	base := fileBase(path)
	idx := strings.LastIndex(base, ".")
	if idx < 0 {
		return ""
	}
	return base[idx:]
}
