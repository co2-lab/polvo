package agent

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const reflectionCmdTimeout = 120 * time.Second

// ReflectionPhase is one named validation phase (e.g. "lint", "test").
type ReflectionPhase struct {
	Name       string
	Commands   []string
	MaxRetries int // retries for this phase only
}

// ReflectionConfig configures the internal reflection loop.
// Backward compat: if Phases is nil but Commands is set, a single "default"
// phase is synthesized via effectivePhases().
type ReflectionConfig struct {
	// Legacy flat fields (still work)
	Enabled    bool     `koanf:"enabled"`
	MaxRetries int      `koanf:"max_retries"` // default 3
	Commands   []string `koanf:"commands"`    // validation commands to run

	// New phased fields
	Phases          []ReflectionPhase
	StopOnFirstPass bool // stop after first phase that passes; default false = run all phases
}

// effectivePhases returns the configured phases, synthesizing a single
// "default" phase from legacy Commands/MaxRetries when Phases is nil.
func (c ReflectionConfig) effectivePhases() []ReflectionPhase {
	if len(c.Phases) > 0 {
		return c.Phases
	}
	if len(c.Commands) > 0 {
		return []ReflectionPhase{{Name: "default", Commands: c.Commands, MaxRetries: c.MaxRetries}}
	}
	return nil
}

// Reflector runs validation commands and returns feedback for the agent loop.
type Reflector struct {
	cfg     ReflectionConfig
	workdir string
}

// NewReflector creates a Reflector. Returns nil if reflection is disabled.
func NewReflector(cfg ReflectionConfig, workdir string) *Reflector {
	hasCommands := len(cfg.Commands) > 0
	hasPhases := len(cfg.Phases) > 0
	if !cfg.Enabled || (!hasCommands && !hasPhases) {
		return nil
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	return &Reflector{cfg: cfg, workdir: workdir}
}

// RunAndReflect executes each validation command and returns combined output.
// passed is true only when all commands exit successfully.
// This uses the legacy flat Commands field (or the first phase's commands).
func (r *Reflector) RunAndReflect(ctx context.Context) (feedback string, passed bool) {
	if r == nil {
		return "", true
	}

	var failures []string
	for _, cmd := range r.cfg.Commands {
		out, err := r.runCmd(ctx, cmd)
		if err != nil {
			failures = append(failures, fmt.Sprintf("$ %s\n%s", cmd, out))
		}
	}

	if len(failures) == 0 {
		return "", true
	}

	return "Validation failed. Fix the following errors before continuing:\n\n" +
		strings.Join(failures, "\n---\n"), false
}

// MaxRetries returns the configured max retries.
func (r *Reflector) MaxRetries() int {
	if r == nil {
		return 0
	}
	return r.cfg.MaxRetries
}

// PhaseResult holds the outcome of a single reflection phase.
type PhaseResult struct {
	PhaseName string
	Passed    bool
	Feedback  string
	Retries   int
}

// RunPhases runs all configured phases in order.
// It returns the feedback from the first failing phase (for LLM re-try),
// a summary of all phases, and whether all phases passed.
// If StopOnFirstPass is true, it stops after the first phase that passes.
func (r *Reflector) RunPhases(ctx context.Context) (feedback string, results []PhaseResult, allPassed bool) {
	if r == nil {
		return "", nil, true
	}

	phases := r.cfg.effectivePhases()
	if len(phases) == 0 {
		return "", nil, true
	}

	allPassed = true
	for _, phase := range phases {
		maxRetries := phase.MaxRetries
		if maxRetries <= 0 {
			maxRetries = r.cfg.MaxRetries
		}
		if maxRetries <= 0 {
			maxRetries = 3
		}

		pr := PhaseResult{PhaseName: phase.Name}

		// Run this phase, retrying up to maxRetries times on failure.
		for attempt := 0; attempt <= maxRetries; attempt++ {
			pr.Retries = attempt
			phaseFeedback, passed := r.runPhase(ctx, phase)
			if passed {
				pr.Passed = true
				pr.Feedback = ""
				break
			}
			pr.Passed = false
			pr.Feedback = phaseFeedback
			// If we've exhausted retries, stop attempting this phase.
			if attempt == maxRetries {
				break
			}
		}

		results = append(results, pr)

		if !pr.Passed {
			allPassed = false
			// Return the feedback from the first failing phase.
			return pr.Feedback, results, false
		}

		if r.cfg.StopOnFirstPass {
			// Stop after first passing phase.
			return "", results, true
		}
	}

	return "", results, allPassed
}

// runPhase executes all commands in a single phase and returns feedback.
func (r *Reflector) runPhase(ctx context.Context, phase ReflectionPhase) (feedback string, passed bool) {
	var failures []string
	for _, cmd := range phase.Commands {
		out, err := r.runCmd(ctx, cmd)
		if err != nil {
			failures = append(failures, fmt.Sprintf("$ %s\n%s", cmd, out))
		}
	}

	if len(failures) == 0 {
		return "", true
	}

	return fmt.Sprintf("Phase %q failed. Fix the following errors before continuing:\n\n%s",
		phase.Name, strings.Join(failures, "\n---\n")), false
}

func (r *Reflector) runCmd(ctx context.Context, cmd string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, reflectionCmdTimeout)
	defer cancel()

	c := exec.CommandContext(ctx, "sh", "-c", cmd)
	c.Dir = r.workdir

	var out bytes.Buffer
	c.Stdout = &out
	c.Stderr = &out

	err := c.Run()
	output := out.String()
	if len(output) > 8000 {
		output = output[:8000] + "\n... (truncated)"
	}
	return output, err
}
