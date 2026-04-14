package agent

import (
	"strings"
	"testing"
)

func TestExtractWorkPlan_ValidJSON(t *testing.T) {
	response := `
Some reasoning text here.

<work_plan>
{
  "summary": "add logging",
  "files_to_edit": ["main.go"],
  "steps": [
    {
      "file": "main.go",
      "description": "add slog import and log call",
      "search_for": "func main()",
      "replace_with": "func main() { slog.Info(\"started\") }"
    }
  ]
}
</work_plan>

Trailing text.
`
	plan := ExtractWorkPlan(response)
	if plan == nil {
		t.Fatal("expected non-nil WorkPlan")
	}
	if plan.Summary != "add logging" {
		t.Errorf("expected summary %q, got %q", "add logging", plan.Summary)
	}
	if len(plan.FilesToEdit) != 1 || plan.FilesToEdit[0] != "main.go" {
		t.Errorf("unexpected FilesToEdit: %v", plan.FilesToEdit)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(plan.Steps))
	}
	if plan.Steps[0].File != "main.go" {
		t.Errorf("expected step file %q, got %q", "main.go", plan.Steps[0].File)
	}
	if plan.Steps[0].SearchFor != "func main()" {
		t.Errorf("unexpected SearchFor: %q", plan.Steps[0].SearchFor)
	}
}

func TestExtractWorkPlan_MissingTags(t *testing.T) {
	response := `{"summary":"no tags","files_to_edit":[],"steps":[]}`
	plan := ExtractWorkPlan(response)
	if plan != nil {
		t.Errorf("expected nil when tags are missing, got %+v", plan)
	}
}

func TestExtractWorkPlan_MalformedJSON(t *testing.T) {
	response := `<work_plan>not valid json</work_plan>`
	plan := ExtractWorkPlan(response)
	if plan != nil {
		t.Errorf("expected nil for malformed JSON, got %+v", plan)
	}
}

func TestExtractWorkPlan_ExtraWhitespace(t *testing.T) {
	response := "<work_plan>   \n  {\"summary\":\"ws\",\"files_to_edit\":[],\"steps\":[]}\n   </work_plan>"
	plan := ExtractWorkPlan(response)
	if plan == nil {
		t.Fatal("expected non-nil WorkPlan with extra whitespace")
	}
	if plan.Summary != "ws" {
		t.Errorf("expected summary %q, got %q", "ws", plan.Summary)
	}
}

func TestRenderWorkPlan_ContainsSummary(t *testing.T) {
	plan := &WorkPlan{
		Summary:     "my summary",
		FilesToEdit: []string{"a.go"},
		Steps: []WorkStep{
			{File: "a.go", Description: "do something"},
		},
	}
	rendered := RenderWorkPlan(plan)
	if !strings.Contains(rendered, "my summary") {
		t.Errorf("rendered output does not contain summary; got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "architect") {
		t.Errorf("rendered output does not mention architect; got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "```json") {
		t.Errorf("rendered output does not contain JSON code block; got:\n%s", rendered)
	}
}
