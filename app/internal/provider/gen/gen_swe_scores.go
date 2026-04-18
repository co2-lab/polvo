//go:build ignore

// gen_swe_scores.go fetches model benchmark scores from vals.ai and writes
// internal/provider/swe_scores_gen.go with maps for:
//
//   - sweScores — SWE-bench Verified (software engineering tasks)
//   - lcbScores — LiveCodeBench (competitive coding)
//   - ioiScores — IOI (International Olympiad in Informatics)
//
// Usage:
//
//	go run ./internal/provider/gen/gen_swe_scores.go
//
// Invoked automatically by `go generate` before builds.
// If the fetch fails the maps are written empty (scores return 0 = unknown).
package main

import (
	"bytes"
	"cmp"
	"fmt"
	"go/format"
	"io"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strconv"
	"time"
)

func main() {
	out, err := os.Create("swe_scores_gen.go")
	if err != nil {
		fatalf("create: %v", err)
	}
	defer out.Close()

	scores := fetchScores()
	if err := render(out, scores); err != nil {
		fatalf("render: %v", err)
	}
	for name, m := range scores {
		fmt.Printf("  %s: %d models\n", name, len(m))
	}
	fmt.Println("wrote swe_scores_gen.go")
}

// benchmarks to extract from per-model data.
var benchmarks = []struct {
	key  string // key in per-model JS object
	name string // Go variable name
	desc string // comment description
}{
	{"swebench", "sweScores", "SWE-bench Verified (software engineering tasks)"},
	{"lcb", "lcbScores", "LiveCodeBench (competitive coding)"},
	{"ioi", "ioiScores", "IOI — International Olympiad in Informatics (algorithmic coding)"},
}

func fetchScores() map[string]map[string]float64 {
	result := make(map[string]map[string]float64)
	for _, b := range benchmarks {
		result[b.name] = make(map[string]float64)
	}

	client := &http.Client{Timeout: 15 * time.Second}

	page, err := get(client, "https://www.vals.ai/benchmarks/swebench")
	if err != nil {
		fmt.Fprintln(os.Stderr, "fetch page failed:", err)
		return result
	}

	chunkURL := findConstantsChunk(client, page)
	if chunkURL == "" {
		fmt.Fprintln(os.Stderr, "constants chunk not found")
		return result
	}

	js, err := get(client, "https://www.vals.ai"+chunkURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fetch constants failed:", err)
		return result
	}

	extractPerModelScores(js, result)
	return result
}

// extractPerModelScores scans the JS for per-model objects of the form:
//
//	"provider/model":{..., swebench:{ranking:N,accuracy:N,...}, lcb:{...}, ...}
//
// and populates result[varName][modelID] = accuracy for each benchmark.
var (
	modelEntryRe = regexp.MustCompile(`"([a-zA-Z][^"]{2,60}/[^"]{2,60})":\{`)
	benchScoreRe = regexp.MustCompile(`\b(%s):\{[^}]*?accuracy:([0-9]+\.[0-9]+)`)
)

func extractPerModelScores(js []byte, result map[string]map[string]float64) {
	// Build a combined regex for all benchmark keys.
	keys := make([]string, len(benchmarks))
	for i, b := range benchmarks {
		keys[i] = b.key
	}
	// Build per-benchmark regexes. Each benchmark block is a flat object
	// with no nested braces: benchkey:{ranking:N,accuracy:N,...}
	// Using separate regexes avoids confusion between similarly-named keys (e.g. lcb vs swebench).
	benchRe := make([]*regexp.Regexp, len(benchmarks))
	for i, b := range benchmarks {
		benchRe[i] = regexp.MustCompile(`\b` + b.key + `:\{[^{}]*?accuracy:([0-9]+\.[0-9]+)[^{}]*?\}`)
	}

	// nextModelRe matches the start of the next model entry to bound the chunk.
	nextModelRe := regexp.MustCompile(`},?"[a-zA-Z][^"]{2,60}/[^"]{2,60}":\{`)

	allMatches := modelEntryRe.FindAllSubmatchIndex(js, -1)
	for _, m := range allMatches {
		modelID := string(js[m[2]:m[3]])
		start := m[1]
		end := start + 6000
		if end > len(js) {
			end = len(js)
		}
		chunk := js[start:end]
		// Trim at the next model boundary within this chunk.
		if next := nextModelRe.FindIndex(chunk); next != nil {
			chunk = chunk[:next[0]+1]
		}

		for j, b := range benchmarks {
			sm := benchRe[j].FindSubmatch(chunk)
			if sm == nil {
				continue
			}
			acc, err := strconv.ParseFloat(string(sm[1]), 64)
			if err != nil {
				continue
			}
			bm := result[b.name]
			if existing, ok := bm[modelID]; !ok || acc > existing {
				bm[modelID] = acc
			}
		}
	}
}

// findConstantsChunk discovers /_astro/constants.*.js via BenchmarkView.*.js.
var (
	benchmarkViewRe = regexp.MustCompile(`_astro/(BenchmarkView\.[A-Za-z0-9_-]+\.js)`)
	constantsRe     = regexp.MustCompile(`constants\.([A-Za-z0-9_-]+\.js)`)
)

func findConstantsChunk(c *http.Client, page []byte) string {
	m := benchmarkViewRe.FindSubmatch(page)
	if m == nil {
		return ""
	}
	bv, err := get(c, "https://www.vals.ai/_astro/"+string(m[1]))
	if err != nil {
		return ""
	}
	cm := constantsRe.Find(bv)
	if cm == nil {
		return ""
	}
	return "/_astro/" + string(cm)
}

func get(c *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; polvo-gen/1.0)")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

type entry struct {
	key   string
	score float64
}

func render(w io.Writer, scores map[string]map[string]float64) error {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "// Code generated by internal/provider/gen/gen_swe_scores.go — DO NOT EDIT.\n")
	fmt.Fprintf(&buf, "// Source: vals.ai — Updated: %s\n\n", time.Now().UTC().Format("2006-01-02"))
	fmt.Fprintf(&buf, "package provider\n\n")

	for _, b := range benchmarks {
		m := scores[b.name]
		entries := make([]entry, 0, len(m))
		for k, v := range m {
			entries = append(entries, entry{k, v})
		}
		slices.SortFunc(entries, func(a, b entry) int { return cmp.Compare(a.key, b.key) })

		fmt.Fprintf(&buf, "// %s maps model IDs (provider/model) to their %s score (0–100).\n", b.name, b.desc)
		fmt.Fprintf(&buf, "// Source: vals.ai. Score of 0 means unknown.\n")
		fmt.Fprintf(&buf, "var %s = map[string]float64{\n", b.name)
		for _, e := range entries {
			fmt.Fprintf(&buf, "\t%q: %.1f,\n", e.key, e.score)
		}
		fmt.Fprintf(&buf, "}\n\n")
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		_, werr := w.Write(buf.Bytes())
		return cmp.Or(err, werr)
	}
	_, err = w.Write(formatted)
	return err
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
