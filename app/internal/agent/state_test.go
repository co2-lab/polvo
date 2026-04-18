package agent

import (
	"sync"
	"testing"
)

// TestStringAppendReducerOrder verifies concatenation order under concurrent Updates.
func TestStringAppendReducerOrder(t *testing.T) {
	ch := NewChannel("", StringAppendReducer("|"))
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch.Update("x")
		}()
	}
	wg.Wait()
	got := ch.Get()
	// Should have exactly 10 "x" tokens separated by "|".
	count := 0
	for _, c := range got {
		if c == 'x' {
			count++
		}
	}
	if count != 10 {
		t.Errorf("expected 10 'x' tokens, got %d in %q", count, got)
	}
}

// TestSliceAppendReducerConcurrent verifies that N concurrent Updates produce
// a slice containing all elements without any lost writes.
func TestSliceAppendReducerConcurrent(t *testing.T) {
	ch := NewChannel([]Finding(nil), SliceAppendReducer[Finding]())
	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ch.Update([]Finding{{Message: "f"}})
		}(i)
	}
	wg.Wait()
	if got := len(ch.Get()); got != n {
		t.Errorf("expected %d findings, got %d", n, got)
	}
}

// TestFromInputToInputRoundTrip verifies no data loss across the conversion.
func TestFromInputToInputRoundTrip(t *testing.T) {
	in := &Input{
		File:            "main.go",
		Content:         "package main",
		Diff:            "- old\n+ new",
		Event:           "push",
		ProjectRoot:     "/repo",
		Interface:       "io.Writer",
		Spec:            "spec text",
		Feature:         "feature A",
		PRDiff:          "pr diff",
		PRComments:      "pr comments",
		PreviousReports: "prev",
		FileHistory:     "history",
	}
	out := FromInput(in).ToInput()
	if out.File != in.File {
		t.Errorf("File: want %q got %q", in.File, out.File)
	}
	if out.Content != in.Content {
		t.Errorf("Content: want %q got %q", in.Content, out.Content)
	}
	if out.Diff != in.Diff {
		t.Errorf("Diff: want %q got %q", in.Diff, out.Diff)
	}
	if out.ProjectRoot != in.ProjectRoot {
		t.Errorf("ProjectRoot: want %q got %q", in.ProjectRoot, out.ProjectRoot)
	}
}

// TestChannelVersion verifies Version increments on each Update.
func TestChannelVersion(t *testing.T) {
	ch := NewChannel("", StringReplaceReducer)
	if ch.Version != 0 {
		t.Errorf("initial version should be 0, got %d", ch.Version)
	}
	ch.Update("a")
	ch.Update("b")
	if ch.Version != 2 {
		t.Errorf("expected version 2, got %d", ch.Version)
	}
}
