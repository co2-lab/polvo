package agent

import (
	"encoding/json"
	"regexp"
	"strings"
)

// WorkPlan is the structured output of the architect phase.
// It is serialized to JSON and injected as the first user turn of the editor phase.
type WorkPlan struct {
	Summary     string     `json:"summary"`
	FilesToEdit []string   `json:"files_to_edit"`
	Steps       []WorkStep `json:"steps"`
}

// WorkStep describes one targeted edit the editor must perform.
type WorkStep struct {
	File        string `json:"file"`
	Description string `json:"description"`
	SearchFor   string `json:"search_for,omitempty"`
	ReplaceWith string `json:"replace_with,omitempty"`
}

var workPlanTagRe = regexp.MustCompile(`(?s)<work_plan>(.*?)</work_plan>`)

// ExtractWorkPlan parses a WorkPlan from an architect's response.
// Returns nil if no <work_plan>...</work_plan> block found or JSON is invalid.
func ExtractWorkPlan(response string) *WorkPlan {
	m := workPlanTagRe.FindStringSubmatch(response)
	if len(m) < 2 {
		return nil
	}
	var plan WorkPlan
	if err := json.Unmarshal([]byte(strings.TrimSpace(m[1])), &plan); err != nil {
		return nil
	}
	return &plan
}

// RenderWorkPlan returns a human-readable representation of the plan
// for injection into the editor's first user message.
func RenderWorkPlan(plan *WorkPlan) string {
	b, _ := json.MarshalIndent(plan, "", "  ")
	return "Here is the work plan from the architect. Implement each step exactly:\n\n" +
		"```json\n" + string(b) + "\n```"
}

// ArchitectSystemSuffix is appended to the architect's system prompt.
const ArchitectSystemSuffix = `
When you have finished reasoning about the required changes, output a work plan in this exact format:

<work_plan>
{
  "summary": "one sentence describing the change",
  "files_to_edit": ["path/to/file.go"],
  "steps": [
    {
      "file": "path/to/file.go",
      "description": "what to change and why",
      "search_for": "exact string to find (optional)",
      "replace_with": "replacement string (optional)"
    }
  ]
}
</work_plan>

Do not edit any files. Your role is to plan; the editor will apply changes.`

// EditorSystemSuffix is appended to the editor's system prompt.
const EditorSystemSuffix = `
You will receive a work plan from the architect. Execute each step using the available file editing tools.
Apply changes precisely as described. Do not re-reason about the approach — only implement what the plan specifies.
After completing all steps, confirm with a brief summary of what was changed.`
