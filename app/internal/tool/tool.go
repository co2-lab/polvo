// Package tool defines the tool abstraction for agentic execution.
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// Tool is a capability that an LLM can invoke.
type Tool interface {
	Name() string
	Description() string
	InputSchema() json.RawMessage
	Execute(ctx context.Context, input json.RawMessage) (*Result, error)
}

// Result is the output of a tool execution.
type Result struct {
	Content string
	IsError bool
}

// ErrorResult creates an error result.
func ErrorResult(msg string) *Result {
	return &Result{Content: msg, IsError: true}
}

// Registry holds available tools.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry.
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// All returns all registered tools.
func (r *Registry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tools := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	return tools
}

// DefaultRegistry creates a registry with all built-in tools.
func DefaultRegistry(workdir string) *Registry {
	r := NewRegistry()
	r.Register(NewRead(workdir))
	r.Register(NewWrite(workdir))
	r.Register(NewEditTool(workdir))
	r.Register(NewBash(workdir))
	r.Register(NewGlob(workdir))
	r.Register(NewGrep(workdir))
	r.Register(NewLS(workdir))
	return r
}

// resolvePath resolves a relative path against workdir and rejects traversal.
func resolvePath(workdir, path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	return securePath(workdir, path)
}
