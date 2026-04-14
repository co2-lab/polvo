// Package patch saves unified diffs of agent-modified files to .polvo/patches/.
package patch

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Writer saves patch files for agent executions.
type Writer struct {
	Root string // project root
}

// New returns a Writer rooted at root.
func New(root string) *Writer {
	return &Writer{Root: root}
}

// Write generates a unified diff of the modified files and saves it to
// .polvo/patches/<timestamp_ns>-<agentName>.patch.
//
// If the project root is a git repo, uses `git diff HEAD -- <files>`.
// Falls back to an empty patch with a comment if git is unavailable or no changes.
//
// Returns the path to the saved patch file.
func (w *Writer) Write(ctx context.Context, agentName string, files []string) (string, error) {
	if len(files) == 0 {
		return "", nil
	}

	patchDir := filepath.Join(w.Root, ".polvo", "patches")
	if err := os.MkdirAll(patchDir, 0o755); err != nil {
		return "", fmt.Errorf("creating patches dir: %w", err)
	}

	content := w.generateDiff(ctx, files)

	ts := time.Now().UnixNano()
	safe := sanitizeName(agentName)
	filename := fmt.Sprintf("%d-%s.patch", ts, safe)
	patchPath := filepath.Join(patchDir, filename)

	// Atomic write: write to temp then rename
	tmp := patchPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("writing patch file: %w", err)
	}
	if err := os.Rename(tmp, patchPath); err != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("renaming patch file: %w", err)
	}

	return patchPath, nil
}

// generateDiff produces a unified diff string for the given files.
// Uses `git diff HEAD` if the project is a git repo.
func (w *Writer) generateDiff(ctx context.Context, files []string) string {
	if isGitRepo(w.Root) {
		return w.gitDiff(ctx, files)
	}
	// Non-git: return a minimal patch header with the file list
	var sb strings.Builder
	sb.WriteString("# polvo patch — no git repo, listing modified files\n")
	for _, f := range files {
		sb.WriteString("# modified: ")
		sb.WriteString(f)
		sb.WriteString("\n")
	}
	return sb.String()
}

func (w *Writer) gitDiff(ctx context.Context, files []string) string {
	// Stage all changes first to catch untracked new files too
	args := append([]string{"-C", w.Root, "add", "--"}, files...)
	exec.CommandContext(ctx, "git", args...).Run() //nolint:errcheck — best effort

	diffArgs := append([]string{"-C", w.Root, "diff", "--cached", "--"}, files...)
	out, err := exec.CommandContext(ctx, "git", diffArgs...).Output()
	if err != nil || len(out) == 0 {
		// Fallback: unstaged diff
		diffArgs2 := append([]string{"-C", w.Root, "diff", "HEAD", "--"}, files...)
		out, _ = exec.CommandContext(ctx, "git", diffArgs2...).Output()
	}
	return string(out)
}

// isGitRepo returns true if root contains a .git directory or file.
func isGitRepo(root string) bool {
	_, err := os.Stat(filepath.Join(root, ".git"))
	return err == nil
}

// sanitizeName replaces characters unsafe for filenames.
func sanitizeName(name string) string {
	r := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-")
	s := r.Replace(name)
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}
