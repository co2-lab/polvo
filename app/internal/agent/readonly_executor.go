package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/co2-lab/polvo/internal/tool"
)

// ReadOnlyToolset is the fixed whitelist of allowed tools for read-only subagents.
var ReadOnlyToolset = []string{
	"read", "glob", "grep", "ls", "think", "memory_read",
}

// writeToolNames is the set of tools that must never appear in a read-only registry.
var writeToolNames = map[string]bool{
	"write": true, "edit": true, "bash": true, "patch": true,
	"memory_write": true, "web_fetch": true, "web_search": true,
	"delegate": true, "explore": true,
}

// ErrToolNotPermitted is returned when a non-whitelisted tool is called via ReadOnlyExecutor.
type ErrToolNotPermitted struct {
	Tool    string
	Allowed []string
}

func (e ErrToolNotPermitted) Error() string {
	return fmt.Sprintf("tool %q is not permitted in read-only subagent; allowed: %v", e.Tool, e.Allowed)
}

// ReadOnlyExecutor wraps a Registry and enforces the whitelist at runtime.
// This is layer 2 of defense-in-depth (layer 1 = tools not registered).
type ReadOnlyExecutor struct {
	whitelist map[string]bool
	registry  *tool.Registry
}

// NewReadOnlyExecutor creates an executor with the hardcoded whitelist.
// Panics if the registry contains any non-whitelisted write tool
// (defense against misconfiguration).
func NewReadOnlyExecutor(reg *tool.Registry) *ReadOnlyExecutor {
	wl := make(map[string]bool, len(ReadOnlyToolset))
	for _, name := range ReadOnlyToolset {
		wl[name] = true
	}

	// Panic if registry contains a write tool — defense against misconfiguration
	if reg != nil {
		for _, t := range reg.All() {
			if writeToolNames[t.Name()] {
				panic(fmt.Sprintf("ReadOnlyExecutor: registry contains non-permitted write tool %q; remove it before creating a read-only executor", t.Name()))
			}
		}
	}

	return &ReadOnlyExecutor{
		whitelist: wl,
		registry:  reg,
	}
}

// Execute checks the whitelist BEFORE calling the tool.
// A non-whitelisted tool returns ErrToolNotPermitted and is never executed.
func (e *ReadOnlyExecutor) Execute(ctx context.Context, name string, input json.RawMessage) (*tool.Result, error) {
	if !e.whitelist[name] {
		return nil, ErrToolNotPermitted{Tool: name, Allowed: ReadOnlyToolset}
	}

	if e.registry == nil {
		return tool.ErrorResult("no tools available"), nil
	}

	t, ok := e.registry.Get(name)
	if !ok {
		return tool.ErrorResult(fmt.Sprintf("tool %q not found in registry", name)), nil
	}

	return t.Execute(ctx, input)
}
