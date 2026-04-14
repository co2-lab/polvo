package browser

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/co2-lab/polvo/internal/tool"
)

// BrowserTool implements tool.Tool by delegating to a @playwright/mcp subprocess
// over stdio using the MCP JSON-RPC protocol.
type BrowserTool struct {
	cfg       Config
	allowlist *URLAllowlist
	session   *BrowserSession

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
	msgID  atomic.Int64
}

// BrowserInput is the structured input accepted by the browser tool.
type BrowserInput struct {
	Action    string `json:"action"`              // navigate|snapshot|click|type|fill|scroll|close
	URL       string `json:"url,omitempty"`       // for navigate
	Ref       string `json:"ref,omitempty"`       // element reference for click/type/fill
	Text      string `json:"text,omitempty"`      // for type/fill
	Direction string `json:"direction,omitempty"` // for scroll: "up"|"down"
}

// mcpRequest is a JSON-RPC 2.0 request sent to the playwright-mcp subprocess.
type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// mcpResponse is a JSON-RPC 2.0 response from the playwright-mcp subprocess.
type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NewBrowserTool creates a BrowserTool with the given config.
// The playwright-mcp process is not started until the first action.
func NewBrowserTool(cfg Config) *BrowserTool {
	return &BrowserTool{
		cfg:       cfg,
		allowlist: NewURLAllowlist(cfg.AllowedDomains),
		session:   &BrowserSession{},
	}
}

// Name returns the tool name.
func (t *BrowserTool) Name() string { return "browser" }

// Description returns a human-readable description.
func (t *BrowserTool) Description() string {
	return "Control a browser via Playwright MCP. Supports navigate, snapshot (accessibility tree), click, type, fill, scroll, and close. Requires Node.js and @playwright/mcp to be installed."
}

// InputSchema returns the JSON Schema for BrowserInput.
func (t *BrowserTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": ["navigate", "snapshot", "click", "type", "fill", "scroll", "close"],
				"description": "Browser action to perform"
			},
			"url": {
				"type": "string",
				"description": "URL to navigate to (required for navigate)"
			},
			"ref": {
				"type": "string",
				"description": "Element reference from a previous snapshot (for click/type/fill)"
			},
			"text": {
				"type": "string",
				"description": "Text to type or fill into a field"
			},
			"direction": {
				"type": "string",
				"enum": ["up", "down"],
				"description": "Scroll direction"
			}
		},
		"required": ["action"]
	}`)
}

// Execute performs the requested browser action.
func (t *BrowserTool) Execute(ctx context.Context, input json.RawMessage) (*tool.Result, error) {
	if !t.cfg.Enabled {
		return tool.ErrorResult("browser tool is disabled; set tools.browser.enabled: true in polvo.yaml"), nil
	}

	var in BrowserInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.ErrorResult("invalid input: " + err.Error()), nil
	}

	// Block denied actions.
	switch in.Action {
	case "file_upload":
		if t.cfg.Security.BlockFileUpload {
			return tool.ErrorResult("browser_file_upload is disabled by security policy"), nil
		}
	case "evaluate":
		if t.cfg.Security.BlockJSEval {
			return tool.ErrorResult("browser_evaluate is disabled by security policy"), nil
		}
	}

	// For navigate: enforce URL allowlist.
	if in.Action == "navigate" {
		if in.URL == "" {
			return tool.ErrorResult("navigate requires a url"), nil
		}
		if t.cfg.Security.URLAllowlist && !t.allowlist.IsAllowed(in.URL) {
			return tool.ErrorResult(fmt.Sprintf(
				"URL not allowed: %q — only URLs that have appeared in the conversation may be navigated to; add extra domains via tools.browser.allowed_domains",
				in.URL,
			)), nil
		}
	}

	// Ensure playwright-mcp is running.
	t.mu.Lock()
	if t.cmd == nil || t.cmd.ProcessState != nil {
		if err := t.startProcess(); err != nil {
			t.mu.Unlock()
			return tool.ErrorResult("could not start playwright-mcp: " + err.Error()), nil
		}
	}
	t.mu.Unlock()

	// Acquire exclusive session.
	if err := t.session.Acquire(); err != nil {
		return tool.ErrorResult("browser session already active: " + err.Error()), nil
	}
	defer func() {
		if in.Action == "close" {
			t.session.Release()
		} else {
			t.session.Release()
		}
	}()

	// Build the playwright-mcp tool name and args.
	playwrightTool, args, err := buildPlaywrightCall(in)
	if err != nil {
		return tool.ErrorResult(err.Error()), nil
	}

	// Set per-call timeout.
	timeoutMS := t.cfg.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = 30000
	}
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
	defer cancel()

	result, err := t.callPlaywright(callCtx, playwrightTool, args)
	if err != nil {
		return tool.ErrorResult("playwright-mcp error: " + err.Error()), nil
	}

	content := string(result)

	// Injection detection on snapshot results.
	if in.Action == "snapshot" && t.cfg.Security.InjectionDetection {
		if detected, pattern := DetectPromptInjection(content); detected {
			return tool.ErrorResult(fmt.Sprintf(
				"[SECURITY] Potential prompt injection detected in page content (pattern: %q). Snapshot content blocked.",
				pattern,
			)), nil
		}
	}

	return &tool.Result{Content: content}, nil
}

// buildPlaywrightCall maps a BrowserInput to a playwright-mcp tool name and args JSON.
func buildPlaywrightCall(in BrowserInput) (string, json.RawMessage, error) {
	switch in.Action {
	case "navigate":
		args, _ := json.Marshal(map[string]string{"url": in.URL})
		return "browser_navigate", args, nil
	case "snapshot":
		return "browser_snapshot", json.RawMessage(`{}`), nil
	case "click":
		if in.Ref == "" {
			return "", nil, fmt.Errorf("click requires a ref")
		}
		args, _ := json.Marshal(map[string]string{"ref": in.Ref})
		return "browser_click", args, nil
	case "type":
		if in.Ref == "" {
			return "", nil, fmt.Errorf("type requires a ref")
		}
		args, _ := json.Marshal(map[string]string{"ref": in.Ref, "text": in.Text})
		return "browser_type", args, nil
	case "fill":
		if in.Ref == "" {
			return "", nil, fmt.Errorf("fill requires a ref")
		}
		args, _ := json.Marshal(map[string]string{"ref": in.Ref, "value": in.Text})
		return "browser_fill", args, nil
	case "scroll":
		dir := in.Direction
		if dir == "" {
			dir = "down"
		}
		args, _ := json.Marshal(map[string]string{"direction": dir})
		return "browser_scroll", args, nil
	case "close":
		return "browser_close", json.RawMessage(`{}`), nil
	default:
		return "", nil, fmt.Errorf("unknown action: %q", in.Action)
	}
}

// startProcess launches the @playwright/mcp Node.js subprocess.
// Must be called with t.mu held.
func (t *BrowserTool) startProcess() error {
	npx, err := exec.LookPath("npx")
	if err != nil {
		return fmt.Errorf("npx not found in PATH; please install Node.js 18+ and run: npx @playwright/mcp --version")
	}

	args := []string{"@playwright/mcp"}
	if t.cfg.Mode == "vision" {
		args = append(args, "--vision")
	} else {
		args = append(args, "--snapshot-mode")
	}
	if t.cfg.Context == "persistent" {
		// Playwright MCP uses a user-data-dir for persistence.
		args = append(args, "--user-data-dir=.polvo/browser-profile")
	}
	if t.cfg.StorageState != "" {
		args = append(args, "--storage-state="+t.cfg.StorageState)
	}

	cmd := exec.Command(npx, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("creating stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("creating stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting playwright-mcp: %w", err)
	}

	t.cmd = cmd
	t.stdin = stdin
	t.stdout = bufio.NewScanner(stdoutPipe)

	return nil
}

// callPlaywright sends an MCP tool/call request to the subprocess and returns the result.
func (t *BrowserTool) callPlaywright(ctx context.Context, toolName string, args json.RawMessage) (json.RawMessage, error) {
	id := t.msgID.Add(1)

	params, err := json.Marshal(map[string]interface{}{
		"name":      toolName,
		"arguments": args,
	})
	if err != nil {
		return nil, fmt.Errorf("marshalling params: %w", err)
	}

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "tools/call",
		Params:  params,
	}

	line, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshalling request: %w", err)
	}

	t.mu.Lock()
	_, writeErr := fmt.Fprintf(t.stdin, "%s\n", line)
	t.mu.Unlock()
	if writeErr != nil {
		return nil, fmt.Errorf("writing to playwright-mcp stdin: %w", writeErr)
	}

	// Read response lines until we find the matching ID, respecting ctx deadline.
	type scanResult struct {
		resp mcpResponse
		err  error
	}
	ch := make(chan scanResult, 1)

	go func() {
		for t.stdout.Scan() {
			raw := t.stdout.Bytes()
			var resp mcpResponse
			if jsonErr := json.Unmarshal(raw, &resp); jsonErr != nil {
				continue // skip non-JSON lines (e.g. stderr-redirected logs)
			}
			if resp.ID == id {
				ch <- scanResult{resp: resp}
				return
			}
		}
		if scanErr := t.stdout.Err(); scanErr != nil {
			ch <- scanResult{err: fmt.Errorf("reading playwright-mcp stdout: %w", scanErr)}
		} else {
			ch <- scanResult{err: fmt.Errorf("playwright-mcp stdout closed unexpectedly")}
		}
	}()

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("playwright-mcp call timed out: %w", ctx.Err())
	case sr := <-ch:
		if sr.err != nil {
			return nil, sr.err
		}
		if sr.resp.Error != nil {
			return nil, fmt.Errorf("playwright-mcp returned error %d: %s", sr.resp.Error.Code, sr.resp.Error.Message)
		}
		return sr.resp.Result, nil
	}
}

// AddURLsFromText seeds the URL allowlist from conversation text.
// Call this with each user message and prior tool results.
func (t *BrowserTool) AddURLsFromText(text string) {
	t.allowlist.AddFromText(text)
}

// Close shuts down the playwright-mcp subprocess if it is running.
func (t *BrowserTool) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cmd != nil && t.cmd.ProcessState == nil {
		_ = t.stdin.Close()
		return t.cmd.Wait()
	}
	return nil
}
