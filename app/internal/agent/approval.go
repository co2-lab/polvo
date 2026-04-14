package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ApprovalRequest is sent to the PermissionCallback before executing a tool
// that requires user approval.
type ApprovalRequest struct {
	AgentName string
	SessionID string
	ToolName  string
	ToolInput json.RawMessage
	RiskLevel string // "low" | "medium" | "high" | "critical"
	Preview   string // human-readable description of what will happen
}

// ApprovalDecision is the callback's response.
type ApprovalDecision int

const (
	ApprovalAllow ApprovalDecision = iota
	ApprovalDeny
)

// PermissionCallback is called before executing any tool that has "ask" permission.
// If nil, ask-permission tools are allowed by default (backward compat).
type PermissionCallback interface {
	RequestApproval(ctx context.Context, req ApprovalRequest) (ApprovalDecision, error)
}

// AutoApproveCallback always approves. Used for AutonomyFull mode.
type AutoApproveCallback struct{}

func (AutoApproveCallback) RequestApproval(_ context.Context, _ ApprovalRequest) (ApprovalDecision, error) {
	return ApprovalAllow, nil
}

// AutoDenyCallback always denies. Useful for dry-run/plan mode.
type AutoDenyCallback struct{}

func (AutoDenyCallback) RequestApproval(_ context.Context, _ ApprovalRequest) (ApprovalDecision, error) {
	return ApprovalDeny, nil
}

// ChannelCallback sends approval requests over a channel and waits for a response.
// Designed for WebSocket/TUI integration.
type ChannelCallback struct {
	Requests  chan<- ApprovalRequest
	Responses <-chan ApprovalDecision
	Timeout   time.Duration // default 60s; 0 = no timeout
}

func (c *ChannelCallback) RequestApproval(ctx context.Context, req ApprovalRequest) (ApprovalDecision, error) {
	// Determine effective timeout.
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	// Send the request (with ctx + timeout cancellation).
	sendCtx, sendCancel := context.WithTimeout(ctx, timeout)
	defer sendCancel()

	select {
	case c.Requests <- req:
	case <-sendCtx.Done():
		return ApprovalDeny, fmt.Errorf("approval request send timed out or cancelled: %w", sendCtx.Err())
	}

	// Wait for a decision (same timeout window from when the request was sent).
	select {
	case decision := <-c.Responses:
		return decision, nil
	case <-sendCtx.Done():
		return ApprovalDeny, fmt.Errorf("approval response timed out or cancelled: %w", sendCtx.Err())
	}
}

// buildApprovalRequest constructs a rich ApprovalRequest from a tool call,
// generating a human-readable preview and risk classification automatically.
func buildApprovalRequest(agentName, toolName string, input json.RawMessage) ApprovalRequest {
	preview, risk := previewAndRisk(toolName, input)
	return ApprovalRequest{
		AgentName: agentName,
		ToolName:  toolName,
		ToolInput: input,
		RiskLevel: risk,
		Preview:   preview,
	}
}

// previewAndRisk generates a human-readable one-liner and a risk level for a tool call.
func previewAndRisk(toolName string, input json.RawMessage) (preview, risk string) {
	var args map[string]json.RawMessage
	_ = json.Unmarshal(input, &args)

	str := func(key string) string {
		v, ok := args[key]
		if !ok {
			return ""
		}
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return strings.Trim(string(v), `"`)
		}
		return s
	}

	switch toolName {
	case "bash":
		cmd := str("command")
		risk = classifyBashRisk(cmd)
		if cmd == "" {
			return "run shell command", risk
		}
		if len(cmd) > 80 {
			cmd = cmd[:77] + "…"
		}
		return "$ " + cmd, risk

	case "write":
		path := str("path")
		risk = "medium"
		if path == "" {
			return "write file", risk
		}
		return "write " + path, risk

	case "edit":
		path := str("path")
		risk = "medium"
		if path == "" {
			return "edit file", risk
		}
		return "edit " + path, risk

	case "patch":
		path := str("path")
		risk = "medium"
		if path == "" {
			return "patch file", risk
		}
		return "patch " + path, risk

	default:
		return toolName, "low"
	}
}

// classifyBashRisk classifies a shell command's risk level.
// Low-risk commands are auto-approved in supervised mode without prompting the user.
// The classification goes: low → medium → high → critical.
func classifyBashRisk(cmd string) string {
	lower := strings.ToLower(strings.TrimSpace(cmd))

	// Critical: destructive, privilege escalation, or remote code execution.
	// These are auto-denied regardless of autonomy mode.
	for _, p := range []string{
		"rm -rf", "rm -fr", "sudo", "chmod 777",
		"curl | sh", "curl|sh", "wget | sh", "wget|sh",
		"mkfs", "dd if=", "> /dev/", ":(){ :|:",
	} {
		if strings.Contains(lower, p) {
			return "critical"
		}
	}

	// High: potentially destructive file or repo operations.
	for _, p := range []string{
		"rm ", "chmod", "chown", "truncate", "shred",
		"git push --force", "git push -f", "git reset --hard",
		"git clean -f", "git checkout -- ",
		"mv /", "cp /",
	} {
		if strings.Contains(lower, p) {
			return "high"
		}
	}

	// Medium: build tools, package managers, containers — safe but slow/side-effectful.
	for _, p := range []string{
		"make", "go build", "go test", "go run", "go generate",
		"npm", "yarn", "pnpm", "bun",
		"pip install", "pip3 install",
		"apt", "brew install", "brew upgrade",
		"docker", "kubectl", "helm",
		"cargo build", "cargo test",
		"python ", "python3 ",
		"git commit", "git merge", "git rebase", "git stash",
		"git add", "git tag", "git branch -d",
	} {
		if strings.Contains(lower, p) {
			return "medium"
		}
	}

	// Low: pure read-only or completely safe commands.
	// These will be auto-approved in supervised mode.
	for _, p := range []string{
		"pwd", "echo ", "printf ",
		"cat ", "head ", "tail ", "less ", "more ",
		"ls ", "ls\n", "ls$", "find ",
		"grep ", "rg ", "awk ", "sed ",
		"git status", "git log", "git diff", "git show", "git branch", "git remote",
		"git fetch", "git stash list",
		"go vet", "golint", "staticcheck",
		"which ", "type ", "command ",
		"env", "printenv", "date", "whoami", "hostname",
		"wc ", "sort ", "uniq ", "cut ", "tr ",
		"test ", "[ ", "[[ ",
	} {
		if strings.Contains(lower, p) || lower == p[:len(p)-1] {
			return "low"
		}
	}

	// Default: treat unknown commands as medium — ask but don't alarm.
	return "medium"
}

// DefaultApprovalCallback returns an appropriate PermissionCallback for the given autonomy mode.
// For AutonomySupervised, callers must supply a ChannelCallback themselves; nil is returned
// as a sentinel — the caller should replace it before use.
func DefaultApprovalCallback(autonomy AutonomyMode) PermissionCallback {
	switch autonomy {
	case AutonomyFull:
		return AutoApproveCallback{}
	case AutonomyPlan:
		return AutoDenyCallback{}
	default: // AutonomySupervised
		return nil // caller must provide ChannelCallback
	}
}
