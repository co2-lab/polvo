package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// defaultBackoff is the default exponential backoff schedule for reconnects.
var defaultBackoff = []time.Duration{
	100 * time.Millisecond,
	500 * time.Millisecond,
	2 * time.Second,
	5 * time.Second,
}

// Capabilities holds the cached tool list and metadata from a connected MCP server.
type Capabilities struct {
	Tools    []ToolDefinition
	Version  string
	CachedAt time.Time
}

// ResilientClient wraps an MCPConnection with automatic reconnection, a
// capability cache, and an optional periodic health checker.
type ResilientClient struct {
	mu         sync.RWMutex
	inner      *MCPConnection
	factory    func() (*MCPConnection, error)
	maxRetries int
	backoff    []time.Duration
	caps       Capabilities
}

// ResilientClientOption is a functional option for NewResilientClient.
type ResilientClientOption func(*ResilientClient)

// WithMaxRetries sets the maximum number of reconnect attempts per call.
func WithMaxRetries(n int) ResilientClientOption {
	return func(r *ResilientClient) { r.maxRetries = n }
}

// WithBackoff sets the backoff schedule used between reconnect attempts.
func WithBackoff(b []time.Duration) ResilientClientOption {
	return func(r *ResilientClient) { r.backoff = b }
}

// NewResilientClient creates a ResilientClient around a factory function that
// produces fresh MCPConnections. factory is called once immediately (to
// establish the initial connection) and again on each reconnect.
func NewResilientClient(factory func() (*MCPConnection, error), opts ...ResilientClientOption) (*ResilientClient, error) {
	r := &ResilientClient{
		factory:    factory,
		maxRetries: 3,
		backoff:    defaultBackoff,
	}
	for _, o := range opts {
		o(r)
	}

	conn, err := factory()
	if err != nil {
		return nil, fmt.Errorf("mcp resilient: initial connect: %w", err)
	}
	r.inner = conn
	r.refreshCaps()
	return r, nil
}

// Call invokes a tool, retrying with reconnection on connection errors.
func (r *ResilientClient) Call(ctx context.Context, toolName string, args json.RawMessage) (json.RawMessage, error) {
	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		r.mu.RLock()
		conn := r.inner
		r.mu.RUnlock()

		result, err := conn.Call(ctx, toolName, args)
		if err == nil {
			return result, nil
		}
		if !isConnectionError(err) {
			return nil, err
		}

		slog.Warn("mcp resilient: connection error, reconnecting",
			"tool", toolName, "attempt", attempt+1, "error", err)

		if reconnErr := r.reconnect(ctx); reconnErr != nil {
			return nil, fmt.Errorf("mcp resilient: reconnect failed: %w", reconnErr)
		}

		if attempt < len(r.backoff) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(r.backoff[attempt]):
			}
		}
	}
	return nil, fmt.Errorf("mcp resilient: call %q failed after %d retries", toolName, r.maxRetries)
}

// ListTools returns the filtered tool list from the inner connection.
// Falls back to the capability cache if the connection is not ready.
func (r *ResilientClient) ListTools() ([]ToolDefinition, error) {
	r.mu.RLock()
	conn := r.inner
	r.mu.RUnlock()

	tools, err := conn.ListTools()
	if err == nil {
		return tools, nil
	}
	if !isConnectionError(err) {
		return nil, err
	}

	// Return cached capabilities on connection error.
	r.mu.RLock()
	cached := r.caps.Tools
	r.mu.RUnlock()

	if cached != nil {
		slog.Warn("mcp resilient: ListTools falling back to cache", "error", err)
		return cached, nil
	}
	return nil, err
}

// Capabilities returns the cached capability snapshot.
// The cache is refreshed on every successful (re)connection.
func (r *ResilientClient) Capabilities() Capabilities {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.caps
}

// HasTool reports whether the cached capability set includes a tool with the
// given name.
func (r *ResilientClient) HasTool(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, t := range r.caps.Tools {
		if t.Name == name {
			return true
		}
	}
	return false
}

// Ping sends a tools/list request to check that the server is alive.
// It is used by the HealthChecker.
func (r *ResilientClient) Ping(ctx context.Context) error {
	r.mu.RLock()
	conn := r.inner
	r.mu.RUnlock()

	_, err := conn.ListTools()
	return err
}

// Disconnect closes the underlying connection.
func (r *ResilientClient) Disconnect() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.inner.Disconnect()
}

// reconnect replaces the inner connection with a freshly connected one.
func (r *ResilientClient) reconnect(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Clean up the dead connection.
	_ = r.inner.Disconnect()

	conn, err := r.factory()
	if err != nil {
		return err
	}
	r.inner = conn
	r.refreshCapsLocked()
	return nil
}

// refreshCaps fetches and caches capabilities, acquiring the write lock.
func (r *ResilientClient) refreshCaps() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refreshCapsLocked()
}

// refreshCapsLocked fetches and caches capabilities; caller must hold r.mu
// (write lock).
func (r *ResilientClient) refreshCapsLocked() {
	tools, err := r.inner.ListTools()
	if err != nil {
		slog.Warn("mcp resilient: capability refresh failed", "error", err)
		return
	}
	r.caps = Capabilities{
		Tools:    tools,
		CachedAt: time.Now(),
	}
}

// isConnectionError reports whether err indicates a broken transport
// (pipe closed, EOF, process exited, or "not connected" sentinel).
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, marker := range []string{
		"not connected",
		"broken pipe",
		"connection closed",
		"connection reset",
		"EOF",
		"closed pipe",
		"write request",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	// io.EOF and io.ErrClosedPipe
	if err == io.EOF || err == io.ErrClosedPipe {
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// HealthChecker
// ---------------------------------------------------------------------------

// HealthChecker periodically pings a ResilientClient and calls onDead when
// the server is unreachable.
type HealthChecker struct {
	client   *ResilientClient
	interval time.Duration
	timeout  time.Duration
	onDead   func()
}

// NewHealthChecker creates a HealthChecker for the given client.
//
//   - interval: how often to ping the server.
//   - timeout:  per-ping context deadline.
//   - onDead:   called (in the background goroutine) when a ping fails.
func NewHealthChecker(client *ResilientClient, interval, timeout time.Duration, onDead func()) *HealthChecker {
	return &HealthChecker{
		client:   client,
		interval: interval,
		timeout:  timeout,
		onDead:   onDead,
	}
}

// Start launches the health-check loop in a background goroutine.
// The loop exits when ctx is cancelled.
func (h *HealthChecker) Start(ctx context.Context) {
	go func() {
		t := time.NewTicker(h.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				hctx, cancel := context.WithTimeout(ctx, h.timeout)
				err := h.client.Ping(hctx)
				cancel()
				if err != nil {
					slog.Warn("mcp health: ping failed", "error", err)
					h.onDead()
				}
			}
		}
	}()
}
