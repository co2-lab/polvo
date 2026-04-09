package template

import (
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	tmpl := "File: {{.File}}\nEvent: {{.Event}}\n{{if .Content}}Content: {{.Content}}{{end}}"
	data := &Data{
		File:    "screens/Home.tsx",
		Event:   "modified",
		Content: "export default function Home() {}",
	}

	result, err := Render(tmpl, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "screens/Home.tsx") {
		t.Error("expected file path in output")
	}
	if !strings.Contains(result, "modified") {
		t.Error("expected event in output")
	}
	if !strings.Contains(result, "export default") {
		t.Error("expected content in output")
	}
}

func TestRenderEmptyOptionalFields(t *testing.T) {
	tmpl := "{{if .Diff}}Diff: {{.Diff}}{{else}}No diff{{end}}"
	data := &Data{}

	result, err := Render(tmpl, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "No diff" {
		t.Errorf("expected 'No diff', got %q", result)
	}
}

func TestRenderInvalidTemplate(t *testing.T) {
	_, err := Render("{{.Invalid", &Data{})
	if err == nil {
		t.Error("expected error for invalid template")
	}
}
