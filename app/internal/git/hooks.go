package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/co2-lab/polvo/internal/assets"
)

// InstallHooks installs polvo's git hooks into the repo's .git/hooks directory.
// Existing hooks that already contain polvo's content are left unchanged.
// If a hook already exists with different content, polvo's check is appended.
func InstallHooks(ctx context.Context, workDir string) error {
	client := &ExecClient{WorkDir: workDir}
	if !client.IsGitRepo(ctx) {
		return nil // not a git repo, nothing to do
	}

	root, err := client.RepoRoot(ctx)
	if err != nil {
		return fmt.Errorf("finding repo root: %w", err)
	}

	hooksDir := filepath.Join(root, ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return fmt.Errorf("creating hooks dir: %w", err)
	}

	for _, hookName := range []string{"pre-commit"} {
		if err := installHook(hooksDir, hookName); err != nil {
			return err
		}
	}
	return nil
}

func installHook(hooksDir, name string) error {
	content, err := assets.ReadHook(name)
	if err != nil {
		return fmt.Errorf("reading embedded hook %q: %w", name, err)
	}

	hookPath := filepath.Join(hooksDir, name)

	// If hook already exists, check if it already has our content.
	if existing, readErr := os.ReadFile(hookPath); readErr == nil {
		if string(existing) == string(content) {
			return nil // already up to date
		}
		// Different content — append polvo's check rather than overwriting.
		combined := string(existing) + "\n# --- polvo hook ---\n" + string(content)
		return os.WriteFile(hookPath, []byte(combined), 0o755)
	}

	// Hook doesn't exist — create it.
	return os.WriteFile(hookPath, content, 0o755)
}
