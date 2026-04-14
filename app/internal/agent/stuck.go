package agent

import (
	"crypto/md5"
	"fmt"
	"sync"
)

// ToolCallRecord captures a single tool call for stuck detection.
type ToolCallRecord struct {
	Tool    string
	Input   string // JSON-serialised input (hashed if large)
	IsError bool
}

// StuckPattern identifies which stuck pattern was detected.
type StuckPattern int

const (
	StuckPatternNone          StuckPattern = iota
	StuckPatternRepeat                     // same tool+input repeated >= threshold times
	StuckPatternErrorLoop                  // same tool fails 3x consecutively
	StuckPatternCyclic                     // A→B→A→B pattern in last 6 calls
	StuckPatternMonologue                  // 4+ text turns without any tool call
	StuckPatternContextWindow              // 3+ condensations without progress
)

// String returns a human-readable name for the pattern.
func (p StuckPattern) String() string {
	switch p {
	case StuckPatternRepeat:
		return "repeat"
	case StuckPatternErrorLoop:
		return "error_loop"
	case StuckPatternCyclic:
		return "cyclic"
	case StuckPatternMonologue:
		return "monologue"
	case StuckPatternContextWindow:
		return "context_window"
	default:
		return "none"
	}
}

// StuckDetector detects when an agent is stuck in a repetitive loop.
type StuckDetector struct {
	WindowSize int // recent calls to examine (default 6)
	Threshold  int // min repetitions to declare stuck (default 3)
	mu         sync.Mutex
	history    []ToolCallRecord

	textTurnCount    int // consecutive turns with no tool calls
	condensationCount int // consecutive condensations
}

// NewStuckDetector creates a detector with sensible defaults.
func NewStuckDetector(windowSize, threshold int) *StuckDetector {
	if windowSize <= 0 {
		windowSize = 6
	}
	if threshold <= 0 {
		threshold = 3
	}
	return &StuckDetector{WindowSize: windowSize, Threshold: threshold}
}

// Record appends a tool call to the history ring (IsError defaults to false).
func (s *StuckDetector) Record(tool, input string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = append(s.history, ToolCallRecord{Tool: tool, Input: inputSig(input), IsError: false})
	s.textTurnCount = 0 // reset monologue counter on any tool call
}

// RecordResult appends a tool call with its error status to the history ring.
// It also resets the text-turn (monologue) counter.
func (s *StuckDetector) RecordResult(tool, input string, isError bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = append(s.history, ToolCallRecord{Tool: tool, Input: inputSig(input), IsError: isError})
	s.textTurnCount = 0 // reset monologue counter on any tool call
}

// RecordTextTurn increments the counter for consecutive text-only turns (no tool calls).
func (s *StuckDetector) RecordTextTurn() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.textTurnCount++
}

// RecordCondensation increments the counter for consecutive condensations.
func (s *StuckDetector) RecordCondensation() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.condensationCount++
}

// ResetTextTurn resets the monologue counter. Call this when a tool call happens
// if you prefer explicit control rather than relying on Record/RecordResult.
func (s *StuckDetector) ResetTextTurn() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.textTurnCount = 0
}

// CheckAll checks all stuck patterns in priority order and returns the first
// match. Priority: ErrorLoop > Cyclic > Repeat > Monologue > ContextWindow.
func (s *StuckDetector) CheckAll() (StuckPattern, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. ErrorLoop: last 3 records are the same tool, all IsError=true
	if len(s.history) >= 3 {
		last3 := s.history[len(s.history)-3:]
		firstName := last3[0].Tool
		allSameTool := true
		allErrors := true
		for _, r := range last3 {
			if r.Tool != firstName {
				allSameTool = false
				break
			}
			if !r.IsError {
				allErrors = false
				break
			}
		}
		if allSameTool && allErrors {
			return StuckPatternErrorLoop, true
		}
	}

	// 2. Cyclic: last 6 records form A,B,A,B,A,B pattern
	// pos 0,2,4 equal AND pos 1,3,5 equal AND A≠B
	if len(s.history) >= 6 {
		last6 := s.history[len(s.history)-6:]
		a := last6[0].Tool + ":" + last6[0].Input
		b := last6[1].Tool + ":" + last6[1].Input
		if a != b &&
			last6[2].Tool+":"+last6[2].Input == a &&
			last6[3].Tool+":"+last6[3].Input == b &&
			last6[4].Tool+":"+last6[4].Input == a &&
			last6[5].Tool+":"+last6[5].Input == b {
			return StuckPatternCyclic, true
		}
	}

	// 3. Repeat: existing logic — same tool+input >= threshold times in window
	if len(s.history) >= s.Threshold {
		window := s.history
		if len(window) > s.WindowSize {
			window = window[len(window)-s.WindowSize:]
		}
		counts := make(map[string]int)
		for _, r := range window {
			key := r.Tool + ":" + r.Input
			counts[key]++
			if counts[key] >= s.Threshold {
				return StuckPatternRepeat, true
			}
		}
	}

	// 4. Monologue: 4+ consecutive text turns without any tool call
	if s.textTurnCount >= 4 {
		return StuckPatternMonologue, true
	}

	// 5. ContextWindow: 3+ condensations without progress
	if s.condensationCount >= 3 {
		return StuckPatternContextWindow, true
	}

	return StuckPatternNone, false
}

// IsStuck returns true if any stuck pattern is detected.
// It is a backward-compatible wrapper around CheckAll.
func (s *StuckDetector) IsStuck() bool {
	_, ok := s.CheckAll()
	return ok
}

// inputSig returns a short hash of the input to avoid storing large blobs.
func inputSig(input string) string {
	if len(input) <= 32 {
		return input
	}
	h := md5.Sum([]byte(input)) //nolint:gosec — used for dedup, not security
	return fmt.Sprintf("%x", h)
}
