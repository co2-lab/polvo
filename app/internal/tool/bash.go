package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/co2-lab/polvo/internal/secrets"
)

const defaultTimeout = 120 * time.Second

// defaultBlocklist contains commands that are blocked by default for safety.
// Three layers: prefix match, exact match, and substring patterns.
var defaultBlocklist = []string{
	// Destructive filesystem
	"rm -rf /",
	"rm -rf /*",
	"rm -rf ",  // catches "rm -rf build/", "rm -rf ." etc.
	"mkfs",
	"> /dev/sd",
	"dd if=",
	// Privilege escalation
	"sudo ",
	"su ",
	// Piped execution (code injection pattern)
	"curl | sh",
	"curl | bash",
	"wget | sh",
	"wget | bash",
	"curl|sh",
	"curl|bash",
	"wget|sh",
	"wget|bash",
	// Fork bombs
	":(){ :|:& };:",
}

// defaultBlocklistRegex contains compiled regex patterns for cases that
// substring matching cannot handle (e.g. end-of-string anchors).
var defaultBlocklistRegex = []*regexp.Regexp{
	// Bare "rm -rf" with no argument (end of string)
	regexp.MustCompile(`(?i)rm\s+-rf$`),
}

type bashInput struct {
	Command      string `json:"command"`
	Timeout      int    `json:"timeout"`       // seconds
	SecurityRisk string `json:"security_risk"` // low | medium | high | critical
}

type bashTool struct {
	workdir     string
	blocklist   []string         // merged: default + config
	blocklistRx []*regexp.Regexp // regex patterns
	session     *BashSession     // optional persistent session
}

// NewBash creates the bash tool with an optional extra blocklist from config.
func NewBash(workdir string, extraBlocklist ...string) Tool {
	bl := make([]string, len(defaultBlocklist))
	copy(bl, defaultBlocklist)
	bl = append(bl, extraBlocklist...)
	rx := make([]*regexp.Regexp, len(defaultBlocklistRegex))
	copy(rx, defaultBlocklistRegex)
	return &bashTool{workdir: workdir, blocklist: bl, blocklistRx: rx}
}

// NewBashWithSession creates the bash tool backed by a persistent BashSession.
// If session is nil the tool falls back to the standard one-shot exec behaviour.
func NewBashWithSession(workdir string, extraBlocklist []string, extraRx []*regexp.Regexp, session *BashSession) Tool {
	bl := make([]string, len(defaultBlocklist))
	copy(bl, defaultBlocklist)
	bl = append(bl, extraBlocklist...)
	rx := make([]*regexp.Regexp, len(defaultBlocklistRegex))
	copy(rx, defaultBlocklistRegex)
	rx = append(rx, extraRx...)
	return &bashTool{workdir: workdir, blocklist: bl, blocklistRx: rx, session: session}
}

func (t *bashTool) Name() string { return "bash" }

func (t *bashTool) Description() string {
	return "Execute a shell command and return stdout/stderr. Use for running tests, git operations, builds, etc. Set security_risk to classify the command risk level."
}

func (t *bashTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command":       {"type": "string", "description": "Shell command to execute"},
			"timeout":       {"type": "integer", "description": "Timeout in seconds (default 120)", "default": 120},
			"security_risk": {"type": "string", "enum": ["low","medium","high","critical"], "description": "Assessed risk level of this command", "default": "low"}
		},
		"required": ["command", "security_risk"]
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

	// Reject commands that require an interactive TTY — they would deadlock a
	// persistent session and are not useful in an agent context.
	if isInteractiveCommand(in.Command) {
		return ErrorResult("command requires interactive TTY — not supported in agent context"), nil
	}

	// Blocklist check — split on ; | && || to catch chained commands
	subCmds := splitCommands(in.Command)
	for _, sub := range subCmds {
		if blocked, pattern := t.isBlocked(sub); blocked {
			return ErrorResult(fmt.Sprintf("command blocked by security policy (matches pattern %q): %s", pattern, sub)), nil
		}
	}

	// Log security risk
	if in.SecurityRisk == "high" || in.SecurityRisk == "critical" {
		slog.Warn("bash tool: high-risk command", "command", in.Command, "security_risk", in.SecurityRisk)
	}

	timeout := defaultTimeout
	if in.Timeout > 0 {
		timeout = time.Duration(in.Timeout) * time.Second
	}

	// --- Persistent session path ---
	if t.session != nil {
		sessionCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		output, exitCode, err := t.session.Run(sessionCtx, in.Command)

		output = TruncateObservation(output, DefaultMaxObservationChars)

		// Mask secrets in command output before returning.
		output, _ = secrets.MaskSecrets(output)

		if err != nil {
			return ErrorResult(fmt.Sprintf("command timed out after %s\n%s", timeout, output)), nil
		}
		if exitCode != 0 {
			if output == "" {
				output = "(no output)"
			}
			return &Result{Content: fmt.Sprintf("[exit %d]\n%s", exitCode, output), IsError: true}, nil
		}
		if output == "" {
			output = "(no output)"
		}
		return &Result{Content: output}, nil
	}

	// --- One-shot exec path (no session) ---
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "sh", "-c", in.Command)
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

	output = TruncateObservation(output, DefaultMaxObservationChars)

	// Mask secrets in command output before returning.
	output, _ = secrets.MaskSecrets(output)

	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			return ErrorResult(fmt.Sprintf("command timed out after %s\n%s", timeout, output)), nil
		}
		return &Result{Content: fmt.Sprintf("exit status: %v\n%s", err, output), IsError: true}, nil
	}

	if output == "" {
		output = "(no output)"
	}

	return &Result{Content: output}, nil
}

// isBlocked returns true if cmd matches any blocklist pattern.
func (t *bashTool) isBlocked(cmd string) (bool, string) {
	cmd = strings.TrimSpace(cmd)
	// Normalize whitespace for matching only — do not change what gets executed.
	normalized := strings.Join(strings.Fields(cmd), " ")
	lower := strings.ToLower(normalized)
	for _, pattern := range t.blocklist {
		p := strings.ToLower(pattern)
		if strings.HasPrefix(lower, p) || strings.Contains(lower, p) {
			return true, pattern
		}
	}
	// Regex patterns applied to the normalized lower-case form.
	for _, rx := range t.blocklistRx {
		if rx.MatchString(lower) {
			return true, rx.String()
		}
	}
	return false, ""
}

// splitCommands splits a shell command string on ; | && || to extract sub-commands.
func splitCommands(cmd string) []string {
	// Simple tokenizer — good enough for blocklist purposes
	var parts []string
	separators := []string{";", "&&", "||", "|"}
	current := cmd
	for _, sep := range separators {
		var next []string
		for _, part := range strings.Split(current, sep) {
			next = append(next, strings.TrimSpace(part))
		}
		// Reassemble for next separator pass
		current = strings.Join(next, "\n")
	}
	for _, p := range strings.Split(current, "\n") {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return []string{cmd}
	}
	return parts
}
