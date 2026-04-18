// Package visual implements desktop/browser visual testing via a screenshot→describe→act loop.
// It captures screenshots via an executor (e.g. Playwright MCP), sends them to a vision-capable
// LLM for description, and drives follow-up actions based on the description.
package visual

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"
)

// Screenshotter takes a screenshot and returns the raw PNG bytes.
type Screenshotter interface {
	Screenshot(ctx context.Context) ([]byte, error)
}

// Describer sends a screenshot (base64 PNG) to an LLM and returns a textual description.
type Describer interface {
	Describe(ctx context.Context, imageB64, prompt string) (string, error)
}

// Actor executes a named action with optional parameters.
// Action names follow Playwright conventions: "click", "type", "navigate", "scroll", "close".
type Actor interface {
	Act(ctx context.Context, action, params string) error
}

// Assertion is a textual expectation about what should be visible on screen.
type Assertion struct {
	// Description is the human-readable expected state, e.g. "login button is visible".
	Description string
	// MustContain lists substrings that must appear in the screen description.
	MustContain []string
	// MustNotContain lists substrings that must NOT appear in the screen description.
	MustNotContain []string
}

// AssertionResult is the outcome of a single assertion check.
type AssertionResult struct {
	Assertion   Assertion
	Description string // what the LLM saw
	Passed      bool
	FailReason  string
}

// StepResult records what happened during one see→describe→act step.
type StepResult struct {
	ScreenshotB64 string
	Description   string
	Action        string
	ActionParams  string
	Error         error
	Timestamp     time.Time
}

// Config configures a visual test session.
type Config struct {
	Screenshotter Screenshotter
	Describer     Describer
	Actor         Actor
	// DescribePrompt is prepended to each LLM vision call (default: generic instruction).
	DescribePrompt string
	// MaxSteps limits the number of see→act iterations (default: 10).
	MaxSteps int
	// StepDelay is an optional pause between steps (default: 0).
	StepDelay time.Duration
}

// Runner executes visual test scenarios.
type Runner struct {
	cfg Config
}

// NewRunner creates a visual test Runner.
func NewRunner(cfg Config) *Runner {
	if cfg.MaxSteps <= 0 {
		cfg.MaxSteps = 10
	}
	if cfg.DescribePrompt == "" {
		cfg.DescribePrompt = "Describe what you see on screen in detail. " +
			"List all visible UI elements, text, buttons, and their states."
	}
	return &Runner{cfg: cfg}
}

// SeeDescribeAct performs a single screenshot→describe→act cycle.
// Returns the step result including the description and any error.
func (r *Runner) SeeDescribeAct(ctx context.Context, action, actionParams string) (*StepResult, error) {
	result := &StepResult{
		Action:       action,
		ActionParams: actionParams,
		Timestamp:    time.Now(),
	}

	// Step 1: Screenshot.
	png, err := r.cfg.Screenshotter.Screenshot(ctx)
	if err != nil {
		result.Error = fmt.Errorf("screenshot: %w", err)
		return result, result.Error
	}
	result.ScreenshotB64 = base64.StdEncoding.EncodeToString(png)

	// Step 2: Describe.
	desc, err := r.cfg.Describer.Describe(ctx, result.ScreenshotB64, r.cfg.DescribePrompt)
	if err != nil {
		result.Error = fmt.Errorf("describe: %w", err)
		return result, result.Error
	}
	result.Description = desc

	// Step 3: Act (optional — empty action = observe only).
	if action != "" {
		if err := r.cfg.Actor.Act(ctx, action, actionParams); err != nil {
			result.Error = fmt.Errorf("act(%s): %w", action, err)
			return result, result.Error
		}
	}

	return result, nil
}

// Assert takes a screenshot, describes it, and checks all assertions.
// Returns one AssertionResult per assertion.
func (r *Runner) Assert(ctx context.Context, assertions []Assertion) ([]AssertionResult, error) {
	if len(assertions) == 0 {
		return nil, nil
	}

	png, err := r.cfg.Screenshotter.Screenshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("assert screenshot: %w", err)
	}
	b64 := base64.StdEncoding.EncodeToString(png)

	// Build a prompt that asks the LLM to describe the screen for assertion matching.
	prompt := r.cfg.DescribePrompt + "\nBe specific about what elements are visible and their states."
	desc, err := r.cfg.Describer.Describe(ctx, b64, prompt)
	if err != nil {
		return nil, fmt.Errorf("assert describe: %w", err)
	}

	descLower := strings.ToLower(desc)
	results := make([]AssertionResult, len(assertions))
	for i, a := range assertions {
		ar := AssertionResult{
			Assertion:   a,
			Description: desc,
			Passed:      true,
		}
		for _, must := range a.MustContain {
			if !strings.Contains(descLower, strings.ToLower(must)) {
				ar.Passed = false
				ar.FailReason = fmt.Sprintf("expected %q in description but not found", must)
				break
			}
		}
		if ar.Passed {
			for _, mustNot := range a.MustNotContain {
				if strings.Contains(descLower, strings.ToLower(mustNot)) {
					ar.Passed = false
					ar.FailReason = fmt.Sprintf("unexpected %q found in description", mustNot)
					break
				}
			}
		}
		results[i] = ar
	}
	return results, nil
}

// RunScenario executes a sequence of (action, params) pairs with optional delays.
// Returns all step results. Stops on first error unless ContinueOnError is true.
func (r *Runner) RunScenario(ctx context.Context, steps []ScenarioStep, continueOnError bool) ([]*StepResult, error) {
	results := make([]*StepResult, 0, len(steps))
	for _, step := range steps {
		if r.cfg.StepDelay > 0 {
			select {
			case <-ctx.Done():
				return results, ctx.Err()
			case <-time.After(r.cfg.StepDelay):
			}
		}
		sr, err := r.SeeDescribeAct(ctx, step.Action, step.Params)
		results = append(results, sr)
		if err != nil && !continueOnError {
			return results, err
		}
	}
	return results, nil
}

// ScenarioStep is a single action in a visual scenario.
type ScenarioStep struct {
	Action string // "" = observe only
	Params string
}

// SaveScreenshot saves a base64-encoded screenshot to a file.
func SaveScreenshot(b64 string, path string) error {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return fmt.Errorf("decode screenshot: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}
