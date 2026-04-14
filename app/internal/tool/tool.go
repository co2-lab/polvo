// Package tool defines the tool abstraction for agentic execution.
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/co2-lab/polvo/internal/git"
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

// RegistryOptions configures optional tools in DefaultRegistry.
type RegistryOptions struct {
	BraveAPIKey    string
	ExtraBlocklist []string
	Ignore         Ignorer        // optional .polvoignore set
	SubAgent       SubAgentRunner // optional: enables the delegate tool
	Explore        ExploreRunner  // optional: enables the explore tool (only at delegate level 0)
	Cache          *ToolCache     // optional: shared result cache for read, glob, grep, ls
	BashSession    *BashSession   // optional: persistent bash session; when set the bash tool runs inside it
	GitClient      git.Client     // optional: enables the diff tool
		GitPath        string         // optional: git repo path, defaults to workdir
}

// DefaultRegistry creates a registry with all built-in tools.
// Pass opts to enable optional tools (web_search) and extra blocklist entries.
func DefaultRegistry(workdir string, opts ...RegistryOptions) *Registry {
	var o RegistryOptions
	if len(opts) > 0 {
		o = opts[0]
	}

	r := NewRegistry()
	r.Register(SecretsMaskingMiddleware(NewReadWithCache(workdir, o.Ignore, o.Cache)))
	r.Register(NewWriteWithCache(workdir, o.Ignore, o.Cache))
	r.Register(NewEditToolWithCache(workdir, o.Ignore, o.Cache))
	if o.BashSession != nil {
		r.Register(NewBashWithSession(workdir, o.ExtraBlocklist, nil, o.BashSession))
	} else {
		r.Register(NewBash(workdir, o.ExtraBlocklist...))
	}
	r.Register(NewGlobWithCache(workdir, o.Cache))
	r.Register(NewGrepWithCache(workdir, o.Cache))
	r.Register(NewLSWithCache(workdir, o.Cache))
	r.Register(NewThink())
	r.Register(NewPatchTool(workdir))
	r.Register(SecretsMaskingMiddleware(NewWebFetch()))
	r.Register(SecretsMaskingMiddleware(NewWebSearch(o.BraveAPIKey)))
	if o.SubAgent != nil {
		r.Register(NewDelegate(o.SubAgent))
	}
	if o.Explore != nil {
		r.Register(NewExploreTool(o.Explore))
	}
	if o.GitClient != nil {
		r.Register(NewDiff(o.GitClient))
	}
	return r
}

// resolvePath resolves a relative path against workdir and rejects traversal.
func resolvePath(workdir, path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	return securePath(workdir, path)
}
