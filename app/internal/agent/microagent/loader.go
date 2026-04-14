package microagent

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Frontmatter is the YAML header parsed from a microagent Markdown file.
type Frontmatter struct {
	Name     string    `yaml:"name"`
	Scope    string    `yaml:"scope"`
	Priority int       `yaml:"priority"`
	Triggers []Trigger `yaml:"triggers"`
}

// Loader loads and caches microagent definitions from workspace and user directories.
// File format: Markdown with a YAML frontmatter block delimited by "---" lines.
//
// Security: fail-closed — a file with invalid YAML frontmatter is skipped with
// slog.Warn and is never passed to the LLM.
type Loader struct {
	workspacePath string // .polvo/microagents/
	userPath      string // ~/.polvo/microagents/
	mu            sync.RWMutex
	cache         []Microagent
}

// NewLoader creates a Loader for the given workspace and user directories.
// Either path may be empty; that directory is simply skipped.
func NewLoader(workspacePath, userPath string) *Loader {
	return &Loader{
		workspacePath: workspacePath,
		userPath:      userPath,
	}
}

// LoadAll scans both directories, parses YAML frontmatter, and returns all
// microagents. When two microagents share the same name, the workspace version
// wins (overrides the user-scoped one).
//
// Files with invalid YAML are logged at Warn level and skipped.
func (l *Loader) LoadAll() ([]Microagent, error) {
	// user-scoped microagents loaded first; workspace overrides same-name entries.
	byName := make(map[string]Microagent)

	dirs := []struct {
		path  string
		scope string
	}{
		{l.userPath, "user"},
		{l.workspacePath, "workspace"}, // workspace wins by being applied last
	}

	for _, dir := range dirs {
		if dir.path == "" {
			continue
		}
		if _, err := os.Stat(dir.path); os.IsNotExist(err) {
			continue
		}

		// Collect all valid microagents from this directory tree. Within a
		// single tree, the file closest to the root wins (first encountered
		// in WalkDir's lexical, depth-first order). Across the two directory
		// sources, the workspace tree wins because it is processed last and
		// always overwrites whatever the user tree loaded.
		treeByName := make(map[string]Microagent)
		scope := dir.scope
		walkErr := filepath.WalkDir(dir.path, func(fullPath string, d os.DirEntry, err error) error {
			if err != nil {
				slog.Warn("microagent: walk error", "path", fullPath, "error", err)
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
				return nil
			}

			ma, err := loadFile(fullPath, scope)
			if err != nil {
				slog.Warn("microagent: skipping invalid file",
					"path", fullPath,
					"error", err,
				)
				return nil
			}
			// Within this tree, root-level entries are encountered first;
			// only store the first (shallowest) occurrence of each name.
			if _, exists := treeByName[ma.Name]; !exists {
				treeByName[ma.Name] = ma
			}
			return nil
		})
		if walkErr != nil {
			return nil, fmt.Errorf("walking microagent dir %s: %w", dir.path, walkErr)
		}

		// Merge tree results into byName; later trees overwrite earlier ones
		// (workspace overwrites user).
		for name, ma := range treeByName {
			byName[name] = ma
		}
	}

	result := make([]Microagent, 0, len(byName))
	for _, ma := range byName {
		result = append(result, ma)
	}

	l.mu.Lock()
	l.cache = result
	l.mu.Unlock()

	return result, nil
}

// Cached returns the last result from LoadAll without hitting the filesystem.
func (l *Loader) Cached() []Microagent {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.cache
}

// loadFile parses a single microagent Markdown file.
func loadFile(path, defaultScope string) (Microagent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Microagent{}, fmt.Errorf("reading file: %w", err)
	}

	fm, body, err := parseFrontmatter(data)
	if err != nil {
		return Microagent{}, fmt.Errorf("frontmatter: %w", err)
	}

	scope := fm.Scope
	if scope == "" {
		scope = defaultScope
	}

	name := fm.Name
	if name == "" {
		// Fall back to filename without extension.
		base := filepath.Base(path)
		name = strings.TrimSuffix(base, filepath.Ext(base))
	}

	return Microagent{
		Name:     name,
		Scope:    scope,
		Priority: fm.Priority,
		Triggers: fm.Triggers,
		Content:  strings.TrimSpace(body),
		Path:     path,
	}, nil
}

// parseFrontmatter splits Markdown content into YAML frontmatter and body.
// The frontmatter must be enclosed by "---" delimiters at the start of the file.
// Returns an error if the opening delimiter is missing or YAML is invalid.
func parseFrontmatter(data []byte) (Frontmatter, string, error) {
	text := string(data)

	// File must start with "---" (allow optional leading newline)
	text = strings.TrimLeft(text, "\r\n")
	if !strings.HasPrefix(text, "---") {
		return Frontmatter{}, "", fmt.Errorf("missing opening '---' delimiter")
	}

	// Strip the opening delimiter line.
	rest := text[3:]
	// Skip the newline after "---"
	if len(rest) > 0 && rest[0] == '\r' {
		rest = rest[1:]
	}
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	}

	// Find the closing "---"
	idx := findClosingDelimiter(rest)
	if idx < 0 {
		return Frontmatter{}, "", fmt.Errorf("missing closing '---' delimiter")
	}

	yamlPart := rest[:idx]
	body := rest[idx:]

	// Skip closing delimiter line in body.
	body = strings.TrimPrefix(body, "---")
	if len(body) > 0 && body[0] == '\r' {
		body = body[1:]
	}
	if len(body) > 0 && body[0] == '\n' {
		body = body[1:]
	}

	var fm Frontmatter
	dec := yaml.NewDecoder(bytes.NewReader([]byte(yamlPart)))
	dec.KnownFields(true)
	if err := dec.Decode(&fm); err != nil {
		return Frontmatter{}, "", fmt.Errorf("yaml decode: %w", err)
	}

	return fm, body, nil
}

// findClosingDelimiter returns the byte offset of the "---" closing delimiter
// within s, or -1 if not found. The delimiter must appear at the start of a line.
func findClosingDelimiter(s string) int {
	for i := 0; i < len(s); {
		end := strings.IndexByte(s[i:], '\n')
		var line string
		if end < 0 {
			line = s[i:]
		} else {
			line = s[i : i+end]
		}
		line = strings.TrimRight(line, "\r")
		if line == "---" {
			return i
		}
		if end < 0 {
			break
		}
		i += end + 1
	}
	return -1
}

// NOTE for future integration with loop.go:
// At the start of each Loop.Run call, obtain matched microagents and inject them:
//
//   loader := microagent.NewLoader(workspaceMicroagentsDir, userMicroagentsDir)
//   agents, _ := loader.LoadAll()
//   evalCtx := microagent.EvalContext{UserMessage: userPrompt, SessionFiles: sessionFiles}
//   matches := microagent.Match(agents, evalCtx)
//   systemPrompt += microagent.Inject(matches, 5)
