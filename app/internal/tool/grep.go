package tool

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type grepInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
	Glob    string `json:"glob"`
}

type grepTool struct {
	workdir string
}

func NewGrep(workdir string) Tool { return &grepTool{workdir: workdir} }

func (t *grepTool) Name() string { return "grep" }

func (t *grepTool) Description() string {
	return "Search file contents using a regex pattern. Returns matching lines with file paths and line numbers."
}

func (t *grepTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {"type": "string", "description": "Regex pattern to search for"},
			"path":    {"type": "string", "description": "File or directory to search in (default: working directory)"},
			"glob":    {"type": "string", "description": "Glob filter for files (e.g. \"*.go\")"}
		},
		"required": ["pattern"]
	}`)
}

func (t *grepTool) Execute(_ context.Context, input json.RawMessage) (*Result, error) {
	var in grepInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ErrorResult(fmt.Sprintf("invalid input: %v", err)), nil
	}

	re, err := regexp.Compile(in.Pattern)
	if err != nil {
		return ErrorResult(fmt.Sprintf("invalid regex: %v", err)), nil
	}

	searchPath := t.workdir
	if in.Path != "" {
		resolved, err := resolvePath(t.workdir, in.Path)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		searchPath = resolved
	}

	info, err := os.Stat(searchPath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("path not found: %v", err)), nil
	}

	var results []string
	maxResults := 200

	if !info.IsDir() {
		lines := searchFile(searchPath, re, t.workdir)
		results = append(results, lines...)
	} else {
		_ = filepath.WalkDir(searchPath, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				if d != nil && d.IsDir() && strings.HasPrefix(d.Name(), ".") && path != searchPath {
					return filepath.SkipDir
				}
				return nil
			}

			if in.Glob != "" {
				matched, _ := filepath.Match(in.Glob, d.Name())
				if !matched {
					return nil
				}
			}

			lines := searchFile(path, re, t.workdir)
			results = append(results, lines...)
			if len(results) >= maxResults {
				return fmt.Errorf("max results reached")
			}
			return nil
		})
	}

	if len(results) == 0 {
		return &Result{Content: "No matches found"}, nil
	}

	if len(results) > maxResults {
		results = results[:maxResults]
		results = append(results, fmt.Sprintf("... (truncated at %d results)", maxResults))
	}

	return &Result{Content: strings.Join(results, "\n")}, nil
}

func searchFile(path string, re *regexp.Regexp, workdir string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	relPath, _ := filepath.Rel(workdir, path)
	var results []string
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if re.MatchString(line) {
			results = append(results, fmt.Sprintf("%s:%d:%s", relPath, lineNum, line))
		}
	}
	return results
}
