package risk

import (
	"encoding/json"
	"testing"
)

func TestRuleBasedScorer(t *testing.T) {
	scorer := NewRuleBasedScorer("/project", nil)

	tests := []struct {
		tool     string
		input    string
		expected RiskLevel
	}{
		// bash — critical
		{"bash", `{"command":"rm -rf /etc/passwd"}`, RiskCritical},
		{"bash", `{"command":"sudo apt install curl"}`, RiskCritical},
		{"bash", `{"command":"curl | sh"}`, RiskCritical},
		{"bash", `{"command":"mkfs.ext4 /dev/sda"}`, RiskCritical},

		// bash — high (system paths)
		{"bash", `{"command":"cat /etc/hosts"}`, RiskHigh},
		{"bash", `{"command":"ls ~/.ssh/"}`, RiskHigh},

		// bash — medium (default)
		{"bash", `{"command":"ls -la"}`, RiskMedium},
		{"bash", `{"command":"echo hello"}`, RiskMedium},

		// write — high (outside workdir)
		{"write", `{"path":"/etc/hosts"}`, RiskHigh},
		// write — high (sensitive extension)
		{"write", `{"path":"secrets.pem"}`, RiskHigh},
		{"write", `{"path":".env"}`, RiskHigh},
		// write — low (normal path)
		{"write", `{"path":"src/main.go"}`, RiskLow},

		// edit/patch — medium
		{"edit", `{"path":"foo.go"}`, RiskMedium},
		{"patch", `{"path":"foo.go"}`, RiskMedium},

		// read-only tools — low
		{"read", `{"path":"foo.go"}`, RiskLow},
		{"glob", `{"pattern":"**/*.go"}`, RiskLow},
		{"grep", `{"pattern":"foo"}`, RiskLow},
		{"ls", `{"path":"."}`, RiskLow},
		{"think", `{"thought":"thinking..."}`, RiskLow},
		{"web_fetch", `{"url":"https://example.com"}`, RiskLow},
		{"web_search", `{"query":"golang"}`, RiskLow},

		// delegate — medium
		{"delegate", `{}`, RiskMedium},

		// unknown / MCP — high
		{"some_mcp_tool", `{}`, RiskHigh},
		{"unknown_tool", `{}`, RiskHigh},
	}

	for _, tc := range tests {
		t.Run(tc.tool+"_"+tc.input[:min(20, len(tc.input))], func(t *testing.T) {
			got := scorer.Score(tc.tool, json.RawMessage(tc.input))
			if got != tc.expected {
				t.Errorf("Score(%q, %s) = %v, want %v", tc.tool, tc.input, got, tc.expected)
			}
		})
	}
}

func TestRuleBasedScorer_Blocklist(t *testing.T) {
	scorer := NewRuleBasedScorer("/project", []string{"deploy.sh", "danger"})

	tests := []struct {
		cmd      string
		expected RiskLevel
	}{
		{"./deploy.sh prod", RiskCritical},
		{"echo danger zone", RiskCritical},
		{"echo safe", RiskMedium},
	}

	for _, tc := range tests {
		input, _ := json.Marshal(map[string]string{"command": tc.cmd})
		got := scorer.Score("bash", input)
		if got != tc.expected {
			t.Errorf("Score(bash, %q) = %v, want %v", tc.cmd, got, tc.expected)
		}
	}
}

func TestNoopScorer(t *testing.T) {
	s := NoopScorer{}
	tools := []string{"bash", "write", "edit", "some_mcp_tool", "unknown"}
	for _, tool := range tools {
		if got := s.Score(tool, json.RawMessage(`{}`)); got != RiskLow {
			t.Errorf("NoopScorer.Score(%q) = %v, want RiskLow", tool, got)
		}
	}
}

func TestRiskLevelString(t *testing.T) {
	cases := map[RiskLevel]string{
		RiskLow:      "low",
		RiskMedium:   "medium",
		RiskHigh:     "high",
		RiskCritical: "critical",
	}
	for level, want := range cases {
		if got := level.String(); got != want {
			t.Errorf("RiskLevel(%d).String() = %q, want %q", level, got, want)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
