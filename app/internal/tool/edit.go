package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/co2-lab/polvo/internal/filelock"
)

// MatchLevel indicates which fallback level matched.
type MatchLevel int

const (
	LevelNone           MatchLevel = 0
	LevelExact          MatchLevel = 1
	LevelWhitespace     MatchLevel = 2
	LevelRelativeIndent MatchLevel = 3
	LevelFuzzy          MatchLevel = 4
	LevelWholeFile      MatchLevel = 5
)

// FallbackConfig holds thresholds for fuzzy matching.
type FallbackConfig struct {
	FuzzyThresholdAuto    float64 // apply silently (default 0.95)
	FuzzyThresholdConfirm float64 // apply + warn (default 0.80)
	FuzzyThresholdMin     float64 // return diagnostic below this (default 0.60)
}

func defaultFallbackConfig() FallbackConfig {
	return FallbackConfig{
		FuzzyThresholdAuto:    0.95,
		FuzzyThresholdConfirm: 0.80,
		FuzzyThresholdMin:     0.60,
	}
}

// EditBlock represents a single search/replace operation.
type EditBlock struct {
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
	StartLine int    `json:"start_line"` // 0 = no hint
}

type editInput struct {
	Path         string      `json:"path"`
	OldString    string      `json:"old_string"`
	NewString    string      `json:"new_string"`
	ReplaceAll   bool        `json:"replace_all"`
	StartLine    int         `json:"start_line"`    // optional hint for fuzzy matching
	NewContent   string      `json:"new_content"`   // level 5: whole-file rewrite
	Edits        []EditBlock `json:"edits"`         // multi-block support
	SecurityRisk string      `json:"security_risk"` // low | medium | high | critical
}

type editTool struct {
	workdir string
	ignore  Ignorer
	cache   *ToolCache
}

// NewEditTool creates an edit tool without cache invalidation.
func NewEditTool(workdir string, ig Ignorer) Tool { return NewEditToolWithCache(workdir, ig, nil) }

// NewEditToolWithCache creates an edit tool that invalidates the cache on successful edits.
func NewEditToolWithCache(workdir string, ig Ignorer, cache *ToolCache) Tool {
	return &editTool{workdir: workdir, ignore: ig, cache: cache}
}

func (t *editTool) Name() string { return "edit" }

func (t *editTool) Description() string {
	return "Replace a string in a file. Uses a 5-level fallback cascade: exact match, whitespace-flexible, relative indent, fuzzy (Levenshtein), and whole-file rewrite."
}

func (t *editTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path":          {"type": "string", "description": "File path relative to working directory"},
			"old_string":    {"type": "string", "description": "Exact string to find and replace"},
			"new_string":    {"type": "string", "description": "Replacement string"},
			"replace_all":   {"type": "boolean", "description": "Replace all occurrences (default false)", "default": false},
			"start_line":    {"type": "integer", "description": "Optional 1-based line hint for fuzzy matching (0 = no hint)", "default": 0},
			"new_content":   {"type": "string", "description": "Level 5: complete new file content (whole-file rewrite)"},
			"security_risk": {"type": "string", "enum": ["low","medium","high","critical"], "description": "Assessed risk level of this edit operation", "default": "low"},
			"edits": {
				"type": "array",
				"description": "Multiple search/replace blocks; applied bottom-to-top by file position",
				"items": {
					"type": "object",
					"properties": {
						"old_string": {"type": "string"},
						"new_string": {"type": "string"},
						"start_line": {"type": "integer", "default": 0}
					},
					"required": ["old_string", "new_string"]
				}
			}
		},
		"required": ["path"]
	}`)
}

func (t *editTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in editInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ErrorResult(fmt.Sprintf("invalid input: %v", err)), nil
	}

	path, err := resolvePath(t.workdir, in.Path)
	if err != nil {
		return ErrorResult(err.Error()), nil
	}
	if err := checkIgnored(t.ignore, path); err != nil {
		return ErrorResult(err.Error()), nil
	}

	// Acquire exclusive write lock with a 30-second timeout.
	lockCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	unlock, err := filelock.Global.LockWrite(lockCtx, path)
	if err != nil {
		return ErrorResult(fmt.Sprintf("acquiring write lock: %v", err)), nil
	}
	defer unlock()

	// Log security risk for high/critical operations.
	if in.SecurityRisk == "high" || in.SecurityRisk == "critical" {
		slog.Warn("file edit", "path", in.Path, "risk", in.SecurityRisk)
	}

	cfg := defaultFallbackConfig()

	// Level 5: whole-file rewrite
	if in.NewContent != "" {
		if err := os.WriteFile(path, []byte(in.NewContent), 0644); err != nil {
			return ErrorResult(fmt.Sprintf("writing file: %v", err)), nil
		}
		if t.cache != nil {
			t.cache.Invalidate(path)
		}
		return &Result{Content: fmt.Sprintf("Whole-file rewrite applied to %s (level 5)", in.Path)}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return ErrorResult(fmt.Sprintf("reading file: %v", err)), nil
	}

	// Multi-block mode
	if len(in.Edits) > 0 {
		result, msgs, applyErr := applyMultipleEdits(data, in.Edits, cfg)
		if applyErr != nil {
			return ErrorResult(applyErr.Error()), nil
		}
		if err := os.WriteFile(path, result, 0644); err != nil {
			return ErrorResult(fmt.Sprintf("writing file: %v", err)), nil
		}
		if t.cache != nil {
			t.cache.Invalidate(path)
		}
		return &Result{Content: fmt.Sprintf("Applied %d edit(s) to %s%s", len(in.Edits), in.Path, msgs)}, nil
	}

	// Single-block mode
	if in.OldString == in.NewString {
		return ErrorResult("old_string and new_string must be different"), nil
	}

	if in.ReplaceAll {
		content := string(data)
		count := strings.Count(content, in.OldString)
		if count == 0 {
			return buildNotFoundError(in.Path, content, in.OldString), nil
		}
		newContent := strings.ReplaceAll(content, in.OldString, in.NewString)
		if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
			return ErrorResult(fmt.Sprintf("writing file: %v", err)), nil
		}
		if t.cache != nil {
			t.cache.Invalidate(path)
		}
		return &Result{Content: fmt.Sprintf("Replaced %d occurrence(s) in %s", count, in.Path)}, nil
	}

	newData, level, score, applyErr := applyWithFallback(data, in.OldString, in.NewString, in.StartLine, cfg)
	if applyErr != nil {
		return ErrorResult(applyErr.Error()), nil
	}

	if err := os.WriteFile(path, newData, 0644); err != nil {
		return ErrorResult(fmt.Sprintf("writing file: %v", err)), nil
	}
	if t.cache != nil {
		t.cache.Invalidate(path)
	}

	msg := fmt.Sprintf("Replaced 1 occurrence(s) in %s", in.Path)
	switch level {
	case LevelWhitespace:
		msg += " (whitespace-flexible match)"
	case LevelRelativeIndent:
		msg += " (relative-indent match)"
	case LevelFuzzy:
		msg += fmt.Sprintf(" (fuzzy match, similarity=%.2f)", score)
		if score < cfg.FuzzyThresholdAuto {
			msg += " — fuzzy match applied; verify the result is correct"
		}
	}
	return &Result{Content: msg}, nil
}

// applyWithFallback tries levels 1–4 in cascade and returns the modified content.
// Returns (newContent, level, score, error).
func applyWithFallback(content []byte, search, replace string, hintLine int, cfg FallbackConfig) ([]byte, MatchLevel, float64, error) {
	s := string(content)

	// Level 1: exact match
	count := strings.Count(s, search)
	if count == 1 {
		return []byte(strings.Replace(s, search, replace, 1)), LevelExact, 1.0, nil
	}
	if count > 1 {
		return nil, LevelNone, 0, fmt.Errorf(
			"edit failed: old_string found %d times (must be unique).\n"+
				"Tip: include more surrounding context in old_string to make it unique, or use replace_all.",
			count,
		)
	}

	// Level 2: whitespace-flexible match
	if start, end, found := matchFlexibleWhitespace(s, search); found {
		newS := s[:start] + replace + s[end:]
		return []byte(newS), LevelWhitespace, 1.0, nil
	}

	// Level 3: relative indentation match
	if start, end, found := matchRelativeIndent(s, search); found {
		newS := s[:start] + replace + s[end:]
		return []byte(newS), LevelRelativeIndent, 1.0, nil
	}

	// Level 4: fuzzy middle-out (Levenshtein)
	start, end, score := matchFuzzyMiddleOut(s, search, hintLine)
	if score >= cfg.FuzzyThresholdConfirm && start >= 0 {
		newS := s[:start] + replace + s[end:]
		return []byte(newS), LevelFuzzy, score, nil
	}

	// Build diagnostic for the LLM
	diagnostic := buildDiagnostic(s, search, score, start, end, cfg)
	return nil, LevelNone, score, fmt.Errorf("%s", diagnostic)
}

// applyMultipleEdits applies multiple EditBlocks in bottom-to-top file order (atomic).
func applyMultipleEdits(content []byte, edits []EditBlock, cfg FallbackConfig) ([]byte, string, error) {
	type positioned struct {
		block EditBlock
		pos   int
	}

	s := string(content)
	positioned_edits := make([]positioned, 0, len(edits))

	// Detect position of each block in the file
	for _, e := range edits {
		pos := strings.Index(s, e.OldString)
		if pos < 0 {
			// Try whitespace-flexible for position detection
			if st, _, found := matchFlexibleWhitespace(s, e.OldString); found {
				pos = st
			} else if st, _, found := matchRelativeIndent(s, e.OldString); found {
				pos = st
			} else {
				pos = len(s) // put unknown at end — will fail gracefully
			}
		}
		positioned_edits = append(positioned_edits, positioned{block: e, pos: pos})
	}

	// Sort by position descending (bottom-to-top application)
	sort.Slice(positioned_edits, func(i, j int) bool {
		return positioned_edits[i].pos > positioned_edits[j].pos
	})

	// Apply each edit on a copy, fail atomically
	result := content
	var extraMsgs []string
	for _, pe := range positioned_edits {
		newData, level, score, err := applyWithFallback(result, pe.block.OldString, pe.block.NewString, pe.block.StartLine, cfg)
		if err != nil {
			return content, "", fmt.Errorf("edit block failed: %w", err)
		}
		result = newData
		if level == LevelFuzzy {
			extraMsgs = append(extraMsgs, fmt.Sprintf(" (block used fuzzy match, similarity=%.2f)", score))
		}
	}

	msg := ""
	if len(extraMsgs) > 0 {
		msg = "\n" + strings.Join(extraMsgs, "\n")
	}
	return result, msg, nil
}

// matchFlexibleWhitespace attempts sub-variants:
// a) trailing whitespace stripped per line
// b) shared minimum leading indentation removed
// c) leading blank line in search ignored
func matchFlexibleWhitespace(content, search string) (start, end int, found bool) {
	// 2a: strip trailing whitespace on each line of search
	stripped := stripTrailingWhitespace(search)
	if stripped != search {
		if s, e, ok := findInContent(content, stripped); ok {
			return s, e, true
		}
	}

	// 2b: remove shared minimum leading indentation
	dedented, minIndent := dedentBlock(search)
	if minIndent > 0 {
		// Try matching dedented search against content lines that have that indentation removed
		if s, e, ok := findDedented(content, dedented, minIndent); ok {
			return s, e, true
		}
	}

	// 2c: leading blank line in search ignored
	trimmed := strings.TrimPrefix(search, "\n")
	if trimmed != search {
		if s, e, ok := findInContent(content, trimmed); ok {
			return s, e, true
		}
	}
	// Also trailing blank line
	trimmed2 := strings.TrimSuffix(search, "\n")
	if trimmed2 != search {
		if s, e, ok := findInContent(content, trimmed2); ok {
			return s, e, true
		}
	}

	return 0, 0, false
}

// matchRelativeIndent tries to match search after normalizing indentation differences.
// It strips all leading whitespace from each line of both sides and matches structurally.
func matchRelativeIndent(content, search string) (start, end int, found bool) {
	searchLines := strings.Split(search, "\n")
	if len(searchLines) < 2 {
		return 0, 0, false
	}

	// Build a normalized version of the search block (strip all leading whitespace)
	normSearch := normalizeIndentLines(searchLines)

	contentLines := strings.Split(content, "\n")
	searchLineCount := len(searchLines)

	for i := 0; i <= len(contentLines)-searchLineCount; i++ {
		window := contentLines[i : i+searchLineCount]
		normWindow := normalizeIndentLines(window)
		if normWindow == normSearch {
			// Found — compute byte offsets
			startOffset := lineOffsets(content, i)
			endOffset := lineOffsets(content, i+searchLineCount)
			return startOffset, endOffset, true
		}
	}
	return 0, 0, false
}

// matchFuzzyMiddleOut does a sliding window Levenshtein search, starting from hintLine.
// Returns (start, end, bestScore). start=-1 if no match above minimum threshold.
func matchFuzzyMiddleOut(content, search string, hintLine int) (start, end int, score float64) {
	contentLines := strings.Split(content, "\n")
	searchLines := strings.Split(search, "\n")
	searchLineCount := len(searchLines)

	if searchLineCount == 0 || len(contentLines) < searchLineCount {
		return -1, 0, 0
	}

	maxWindows := len(contentLines) - searchLineCount + 1
	if maxWindows <= 0 {
		return -1, 0, 0
	}

	// Determine starting index for middle-out search
	centerIdx := 0
	if hintLine > 0 {
		centerIdx = hintLine - 1 // convert 1-based to 0-based
		if centerIdx >= maxWindows {
			centerIdx = maxWindows - 1
		}
	}

	bestScore := -1.0
	bestStart := -1
	bestEnd := 0

	visited := make([]bool, maxWindows)

	// Middle-out: alternating around centerIdx
	for delta := 0; delta < maxWindows; delta++ {
		for _, sign := range []int{0, 1} {
			var idx int
			if delta == 0 {
				idx = centerIdx
			} else if sign == 0 {
				idx = centerIdx - delta
			} else {
				idx = centerIdx + delta
			}
			if idx < 0 || idx >= maxWindows || visited[idx] {
				continue
			}
			visited[idx] = true

			window := contentLines[idx : idx+searchLineCount]
			windowStr := strings.Join(window, "\n")
			sim := stringSimilarity(search, windowStr)

			if sim > bestScore {
				bestScore = sim
				bestStart = idx
				bestEnd = idx + searchLineCount
			}

			// Early exit if perfect match found at level 2+
			if bestScore >= 0.999 {
				break
			}
		}
		if bestScore >= 0.999 {
			break
		}
	}

	if bestScore < 0.60 || bestStart < 0 {
		return -1, 0, bestScore
	}

	// Convert line indices to byte offsets
	startOffset := lineOffsets(content, bestStart)
	endOffset := lineOffsets(content, bestEnd)
	return startOffset, endOffset, bestScore
}

// levenshtein computes the edit distance between two strings (O(m*n)).
func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	// Use two-row DP to save memory
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			curr[j] = minInt(del, minInt(ins, sub))
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

// stringSimilarity returns 1 - levenshtein(a,b)/max(len(a),len(b)).
func stringSimilarity(a, b string) float64 {
	if a == b {
		return 1.0
	}
	la, lb := len(a), len(b)
	maxLen := la
	if lb > maxLen {
		maxLen = lb
	}
	if maxLen == 0 {
		return 1.0
	}
	dist := levenshtein(a, b)
	return 1.0 - float64(dist)/float64(maxLen)
}

// buildDiagnostic creates a rich error message for the LLM.
func buildDiagnostic(content, search string, bestScore float64, bestStart, bestEnd int, cfg FallbackConfig) string {
	msg := fmt.Sprintf("edit failed: old_string not found.\n")

	if bestScore >= cfg.FuzzyThresholdMin && bestStart >= 0 {
		contentLines := strings.Split(content, "\n")
		// Recover line numbers from byte offset
		lineNum := strings.Count(content[:bestStart], "\n")
		endLine := lineNum + strings.Count(search, "\n") + 1
		if endLine > len(contentLines) {
			endLine = len(contentLines)
		}

		snippet := ""
		for i := lineNum; i < endLine && i < len(contentLines); i++ {
			snippet += contentLines[i] + "\n"
		}

		msg += fmt.Sprintf("\nBest match found (similarity %.2f, around line %d):\n```\n%s```\n", bestScore, lineNum+1, snippet)
		msg += fmt.Sprintf("\nThe SEARCH block provided was:\n```\n%s```\n", search)
		msg += "\nTip: Use the 'read' tool to see the exact current file content, then retry with the exact string."
	} else {
		// General not-found hint
		normalized := normalizeWhitespace(content)
		normalizedSearch := normalizeWhitespace(search)
		if strings.Contains(normalized, normalizedSearch) {
			msg += "\nWhitespace mismatch detected. Use 'read' to see exact content."
		} else {
			hint := findSimilarLines(content, search)
			msg += "\nTip: use the 'read' tool to verify the current file content, then retry."
			if hint != "" {
				msg += "\n\nDid you mean one of these lines?\n" + hint
			}
		}
	}
	return msg
}

// buildNotFoundError builds the existing-style not-found error for replace_all mode.
func buildNotFoundError(filePath, content, search string) *Result {
	normalized := normalizeWhitespace(content)
	normalizedSearch := normalizeWhitespace(search)
	if strings.Contains(normalized, normalizedSearch) {
		return ErrorResult(fmt.Sprintf(
			"edit failed: old_string not found verbatim in %s — whitespace mismatch detected.\n"+
				"Tip: use the 'read' tool to see the exact whitespace, then retry with the exact string.",
			filePath,
		))
	}
	hint := findSimilarLines(content, search)
	msg := fmt.Sprintf(
		"edit failed: old_string not found in %s.\n"+
			"Tip: use the 'read' tool to verify the current file content, "+
			"then retry with the exact string, or use 'write' to rewrite the entire file.",
		filePath,
	)
	if hint != "" {
		msg += "\n\nDid you mean one of these lines?\n" + hint
	}
	return ErrorResult(msg)
}

// --- helpers ---

// stripTrailingWhitespace removes trailing spaces/tabs from each line.
func stripTrailingWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.Join(lines, "\n")
}

// dedentBlock removes the shared minimum leading whitespace from all non-empty lines.
// Returns the dedented block and the amount removed.
func dedentBlock(s string) (string, int) {
	lines := strings.Split(s, "\n")
	minIndent := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n := leadingSpaces(l)
		if minIndent < 0 || n < minIndent {
			minIndent = n
		}
	}
	if minIndent <= 0 {
		return s, 0
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		if len(l) >= minIndent {
			out[i] = l[minIndent:]
		} else {
			out[i] = l
		}
	}
	return strings.Join(out, "\n"), minIndent
}

// findDedented searches for dedented search in content, reindenting the replacement correctly.
// It slides a window of len(searchLines) lines and tries matching after dedenting the window.
func findDedented(content, dedentedSearch string, removedIndent int) (start, end int, found bool) {
	contentLines := strings.Split(content, "\n")
	searchLines := strings.Split(dedentedSearch, "\n")
	searchLineCount := len(searchLines)

	for i := 0; i <= len(contentLines)-searchLineCount; i++ {
		window := contentLines[i : i+searchLineCount]
		// Detect indentation of this window
		windowIndent := -1
		for _, l := range window {
			if strings.TrimSpace(l) == "" {
				continue
			}
			n := leadingSpaces(l)
			if windowIndent < 0 || n < windowIndent {
				windowIndent = n
			}
		}
		if windowIndent < 0 {
			windowIndent = 0
		}
		// Dedent window by its own minimum indent
		dedentedWindow := make([]string, len(window))
		for j, l := range window {
			if len(l) >= windowIndent {
				dedentedWindow[j] = l[windowIndent:]
			} else {
				dedentedWindow[j] = l
			}
		}
		if strings.Join(dedentedWindow, "\n") == dedentedSearch {
			startOffset := lineOffsets(content, i)
			endOffset := lineOffsets(content, i+searchLineCount)
			return startOffset, endOffset, true
		}
	}
	return 0, 0, false
}

// findInContent finds needle in content and returns byte offsets.
func findInContent(content, needle string) (start, end int, found bool) {
	idx := strings.Index(content, needle)
	if idx < 0 {
		return 0, 0, false
	}
	return idx, idx + len(needle), true
}

// normalizeIndentLines returns the joined lines with all leading whitespace stripped.
func normalizeIndentLines(lines []string) string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.TrimLeft(l, " \t")
	}
	return strings.Join(out, "\n")
}

// lineOffsets returns the byte offset of the start of lineIdx in content.
// If lineIdx >= number of lines, returns len(content).
func lineOffsets(content string, lineIdx int) int {
	if lineIdx <= 0 {
		return 0
	}
	offset := 0
	for i := 0; i < lineIdx; i++ {
		nl := strings.Index(content[offset:], "\n")
		if nl < 0 {
			return len(content)
		}
		offset += nl + 1
	}
	return offset
}

// leadingSpaces counts leading spaces (tabs count as 1).
func leadingSpaces(s string) int {
	n := 0
	for _, c := range s {
		if c == ' ' || c == '\t' {
			n++
		} else {
			break
		}
	}
	return n
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// findSimilarLines returns up to 3 lines from content that contain any word
// from the search string (for "did you mean?" hints).
func findSimilarLines(content, search string) string {
	words := strings.Fields(search)
	if len(words) == 0 {
		return ""
	}
	// Use first distinctive word (skip short words)
	keyword := ""
	for _, w := range words {
		if len(w) > 3 {
			keyword = w
			break
		}
	}
	if keyword == "" {
		return ""
	}

	var matches []string
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, keyword) {
			matches = append(matches, "  "+strings.TrimSpace(line))
			if len(matches) >= 3 {
				break
			}
		}
	}
	return strings.Join(matches, "\n")
}

func normalizeWhitespace(s string) string {
	var sb strings.Builder
	inSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' {
			if !inSpace {
				sb.WriteByte(' ')
				inSpace = true
			}
		} else {
			sb.WriteRune(r)
			inSpace = false
		}
	}
	return sb.String()
}
