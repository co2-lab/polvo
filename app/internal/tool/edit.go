package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type editInput struct {
	Path       string `json:"path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
}

type editTool struct {
	workdir string
}

func NewEditTool(workdir string) Tool { return &editTool{workdir: workdir} }

func (t *editTool) Name() string { return "edit" }

func (t *editTool) Description() string {
	return "Replace a string in a file. The old_string must match exactly and be unique (unless replace_all is true)."
}

func (t *editTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path":        {"type": "string", "description": "File path relative to working directory"},
			"old_string":  {"type": "string", "description": "Exact string to find and replace"},
			"new_string":  {"type": "string", "description": "Replacement string"},
			"replace_all": {"type": "boolean", "description": "Replace all occurrences (default false)", "default": false}
		},
		"required": ["path", "old_string", "new_string"]
	}`)
}

func (t *editTool) Execute(_ context.Context, input json.RawMessage) (*Result, error) {
	var in editInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ErrorResult(fmt.Sprintf("invalid input: %v", err)), nil
	}

	if in.OldString == in.NewString {
		return ErrorResult("old_string and new_string must be different"), nil
	}

	path, err := resolvePath(t.workdir, in.Path)
	if err != nil {
		return ErrorResult(err.Error()), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return ErrorResult(fmt.Sprintf("reading file: %v", err)), nil
	}

	content := string(data)

	count := strings.Count(content, in.OldString)
	if count == 0 {
		// Try whitespace-normalized match
		normalized := normalizeWhitespace(content)
		normalizedOld := normalizeWhitespace(in.OldString)
		if strings.Contains(normalized, normalizedOld) {
			return ErrorResult("old_string not found as exact match, but found with different whitespace. Please match the exact whitespace in the file."), nil
		}
		return ErrorResult("old_string not found in file"), nil
	}

	if !in.ReplaceAll && count > 1 {
		return ErrorResult(fmt.Sprintf("old_string found %d times — must be unique. Provide more context or use replace_all.", count)), nil
	}

	var newContent string
	if in.ReplaceAll {
		newContent = strings.ReplaceAll(content, in.OldString, in.NewString)
	} else {
		newContent = strings.Replace(content, in.OldString, in.NewString, 1)
	}

	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return ErrorResult(fmt.Sprintf("writing file: %v", err)), nil
	}

	return &Result{Content: fmt.Sprintf("Replaced %d occurrence(s) in %s", count, in.Path)}, nil
}

func normalizeWhitespace(s string) string {
	var sb strings.Builder
	inSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' {
			if !inSpace {
				sb.WriteByte(' ')
				inSpace = true
			}
		} else {
			sb.WriteRune(r)
			inSpace = false
		}
	}
	return sb.String()
}
