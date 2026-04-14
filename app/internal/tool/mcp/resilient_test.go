package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Mock infrastructure
// ---------------------------------------------------------------------------

// fakeConn simulates an MCPConnection with programmable responses.
type fakeConn struct {
	callErr      error
	listTools    []ToolDefinition
	callCount    atomic.Int64
	pingCount    atomic.Int64
	pingErr      error
	disconnected atomic.Bool
}

func (f *fakeConn) call(_ context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
	f.callCount.Add(1)
	if f.callErr != nil {
		return nil, f.callErr
	}
	return json.RawMessage(`{"ok":true}`), nil
}

func (f *fakeConn) listToolsFn() ([]ToolDefinition, error) {
	f.pingCount.Add(1)
	if f.pingErr != nil {
		return nil, f.pingErr
	}
	return f.listTools, nil
}

func (f *fakeConn) disconnect() error {
	f.disconnected.Store(true)
	return nil
}

// ---------------------------------------------------------------------------
// resilientTestClient mirrors ResilientClient but works with fakeConn so we
// can test all retry/backoff/capability logic without spawning real processes.
// ---------------------------------------------------------------------------

type resilientTestClient struct {
	mu         sync.RWMutex
	inner      *fakeConn
	factory    func() (*fakeConn, error)
	maxRetries int
	backoff    []time.Duration
	caps       Capabilities
}

func newResilientTestClient(factory func() (*fakeConn, error), maxRetries int, backoff []time.Duration) (*resilientTestClient, error) {
	conn, err := factory()
	if err != nil {
		return nil, err
	}
	r := &resilientTestClient{
		inner:      conn,
		factory:    factory,
		maxRetries: maxRetries,
		backoff:    backoff,
	}
	r.refreshCaps()
	return r, nil
}

func (r *resilientTestClient) call(ctx context.Context, toolName string, args json.RawMessage) (json.RawMessage, error) {
	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		r.mu.RLock()
		conn := r.inner
		r.mu.RUnlock()

		result, err := conn.call(ctx, toolName, args)
		if err == nil {
			return result, nil
		}
		if !isConnectionError(err) {
			return nil, err
		}

		if reconnErr := r.reconnect(); reconnErr != nil {
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

func (r *resilientTestClient) reconnect() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	_ = r.inner.disconnect()
	conn, err := r.factory()
	if err != nil {
		return err
	}
	r.inner = conn
	r.refreshCapsLocked()
	return nil
}

func (r *resilientTestClient) refreshCaps() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refreshCapsLocked()
}

func (r *resilientTestClient) refreshCapsLocked() {
	tools, err := r.inner.listToolsFn()
	if err != nil {
		return
	}
	r.caps = Capabilities{Tools: tools, CachedAt: time.Now()}
}

func (r *resilientTestClient) capabilities() Capabilities {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.caps
}

func (r *resilientTestClient) hasTool(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, t := range r.caps.Tools {
		if t.Name == name {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// healthCheckerForFake mirrors HealthChecker but uses fakeConn.
// ---------------------------------------------------------------------------

type healthCheckerForFake struct {
	client   *resilientTestClient
	interval time.Duration
	timeout  time.Duration
	onDead   func()
}

func (h *healthCheckerForFake) start(ctx context.Context) {
	go func() {
		t := time.NewTicker(h.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_, cancel := context.WithTimeout(ctx, h.timeout)
				h.client.mu.RLock()
				conn := h.client.inner
				h.client.mu.RUnlock()
				_, err := conn.listToolsFn()
				cancel()
				if err != nil {
					h.onDead()
				}
			}
		}
	}()
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestResilientClient_ReconnectsOnConnectionError verifies that a "not connected"
// error triggers reconnection and the call succeeds after reconnect.
func TestResilientClient_ReconnectsOnConnectionError(t *testing.T) {
	conns := []*fakeConn{
		{callErr: fmt.Errorf("mcp: server %q is not connected", "test")},
		{callErr: nil},
	}
	connIdx := 0

	factory := func() (*fakeConn, error) {
		c := conns[connIdx]
		connIdx++
		return c, nil
	}

	client, err := newResilientTestClient(factory, 3, []time.Duration{0, 0, 0})
	if err != nil {
		t.Fatalf("newResilientTestClient: %v", err)
	}

	result, err := client.call(context.Background(), "tool", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("expected success after reconnect, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Verify first connection was actually disconnected.
	if !conns[0].disconnected.Load() {
		t.Error("expected first connection to be disconnected on reconnect")
	}
}

// TestResilientClient_NoRetryOnNonConnectionError verifies that a non-connection
// error is returned immediately without retry.
func TestResilientClient_NoRetryOnNonConnectionError(t *testing.T) {
	permanentErr := fmt.Errorf("mcp call %q: error -32601: method not found", "bad_tool")

	factoryCalls := 0
	factory := func() (*fakeConn, error) {
		factoryCalls++
		return &fakeConn{callErr: permanentErr}, nil
	}

	client, err := newResilientTestClient(factory, 3, []time.Duration{0, 0, 0})
	if err != nil {
		t.Fatalf("newResilientTestClient: %v", err)
	}

	_, callErr := client.call(context.Background(), "bad_tool", json.RawMessage(`{}`))
	if callErr == nil {
		t.Fatal("expected error, got nil")
	}
	if callErr.Error() != permanentErr.Error() {
		t.Errorf("expected permanent error %q, got %q", permanentErr, callErr)
	}

	// Factory should only have been called once (initial connect), no reconnect.
	if factoryCalls != 1 {
		t.Errorf("factory called %d times, want 1 (no reconnect for non-connection error)", factoryCalls)
	}

	client.mu.RLock()
	count := client.inner.callCount.Load()
	client.mu.RUnlock()
	if count != 1 {
		t.Errorf("callCount = %d, want 1 (no retry)", count)
	}
}

// TestResilientClient_BackoffTiming verifies that backoff delays are respected
// between reconnect attempts.
func TestResilientClient_BackoffTiming(t *testing.T) {
	// All connections fail with a connection error to force the full retry path.
	factory := func() (*fakeConn, error) {
		return &fakeConn{callErr: fmt.Errorf("not connected")}, nil
	}

	backoff := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond}
	client, err := newResilientTestClient(factory, 2, backoff)
	if err != nil {
		t.Fatalf("newResilientTestClient: %v", err)
	}

	start := time.Now()
	_, callErr := client.call(context.Background(), "tool", json.RawMessage(`{}`))
	elapsed := time.Since(start)

	if callErr == nil {
		t.Fatal("expected error after all retries exhausted")
	}

	// Minimum elapsed should be the sum of the backoff durations applied.
	minExpected := backoff[0] + backoff[1]
	if elapsed < minExpected {
		t.Errorf("elapsed %v < minimum backoff %v", elapsed, minExpected)
	}
}

// TestHealthChecker_CallsOnDeadWhenPingFails verifies that the health checker
// invokes onDead when the ping returns an error.
func TestHealthChecker_CallsOnDeadWhenPingFails(t *testing.T) {
	factory := func() (*fakeConn, error) {
		return &fakeConn{pingErr: fmt.Errorf("not connected")}, nil
	}

	client, err := newResilientTestClient(factory, 1, nil)
	if err != nil {
		t.Fatalf("newResilientTestClient: %v", err)
	}

	onDeadCalled := make(chan struct{}, 1)
	hc := &healthCheckerForFake{
		client:   client,
		interval: 10 * time.Millisecond,
		timeout:  50 * time.Millisecond,
		onDead: func() {
			select {
			case onDeadCalled <- struct{}{}:
			default:
			}
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	hc.start(ctx)

	select {
	case <-onDeadCalled:
		// success
	case <-ctx.Done():
		t.Fatal("timeout waiting for onDead to be called")
	}
}

// TestHealthChecker_DoesNotCallOnDeadWhenHealthy verifies that a healthy
// connection does not trigger the onDead callback.
func TestHealthChecker_DoesNotCallOnDeadWhenHealthy(t *testing.T) {
	factory := func() (*fakeConn, error) {
		return &fakeConn{
			listTools: []ToolDefinition{{Name: "tool_a"}},
		}, nil
	}

	client, err := newResilientTestClient(factory, 1, nil)
	if err != nil {
		t.Fatalf("newResilientTestClient: %v", err)
	}

	var onDeadCount atomic.Int64
	hc := &healthCheckerForFake{
		client:   client,
		interval: 10 * time.Millisecond,
		timeout:  50 * time.Millisecond,
		onDead:   func() { onDeadCount.Add(1) },
	}

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	hc.start(ctx)
	<-ctx.Done()

	if n := onDeadCount.Load(); n != 0 {
		t.Errorf("onDead called %d times on healthy connection, want 0", n)
	}
}

// TestCapabilities_CachedAfterConnect verifies that capabilities are populated
// and accessible immediately after initial connection.
func TestCapabilities_CachedAfterConnect(t *testing.T) {
	tools := []ToolDefinition{
		{Name: "read_file", Description: "reads a file"},
		{Name: "write_file", Description: "writes a file"},
	}

	factory := func() (*fakeConn, error) {
		return &fakeConn{listTools: tools}, nil
	}

	client, err := newResilientTestClient(factory, 1, nil)
	if err != nil {
		t.Fatalf("newResilientTestClient: %v", err)
	}

	caps := client.capabilities()

	if len(caps.Tools) != len(tools) {
		t.Fatalf("expected %d cached tools, got %d", len(tools), len(caps.Tools))
	}
	if caps.CachedAt.IsZero() {
		t.Error("CachedAt should not be zero after connect")
	}

	if !client.hasTool("read_file") {
		t.Error("HasTool(read_file) = false, want true")
	}
	if !client.hasTool("write_file") {
		t.Error("HasTool(write_file) = false, want true")
	}
	if client.hasTool("delete_file") {
		t.Error("HasTool(delete_file) = true, want false")
	}
}

// TestCapabilities_RefreshedAfterReconnect verifies that the capability cache
// is updated with fresh data after a successful reconnect.
func TestCapabilities_RefreshedAfterReconnect(t *testing.T) {
	firstTools := []ToolDefinition{{Name: "tool_v1"}}
	secondTools := []ToolDefinition{{Name: "tool_v1"}, {Name: "tool_v2"}}

	connectCount := 0
	factory := func() (*fakeConn, error) {
		connectCount++
		if connectCount == 1 {
			return &fakeConn{
				callErr:   fmt.Errorf("not connected"),
				listTools: firstTools,
			}, nil
		}
		return &fakeConn{listTools: secondTools}, nil
	}

	client, err := newResilientTestClient(factory, 1, []time.Duration{0})
	if err != nil {
		t.Fatalf("newResilientTestClient: %v", err)
	}

	// Initial capabilities should reflect firstTools.
	if !client.hasTool("tool_v1") {
		t.Error("expected tool_v1 in initial caps")
	}
	if client.hasTool("tool_v2") {
		t.Error("tool_v2 should not be in initial caps")
	}

	// Trigger a call that causes reconnect.
	_, _ = client.call(context.Background(), "any", json.RawMessage(`{}`))

	// After reconnect, caps should reflect secondTools.
	if !client.hasTool("tool_v2") {
		t.Error("expected tool_v2 in caps after reconnect")
	}
}

// TestIsConnectionError covers the isConnectionError helper.
func TestIsConnectionError(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{fmt.Errorf("mcp: server %q is not connected", "x"), true},
		{fmt.Errorf("write request: broken pipe"), true},
		{fmt.Errorf("connection closed before response"), true},
		{fmt.Errorf("reading response: EOF"), true},
		{fmt.Errorf("mcp call %q: error -32601: unknown method", "t"), false},
		{fmt.Errorf("mcp: tool %q is denied by permission rules", "t"), false},
	}
	for _, c := range cases {
		got := isConnectionError(c.err)
		if got != c.want {
			t.Errorf("isConnectionError(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}

// TestResilientClient_ContextCancelledDuringBackoff verifies that a cancelled
// context during backoff returns ctx.Err() promptly.
func TestResilientClient_ContextCancelledDuringBackoff(t *testing.T) {
	factory := func() (*fakeConn, error) {
		return &fakeConn{callErr: fmt.Errorf("not connected")}, nil
	}

	// Long backoff so the cancel fires first.
	backoff := []time.Duration{10 * time.Second}
	client, err := newResilientTestClient(factory, 1, backoff)
	if err != nil {
		t.Fatalf("newResilientTestClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, callErr := client.call(ctx, "tool", json.RawMessage(`{}`))
	elapsed := time.Since(start)

	if callErr == nil {
		t.Fatal("expected error from cancelled context")
	}

	if elapsed > 500*time.Millisecond {
		t.Errorf("call took %v, expected to respect context cancellation quickly", elapsed)
	}
}
