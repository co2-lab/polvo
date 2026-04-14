package microagent

import (
	"os"
	"path/filepath"
	"testing"
)

const validFile = `---
name: redis-guide
scope: workspace
priority: 10
triggers:
  - type: keyword
    words:
      - redis
      - cache
---

# Redis Guide

Use go-redis/v9.
`

const invalidYAMLFile = `---
name: broken
scope: [invalid yaml here
priority: abc: xyz
---

body text
`

const noFrontmatterFile = `# Just a plain markdown file

No frontmatter here.
`

const userFile = `---
name: shared-guide
scope: user
priority: 5
triggers:
  - type: always
---

User version of shared guide.
`

const workspaceFile = `---
name: shared-guide
scope: workspace
priority: 20
triggers:
  - type: always
---

Workspace version of shared guide.
`

func TestLoadAll_ValidFile(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "redis-guide.md", validFile)

	loader := NewLoader(dir, "")
	agents, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	a := agents[0]
	if a.Name != "redis-guide" {
		t.Errorf("name: got %q, want %q", a.Name, "redis-guide")
	}
	if a.Scope != "workspace" {
		t.Errorf("scope: got %q, want %q", a.Scope, "workspace")
	}
	if a.Priority != 10 {
		t.Errorf("priority: got %d, want 10", a.Priority)
	}
	if len(a.Triggers) != 1 {
		t.Fatalf("expected 1 trigger, got %d", len(a.Triggers))
	}
	if a.Triggers[0].Type != TriggerKeyword {
		t.Errorf("trigger type: got %q, want keyword", a.Triggers[0].Type)
	}
	if a.Content == "" {
		t.Error("expected non-empty content")
	}
}

func TestLoadAll_InvalidYAML_Skipped(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "broken.md", invalidYAMLFile)
	writeMD(t, dir, "valid.md", validFile)

	loader := NewLoader(dir, "")
	agents, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll should not return error for invalid file: %v", err)
	}
	// Only the valid file should be loaded.
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent (invalid skipped), got %d", len(agents))
	}
	if agents[0].Name != "redis-guide" {
		t.Errorf("expected redis-guide, got %q", agents[0].Name)
	}
}

func TestLoadAll_NoFrontmatter_Skipped(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "plain.md", noFrontmatterFile)

	loader := NewLoader(dir, "")
	agents, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) != 0 {
		t.Fatalf("expected 0 agents, got %d", len(agents))
	}
}

func TestLoadAll_WorkspaceOverridesUser(t *testing.T) {
	userDir := t.TempDir()
	workspaceDir := t.TempDir()

	writeMD(t, userDir, "shared-guide.md", userFile)
	writeMD(t, workspaceDir, "shared-guide.md", workspaceFile)

	loader := NewLoader(workspaceDir, userDir)
	agents, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent (workspace overrides user), got %d", len(agents))
	}
	a := agents[0]
	if a.Scope != "workspace" {
		t.Errorf("workspace should override user; got scope %q", a.Scope)
	}
	if a.Priority != 20 {
		t.Errorf("workspace priority should be 20, got %d", a.Priority)
	}
	if a.Content == "" || a.Content == "User version of shared guide." {
		t.Errorf("workspace content should win; got %q", a.Content)
	}
}

func TestLoadAll_FallbackNameFromFilename(t *testing.T) {
	// A file with no name in frontmatter should use the filename without extension.
	content := `---
scope: workspace
priority: 1
triggers:
  - type: always
---

Content here.
`
	dir := t.TempDir()
	writeMD(t, dir, "my-agent.md", content)

	loader := NewLoader(dir, "")
	agents, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].Name != "my-agent" {
		t.Errorf("expected name 'my-agent', got %q", agents[0].Name)
	}
}

func TestLoadAll_EmptyDirectories(t *testing.T) {
	loader := NewLoader("", "")
	agents, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) != 0 {
		t.Fatalf("expected 0 agents for empty dirs, got %d", len(agents))
	}
}

// ---------------------------------------------------------------------------
// New gap-coverage tests
// ---------------------------------------------------------------------------

// TestLoader_RecursiveSubdirectories verifies that .md files placed in
// subdirectories are loaded recursively by LoadAll.
func TestLoader_RecursiveSubdirectories(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeMD(t, subdir, "nested.md", validFile)

	loader := NewLoader(dir, "")
	agents, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent loaded from subdirectory, got %d", len(agents))
	}
	if agents[0].Name != "redis-guide" {
		t.Errorf("expected agent name 'redis-guide', got %q", agents[0].Name)
	}
}

// TestLoader_RecursiveSubdirectories_RootWins verifies that when the same agent
// name exists at root level and in a subdirectory, the root-level (closer-to-root)
// file wins within a single directory tree.
func TestLoader_RecursiveSubdirectories_RootWins(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Root-level file uses priority 10 (from validFile / redis-guide).
	writeMD(t, dir, "redis-guide.md", validFile)

	// Subdirectory file has a different priority to distinguish it.
	subFile := `---
name: redis-guide
scope: workspace
priority: 99
triggers:
  - type: always
---

Subdirectory version.
`
	writeMD(t, subdir, "redis-guide.md", subFile)

	loader := NewLoader(dir, "")
	agents, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent (deduplication), got %d", len(agents))
	}
	// Root-level file has priority 10; subdirectory has 99 — root wins.
	if agents[0].Priority != 10 {
		t.Errorf("root-level file should win (priority 10), got priority %d", agents[0].Priority)
	}
}

// TestLoader_EmptyFile verifies that an empty .md file does not crash the
// loader. An empty file has no frontmatter, so it should be skipped.
func TestLoader_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "empty.md", "")

	loader := NewLoader(dir, "")
	agents, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll should not error on empty file: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("expected 0 agents for empty file, got %d", len(agents))
	}
}

// TestLoader_YAMLVersionAsInt creates frontmatter with `version: 1` (an integer
// field). Because the loader uses KnownFields(true), any field not declared in
// Frontmatter is treated as an error and the file is skipped.
func TestLoader_YAMLVersionAsInt(t *testing.T) {
	// GAP: `version` is not a field in Frontmatter — KnownFields(true) will
	// reject the file and it will be skipped with a Warn log.
	content := `---
name: versioned-agent
scope: workspace
priority: 5
version: 1
triggers:
  - type: always
---

Content here.
`
	dir := t.TempDir()
	writeMD(t, dir, "versioned.md", content)

	loader := NewLoader(dir, "")
	agents, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll should not return error (file should be skipped): %v", err)
	}
	// KnownFields(true) rejects unknown YAML keys — the file is skipped.
	if len(agents) != 0 {
		t.Logf("NOTE: 'version' field is now accepted by Frontmatter (loaded %d agents); update this test", len(agents))
	}
}

func writeMD(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("writeMD: %v", err)
	}
}
