package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"sync"
	"sync/atomic"
)

// ToolDefinition represents a tool exposed by an MCP server.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ConnectionState describes the lifecycle state of an MCP connection.
type ConnectionState string

const (
	StateConnecting   ConnectionState = "connecting"
	StateConnected    ConnectionState = "connected"
	StateDisconnected ConnectionState = "disconnected"
)

// jsonRPCRequest is a JSON-RPC 2.0 request message.
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// jsonRPCResponse is a JSON-RPC 2.0 response message.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

// jsonRPCError holds a JSON-RPC error detail.
type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCPConnection manages a single MCP server connection (stdio transport).
type MCPConnection struct {
	cfg    MCPServerConfig
	state  ConnectionState
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
	tools  []ToolDefinition
	mu     sync.Mutex
	msgID  atomic.Int64
}

// NewMCPConnection creates a new MCPConnection for the given server config.
func NewMCPConnection(cfg MCPServerConfig) *MCPConnection {
	return &MCPConnection{
		cfg:   cfg,
		state: StateDisconnected,
	}
}

// Connect starts the subprocess, performs the MCP handshake, and lists available tools.
// Sets state to StateConnected on success.
func (c *MCPConnection) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.state = StateConnecting

	// Build command with expanded environment.
	args := c.cfg.Args
	cmd := exec.CommandContext(ctx, c.cfg.Command, args...)

	// Merge custom env with process env (already expanded by LoadMCPConfig).
	if len(c.cfg.Env) > 0 {
		extra := make([]string, 0, len(c.cfg.Env))
		for k, v := range c.cfg.Env {
			extra = append(extra, k+"="+v)
		}
		cmd.Env = append(cmd.Environ(), extra...)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		c.state = StateDisconnected
		return fmt.Errorf("mcp connect: stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		c.state = StateDisconnected
		return fmt.Errorf("mcp connect: stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		c.state = StateDisconnected
		return fmt.Errorf("mcp connect: start process: %w", err)
	}

	c.cmd = cmd
	c.stdin = stdin
	c.stdout = bufio.NewScanner(stdoutPipe)

	// MCP initialize handshake.
	initParams := json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"polvo","version":"0.1"}}`)
	resp, err := c.sendRequest("initialize", initParams)
	if err != nil {
		_ = c.disconnectLocked()
		return fmt.Errorf("mcp connect: initialize: %w", err)
	}
	if resp.Error != nil {
		_ = c.disconnectLocked()
		return fmt.Errorf("mcp connect: initialize error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	// Send initialized notification (no ID = notification).
	if err := c.sendNotification("notifications/initialized", nil); err != nil {
		_ = c.disconnectLocked()
		return fmt.Errorf("mcp connect: initialized notification: %w", err)
	}

	// List tools.
	tools, err := c.listToolsLocked()
	if err != nil {
		_ = c.disconnectLocked()
		return fmt.Errorf("mcp connect: list tools: %w", err)
	}
	c.tools = tools
	c.state = StateConnected
	return nil
}

// ListTools returns the tools available from this server (namespaced).
// Applies FilterTools regex if set.
func (c *MCPConnection) ListTools() ([]ToolDefinition, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state != StateConnected {
		return nil, fmt.Errorf("mcp: server %q is not connected", c.cfg.Name)
	}
	return c.applyFilter(c.tools), nil
}

// Call invokes a tool on the MCP server.
func (c *MCPConnection) Call(ctx context.Context, toolName string, args json.RawMessage) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state != StateConnected {
		return nil, fmt.Errorf("mcp: server %q is not connected", c.cfg.Name)
	}

	type callParams struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	params, err := json.Marshal(callParams{Name: toolName, Arguments: args})
	if err != nil {
		return nil, fmt.Errorf("mcp call: marshal params: %w", err)
	}

	resp, err := c.sendRequest("tools/call", params)
	if err != nil {
		return nil, fmt.Errorf("mcp call %q: %w", toolName, err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("mcp call %q: error %d: %s", toolName, resp.Error.Code, resp.Error.Message)
	}
	return resp.Result, nil
}

// Disconnect kills the subprocess gracefully.
func (c *MCPConnection) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.disconnectLocked()
}

// disconnectLocked performs disconnect while the caller already holds c.mu.
func (c *MCPConnection) disconnectLocked() error {
	c.state = StateDisconnected
	if c.stdin != nil {
		_ = c.stdin.Close()
		c.stdin = nil
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		_ = c.cmd.Wait()
		c.cmd = nil
	}
	return nil
}

// sendRequest writes a JSON-RPC request and reads the response line.
// Must be called with c.mu held.
func (c *MCPConnection) sendRequest(method string, params json.RawMessage) (*jsonRPCResponse, error) {
	id := c.msgID.Add(1)
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  method,
		Params:  params,
	}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	if _, err := c.stdin.Write(data); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	// Read lines until we find one that is a response (has an "id" field matching ours).
	for c.stdout.Scan() {
		line := c.stdout.Bytes()
		if len(line) == 0 {
			continue
		}
		var resp jsonRPCResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			continue // skip non-JSON or notification lines
		}
		if resp.ID == id {
			return &resp, nil
		}
		// Otherwise it's a notification; skip it.
	}
	if err := c.stdout.Err(); err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	return nil, fmt.Errorf("connection closed before response for method %q", method)
}

// sendNotification writes a JSON-RPC notification (no ID).
// Must be called with c.mu held.
func (c *MCPConnection) sendNotification(method string, params json.RawMessage) error {
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = c.stdin.Write(data)
	return err
}

// listToolsLocked fetches the tool list from the server.
// Must be called with c.mu held.
func (c *MCPConnection) listToolsLocked() ([]ToolDefinition, error) {
	resp, err := c.sendRequest("tools/list", json.RawMessage(`{}`))
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("tools/list error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	var result struct {
		Tools []ToolDefinition `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse tools/list response: %w", err)
	}
	return result.Tools, nil
}

// applyFilter applies the FilterTools regex to the tool list.
// If FilterTools is empty, all tools are returned unchanged.
func (c *MCPConnection) applyFilter(tools []ToolDefinition) []ToolDefinition {
	if c.cfg.FilterTools == "" {
		return tools
	}
	re, err := regexp.Compile(c.cfg.FilterTools)
	if err != nil {
		return tools
	}
	filtered := tools[:0:0]
	for _, t := range tools {
		if re.MatchString(t.Name) {
			filtered = append(filtered, t)
		}
	}
	return filtered
}
