package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

const (
	defaultTimeout  = 120 * time.Second
	maxOutputBytes  = 100 * 1024 // 100KB
)

type bashInput struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout"` // seconds
}

type bashTool struct {
	workdir string
}

func NewBash(workdir string) Tool { return &bashTool{workdir: workdir} }

func (t *bashTool) Name() string { return "bash" }

func (t *bashTool) Description() string {
	return "Execute a shell command and return stdout/stderr. Use for running tests, git operations, builds, etc."
}

func (t *bashTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {"type": "string", "description": "Shell command to execute"},
			"timeout": {"type": "integer", "description": "Timeout in seconds (default 120)", "default": 120}
		},
		"required": ["command"]
	}`)
}

func (t *bashTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in bashInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ErrorResult(fmt.Sprintf("invalid input: %v", err)), nil
	}

	if in.Command == "" {
		return ErrorResult("command is required"), nil
	}

	timeout := defaultTimeout
	if in.Timeout > 0 {
		timeout = time.Duration(in.Timeout) * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", in.Command)
	cmd.Dir = t.workdir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	var output string
	if stdout.Len() > 0 {
		output = stdout.String()
	}
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}

	// Truncate if too large
	if len(output) > maxOutputBytes {
		output = output[:maxOutputBytes] + "\n... (output truncated at 100KB)"
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return ErrorResult(fmt.Sprintf("command timed out after %s\n%s", timeout, output)), nil
		}
		return &Result{Content: fmt.Sprintf("exit status: %v\n%s", err, output), IsError: true}, nil
	}

	if output == "" {
		output = "(no output)"
	}

	return &Result{Content: output}, nil
}
