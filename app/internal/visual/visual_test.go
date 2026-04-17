package visual

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mockScreenshotter returns a 1x1 PNG (minimal valid PNG bytes).
type mockScreenshotter struct {
	data []byte
	err  error
}

// Minimal 1x1 transparent PNG (67 bytes).
var minimalPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9c, 0x62, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
	0x42, 0x60, 0x82,
}

func (m *mockScreenshotter) Screenshot(_ context.Context) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.data != nil {
		return m.data, nil
	}
	return minimalPNG, nil
}

// mockDescriber returns a fixed description.
type mockDescriber struct {
	desc string
	err  error
}

func (m *mockDescriber) Describe(_ context.Context, _, _ string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.desc, nil
}

// mockActor records called actions.
type mockActor struct {
	calls []string
	err   error
}

func (m *mockActor) Act(_ context.Context, action, params string) error {
	if m.err != nil {
		return m.err
	}
	m.calls = append(m.calls, action+":"+params)
	return nil
}

func newRunner(desc string) *Runner {
	return NewRunner(Config{
		Screenshotter: &mockScreenshotter{},
		Describer:     &mockDescriber{desc: desc},
		Actor:         &mockActor{},
	})
}

func TestRunner_Defaults(t *testing.T) {
	r := NewRunner(Config{
		Screenshotter: &mockScreenshotter{},
		Describer:     &mockDescriber{desc: "ui"},
		Actor:         &mockActor{},
	})
	if r.cfg.MaxSteps != 10 {
		t.Errorf("default MaxSteps: want 10, got %d", r.cfg.MaxSteps)
	}
	if r.cfg.DescribePrompt == "" {
		t.Error("DescribePrompt should have default value")
	}
}

func TestSeeDescribeAct_ObserveOnly(t *testing.T) {
	r := newRunner("login button visible")
	sr, err := r.SeeDescribeAct(context.Background(), "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sr.Description != "login button visible" {
		t.Errorf("got %q", sr.Description)
	}
	if sr.ScreenshotB64 == "" {
		t.Error("ScreenshotB64 should be set")
	}
	if sr.Timestamp.IsZero() {
		t.Error("Timestamp should be set")
	}
}

func TestSeeDescribeAct_WithAction(t *testing.T) {
	actor := &mockActor{}
	r := NewRunner(Config{
		Screenshotter: &mockScreenshotter{},
		Describer:     &mockDescriber{desc: "page loaded"},
		Actor:         actor,
	})
	_, err := r.SeeDescribeAct(context.Background(), "click", "#submit")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actor.calls) != 1 || actor.calls[0] != "click:#submit" {
		t.Errorf("actor calls: %v", actor.calls)
	}
}

func TestSeeDescribeAct_ScreenshotError(t *testing.T) {
	r := NewRunner(Config{
		Screenshotter: &mockScreenshotter{err: errors.New("browser crashed")},
		Describer:     &mockDescriber{},
		Actor:         &mockActor{},
	})
	sr, err := r.SeeDescribeAct(context.Background(), "", "")
	if err == nil {
		t.Error("expected error from screenshotter")
	}
	if sr == nil {
		t.Fatal("StepResult should always be returned")
	}
	if !strings.Contains(err.Error(), "screenshot") {
		t.Errorf("expected 'screenshot' in error: %v", err)
	}
}

func TestSeeDescribeAct_DescribeError(t *testing.T) {
	r := NewRunner(Config{
		Screenshotter: &mockScreenshotter{},
		Describer:     &mockDescriber{err: errors.New("vision model unavailable")},
		Actor:         &mockActor{},
	})
	_, err := r.SeeDescribeAct(context.Background(), "", "")
	if err == nil {
		t.Error("expected error from describer")
	}
}

func TestSeeDescribeAct_ActorError(t *testing.T) {
	r := NewRunner(Config{
		Screenshotter: &mockScreenshotter{},
		Describer:     &mockDescriber{desc: "ok"},
		Actor:         &mockActor{err: errors.New("click failed")},
	})
	_, err := r.SeeDescribeAct(context.Background(), "click", "#btn")
	if err == nil {
		t.Error("expected error from actor")
	}
}

func TestAssert_AllPass(t *testing.T) {
	r := newRunner("login button is visible and enabled")
	results, err := r.Assert(context.Background(), []Assertion{
		{Description: "login visible", MustContain: []string{"login", "button"}},
		{Description: "no error", MustNotContain: []string{"error", "crash"}},
	})
	if err != nil {
		t.Fatalf("Assert: %v", err)
	}
	for _, res := range results {
		if !res.Passed {
			t.Errorf("assertion %q failed: %s", res.Assertion.Description, res.FailReason)
		}
	}
}

func TestAssert_Fail_MustContain(t *testing.T) {
	r := newRunner("error page displayed")
	results, err := r.Assert(context.Background(), []Assertion{
		{MustContain: []string{"login"}},
	})
	if err != nil {
		t.Fatalf("Assert: %v", err)
	}
	if len(results) != 1 || results[0].Passed {
		t.Error("assertion should fail: 'login' not in description")
	}
}

func TestAssert_Fail_MustNotContain(t *testing.T) {
	r := newRunner("fatal error: null pointer")
	results, err := r.Assert(context.Background(), []Assertion{
		{MustNotContain: []string{"error"}},
	})
	if err != nil {
		t.Fatalf("Assert: %v", err)
	}
	if len(results) != 1 || results[0].Passed {
		t.Error("assertion should fail: 'error' found in description")
	}
}

func TestAssert_Empty(t *testing.T) {
	r := newRunner("anything")
	results, err := r.Assert(context.Background(), nil)
	if err != nil || results != nil {
		t.Errorf("empty assertions: want nil,nil got %v,%v", results, err)
	}
}

func TestRunScenario_AllSteps(t *testing.T) {
	actor := &mockActor{}
	r := NewRunner(Config{
		Screenshotter: &mockScreenshotter{},
		Describer:     &mockDescriber{desc: "step done"},
		Actor:         actor,
	})
	steps := []ScenarioStep{
		{Action: "navigate", Params: "http://localhost"},
		{Action: "click", Params: "#start"},
		{Action: "", Params: ""}, // observe only
	}
	results, err := r.RunScenario(context.Background(), steps, false)
	if err != nil {
		t.Fatalf("RunScenario: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
	if len(actor.calls) != 2 { // observe step has no action
		t.Errorf("expected 2 actor calls, got %d: %v", len(actor.calls), actor.calls)
	}
}

func TestRunScenario_StopOnError(t *testing.T) {
	callCount := 0
	r := NewRunner(Config{
		Screenshotter: &mockScreenshotter{},
		Describer:     &mockDescriber{desc: "ui"},
		Actor: &mockActor{err: errors.New("act failed")},
	})
	_ = callCount
	steps := []ScenarioStep{
		{Action: "click", Params: "a"},
		{Action: "click", Params: "b"},
	}
	results, err := r.RunScenario(context.Background(), steps, false)
	if err == nil {
		t.Error("expected error from actor")
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result before stop, got %d", len(results))
	}
}

func TestRunScenario_ContinueOnError(t *testing.T) {
	r := NewRunner(Config{
		Screenshotter: &mockScreenshotter{},
		Describer:     &mockDescriber{desc: "ui"},
		Actor:         &mockActor{err: errors.New("act failed")},
	})
	steps := []ScenarioStep{
		{Action: "click", Params: "a"},
		{Action: "click", Params: "b"},
	}
	results, _ := r.RunScenario(context.Background(), steps, true)
	if len(results) != 2 {
		t.Errorf("continueOnError=true: expected 2 results, got %d", len(results))
	}
}

func TestRunScenario_StepDelay(t *testing.T) {
	r := NewRunner(Config{
		Screenshotter: &mockScreenshotter{},
		Describer:     &mockDescriber{desc: "ui"},
		Actor:         &mockActor{},
		StepDelay:     5 * time.Millisecond,
	})
	steps := []ScenarioStep{{Action: "", Params: ""}, {Action: "", Params: ""}}
	start := time.Now()
	_, _ = r.RunScenario(context.Background(), steps, false)
	elapsed := time.Since(start)
	if elapsed < 5*time.Millisecond {
		t.Errorf("expected at least 5ms delay, got %v", elapsed)
	}
}

func TestSaveScreenshot(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString(minimalPNG)
	path := filepath.Join(t.TempDir(), "shot.png")
	if err := SaveScreenshot(b64, path); err != nil {
		t.Fatalf("SaveScreenshot: %v", err)
	}
	// File should exist and have PNG bytes.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty file")
	}
}
