// Package risk provides authoritative risk scoring for tool executions.
package risk

import (
	"encoding/json"
	"strings"
)

// RiskLevel is an ordered integer so levels can be compared with < and >.
type RiskLevel int

const (
	RiskLow      RiskLevel = 1
	RiskMedium   RiskLevel = 2
	RiskHigh     RiskLevel = 3
	RiskCritical RiskLevel = 4
)

func (r RiskLevel) String() string {
	switch r {
	case RiskLow:
		return "low"
	case RiskMedium:
		return "medium"
	case RiskHigh:
		return "high"
	case RiskCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// RiskScorer scores a tool call before execution.
type RiskScorer interface {
	Score(toolName string, toolInput json.RawMessage) RiskLevel
}

// NoopScorer always returns RiskLow. Used in tests and backward-compat paths.
type NoopScorer struct{}

func (NoopScorer) Score(_ string, _ json.RawMessage) RiskLevel { return RiskLow }

// RuleBasedScorer scores tool calls using a static rule table.
type RuleBasedScorer struct {
	// WorkDir is used to detect writes outside the project root.
	WorkDir string
	// Blocklist is the set of bash patterns that are auto-denied (Critical).
	Blocklist []string
}

// NewRuleBasedScorer creates a scorer with sensible defaults.
func NewRuleBasedScorer(workDir string, blocklist []string) *RuleBasedScorer {
	return &RuleBasedScorer{WorkDir: workDir, Blocklist: blocklist}
}

func (s *RuleBasedScorer) Score(toolName string, toolInput json.RawMessage) RiskLevel {
	var args map[string]json.RawMessage
	_ = json.Unmarshal(toolInput, &args)

	strVal := func(key string) string {
		v, ok := args[key]
		if !ok {
			return ""
		}
		var sv string
		if err := json.Unmarshal(v, &sv); err != nil {
			return strings.Trim(string(v), `"`)
		}
		return sv
	}

	switch toolName {
	case "bash":
		return s.scoreBash(strVal("command"))

	case "write":
		return s.scorePath(strVal("path"), RiskLow)

	case "edit", "patch":
		return RiskMedium

	case "read", "glob", "grep", "ls", "think",
		"web_fetch", "web_search", "memory_read":
		return RiskLow

	case "delegate":
		return RiskMedium

	default:
		// MCP tools and unknown tools default to High.
		return RiskHigh
	}
}

// scoreBash classifies a shell command.
func (s *RuleBasedScorer) scoreBash(cmd string) RiskLevel {
	lower := strings.ToLower(strings.TrimSpace(cmd))

	// Blocklist → Critical.
	for _, p := range s.Blocklist {
		if strings.Contains(lower, strings.ToLower(p)) {
			return RiskCritical
		}
	}

	// Built-in critical patterns.
	for _, p := range []string{
		"rm -rf", "rm -fr", "sudo", "chmod 777",
		"curl | sh", "curl|sh", "wget | sh", "wget|sh",
		"mkfs", "dd if=", "> /dev/", ":(){ :|:",
	} {
		if strings.Contains(lower, p) {
			return RiskCritical
		}
	}

	// High: system paths.
	for _, p := range []string{"/etc/", "/usr/", "/sys/", "~/.ssh/", "~/.aws/"} {
		if strings.Contains(lower, p) {
			return RiskHigh
		}
	}

	return RiskMedium
}

// scorePath classifies a file path write operation.
func (s *RuleBasedScorer) scorePath(path string, defaultLevel RiskLevel) RiskLevel {
	if path == "" {
		return defaultLevel
	}

	// Writes outside working dir.
	if s.WorkDir != "" && !strings.HasPrefix(path, s.WorkDir) && !strings.HasPrefix(path, "./") && !strings.HasPrefix(path, "../") {
		if strings.HasPrefix(path, "/") {
			return RiskHigh
		}
	}

	// Sensitive file extensions.
	lower := strings.ToLower(path)
	for _, suf := range []string{".env", ".pem", ".key", "_rsa", ".p12", ".pfx"} {
		if strings.HasSuffix(lower, suf) {
			return RiskHigh
		}
	}

	return defaultLevel
}
