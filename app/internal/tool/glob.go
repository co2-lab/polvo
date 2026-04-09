package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type globInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

type globTool struct {
	workdir string
}

func NewGlob(workdir string) Tool { return &globTool{workdir: workdir} }

func (t *globTool) Name() string { return "glob" }

func (t *globTool) Description() string {
	return "Find files matching a glob pattern. Returns matching file paths."
}

func (t *globTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {"type": "string", "description": "Glob pattern (e.g. \"**/*.go\", \"src/**/*.ts\")"},
			"path":    {"type": "string", "description": "Directory to search in (default: working directory)"}
		},
		"required": ["pattern"]
	}`)
}

func (t *globTool) Execute(_ context.Context, input json.RawMessage) (*Result, error) {
	var in globInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ErrorResult(fmt.Sprintf("invalid input: %v", err)), nil
	}

	searchDir := t.workdir
	if in.Path != "" {
		resolved, err := resolvePath(t.workdir, in.Path)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		searchDir = resolved
	}

	var matches []string
	err := filepath.WalkDir(searchDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}

		// Skip hidden directories
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") && path != searchDir {
			return filepath.SkipDir
		}

		rel, err := filepath.Rel(searchDir, path)
		if err != nil {
			return nil
		}

		matched, err := filepath.Match(in.Pattern, d.Name())
		if err != nil {
			return nil
		}

		// Also try matching against the relative path for patterns like "**/*.go"
		if !matched {
			matched = matchDoublestar(in.Pattern, rel)
		}

		if matched {
			// Return path relative to workdir
			relToWorkdir, _ := filepath.Rel(t.workdir, path)
			matches = append(matches, relToWorkdir)
		}
		return nil
	})
	if err != nil {
		return ErrorResult(fmt.Sprintf("walking directory: %v", err)), nil
	}

	if len(matches) == 0 {
		return &Result{Content: "No matches found"}, nil
	}

	return &Result{Content: strings.Join(matches, "\n")}, nil
}

// matchDoublestar handles simple ** patterns.
func matchDoublestar(pattern, path string) bool {
	// Handle **/*.ext patterns
	if strings.HasPrefix(pattern, "**/") {
		suffix := pattern[3:]
		// Match against just the filename
		matched, _ := filepath.Match(suffix, filepath.Base(path))
		return matched
	}
	// Handle dir/**/*.ext
	parts := strings.SplitN(pattern, "/**/", 2)
	if len(parts) == 2 {
		if !strings.HasPrefix(path, parts[0]+"/") && path != parts[0] {
			return false
		}
		suffix := parts[1]
		matched, _ := filepath.Match(suffix, filepath.Base(path))
		return matched
	}
	return false
}
