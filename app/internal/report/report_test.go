package report

import (
	"strings"
	"testing"

	"github.com/co2-lab/polvo/internal/agent"
)

func TestNewReport(t *testing.T) {
	result := &agent.Result{
		Decision: "REJECT",
		Summary:  "Found 2 issues",
		Findings: []agent.Finding{
			{File: "main.go", Line: 10, Severity: "error", Message: "unused variable"},
			{File: "main.go", Line: 20, Severity: "warning", Message: "long function"},
		},
	}

	r := NewReport("lint", "lint", "main.go", result)

	if r.Agent != "lint" {
		t.Errorf("expected agent 'lint', got %q", r.Agent)
	}
	if r.Decision != "REJECT" {
		t.Errorf("expected decision 'REJECT', got %q", r.Decision)
	}
	if r.Severity != SeverityHigh {
		t.Errorf("expected severity 'high', got %q", r.Severity)
	}
	if len(r.Findings) != 2 {
		t.Errorf("expected 2 findings, got %d", len(r.Findings))
	}
}

func TestReportToMarkdown(t *testing.T) {
	r := &Report{
		Agent:    "lint",
		File:     "main.go",
		Decision: "REJECT",
		Severity: SeverityHigh,
		Summary:  "Issues found",
		Findings: []agent.Finding{
			{File: "main.go", Line: 10, Severity: "error", Message: "unused variable", Suggestion: "remove it"},
		},
	}

	md := r.ToMarkdown()

	if !strings.Contains(md, "## Polvo Report: lint") {
		t.Error("expected report header in markdown")
	}
	if !strings.Contains(md, "REJECT") {
		t.Error("expected decision in markdown")
	}
	if !strings.Contains(md, "unused variable") {
		t.Error("expected finding message in markdown")
	}
	if !strings.Contains(md, "remove it") {
		t.Error("expected suggestion in markdown")
	}
}
