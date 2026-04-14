package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ErrPermissionRequired is returned by Hub.Call when a tool requires user
// approval before execution (PermAsk level and no approval has been granted).
var ErrPermissionRequired = errors.New("mcp: permission required for tool")

// MCPHub manages multiple MCP server connections, each wrapped in a
// ResilientClient that provides automatic reconnection, capability caching,
// and health monitoring.
type MCPHub struct {
	connections map[string]*ResilientClient
	cfg         *MCPConfig
	permissions *PermissionEngine
	mu          sync.RWMutex
}

// NewMCPHub creates an MCPHub from a loaded MCPConfig.
func NewMCPHub(cfg *MCPConfig) *MCPHub {
	return &MCPHub{
		connections: make(map[string]*ResilientClient),
		cfg:         cfg,
		permissions: NewPermissionEngine(cfg.Permissions),
	}
}

// Start connects to all enabled servers in the config.
// Each connection is wrapped in a ResilientClient for automatic reconnection.
// Individual server failures are non-fatal: a warning is logged and the hub
// continues starting the remaining servers.
func (h *MCPHub) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	for name, srv := range h.cfg.MCPServers {
		if srv.Disabled {
			slog.Info("mcp: server disabled, skipping", "server", name)
			continue
		}

		srv := srv // capture for closure
		factory := func() (*MCPConnection, error) {
			conn := NewMCPConnection(srv)
			if err := conn.Connect(ctx); err != nil {
				return nil, err
			}
			return conn, nil
		}

		rc, err := NewResilientClient(factory)
		if err != nil {
			slog.Warn("mcp: failed to connect to server", "server", name, "error", err)
			continue
		}
		h.connections[name] = rc
		slog.Info("mcp: server connected", "server", name,
			"tools", len(rc.Capabilities().Tools))
	}
	return nil
}

// Tools returns all tools from all connected servers with namespaced names.
// For each server the capability cache is consulted first; a live tools/list
// is issued as a fallback if the cache is empty.
func (h *MCPHub) Tools() []ToolDefinition {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var all []ToolDefinition
	for serverName, rc := range h.connections {
		tools, err := rc.ListTools()
		if err != nil {
			slog.Warn("mcp: listing tools failed", "server", serverName, "error", err)
			continue
		}
		for _, t := range tools {
			namespaced := ToolDefinition{
				Name:        NamespacedToolName(serverName, t.Name),
				Description: t.Description,
				InputSchema: t.InputSchema,
			}
			all = append(all, namespaced)
		}
	}
	return all
}

// Call invokes a namespaced MCP tool after checking permissions.
// The underlying ResilientClient will automatically reconnect on transient
// connection errors before returning a failure to the caller.
//
// Permission outcomes:
//   - PermDeny  → returns an error
//   - PermAsk   → returns ErrPermissionRequired (caller must handle approval UI)
//   - PermAllow → invokes the tool and returns the result
func (h *MCPHub) Call(ctx context.Context, toolName string, args json.RawMessage) (json.RawMessage, error) {
	level := h.permissions.Evaluate(toolName)
	switch level {
	case PermDeny:
		return nil, fmt.Errorf("mcp: tool %q is denied by permission rules", toolName)
	case PermAsk:
		return nil, fmt.Errorf("%w: %s", ErrPermissionRequired, toolName)
	}

	serverName, rawToolName, ok := ParseNamespacedTool(toolName)
	if !ok {
		return nil, fmt.Errorf("mcp: %q is not a valid namespaced tool name", toolName)
	}

	h.mu.RLock()
	rc, exists := h.connections[serverName]
	h.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("mcp: no connected server %q", serverName)
	}

	return rc.Call(ctx, rawToolName, args)
}

// Stop disconnects all servers.
func (h *MCPHub) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for name, rc := range h.connections {
		if err := rc.Disconnect(); err != nil {
			slog.Warn("mcp: error disconnecting server", "server", name, "error", err)
		}
	}
	h.connections = make(map[string]*ResilientClient)
}

// StartHealthChecks launches a HealthChecker for every connected server.
// onDead is called whenever a server fails a ping; the hub does not
// automatically remove the server — callers can trigger a reload if desired.
// The returned stop function cancels all health-check goroutines.
func (h *MCPHub) StartHealthChecks(ctx context.Context, onDead func(serverName string)) context.CancelFunc {
	hctx, cancel := context.WithCancel(ctx)

	h.mu.RLock()
	defer h.mu.RUnlock()

	for name, rc := range h.connections {
		name, rc := name, rc // capture
		hc := NewHealthChecker(rc,
			30*time.Second,
			5*time.Second,
			func() {
				slog.Warn("mcp: server health check failed", "server", name)
				if onDead != nil {
					onDead(name)
				}
			},
		)
		hc.Start(hctx)
	}
	return cancel
}
