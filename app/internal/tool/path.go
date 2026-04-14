package tool

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Ignorer is implemented by ignore.Set to check whether a path should be blocked.
type Ignorer interface {
	Ignored(absPath string) bool
}

// checkIgnored returns an error if path is on the ignore list.
func checkIgnored(ig Ignorer, path string) error {
	if ig != nil && ig.Ignored(path) {
		return fmt.Errorf("path %q is excluded by .polvoignore", filepath.Base(path))
	}
	return nil
}

// resolveLexical computes the absolute path lexically (no symlink resolution).
// Used for ignore checks so that .polvoignore entries match the logical path.
func resolveLexical(workdir, path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	wdClean := filepath.Clean(workdir)
	var absPath string
	if filepath.IsAbs(path) {
		absPath = filepath.Clean(path)
	} else {
		absPath = filepath.Clean(filepath.Join(workdir, path))
	}
	if !strings.HasPrefix(absPath, wdClean+string(filepath.Separator)) && absPath != wdClean {
		if filepath.IsAbs(path) {
			return "", fmt.Errorf("path %q is outside working directory", path)
		}
		return "", fmt.Errorf("path %q escapes working directory", path)
	}
	return absPath, nil
}

// securePath resolves path relative to workdir, rejecting traversal.
// It performs both a lexical check and a symlink-resolution check to prevent
// symlinks inside workdir from pointing to paths outside it.
func securePath(workdir, path string) (string, error) {
	wdClean := filepath.Clean(workdir)

	var absPath string
	if filepath.IsAbs(path) {
		absPath = filepath.Clean(path)
	} else {
		absPath = filepath.Clean(filepath.Join(workdir, path))
	}

	// Lexical containment check.
	if !strings.HasPrefix(absPath, wdClean+string(filepath.Separator)) && absPath != wdClean {
		if filepath.IsAbs(path) {
			return "", fmt.Errorf("path %q is outside working directory", path)
		}
		return "", fmt.Errorf("path %q escapes working directory", path)
	}

	// Symlink resolution: resolve the real path to catch symlinks that point outside.
	// If EvalSymlinks fails (e.g., file does not exist yet), fall back to the
	// lexical result — new files cannot be symlinks.
	if real, err := filepath.EvalSymlinks(absPath); err == nil {
		realWd, wdErr := filepath.EvalSymlinks(wdClean)
		if wdErr != nil {
			realWd = wdClean
		}
		realWd = filepath.Clean(realWd)
		real = filepath.Clean(real)
		if !strings.HasPrefix(real, realWd+string(filepath.Separator)) && real != realWd {
			return "", fmt.Errorf("path %q resolves via symlink to %q which is outside working directory", path, real)
		}
		return real, nil
	}

	// Check that at least the parent exists and is inside workdir (best-effort for new files).
	parent := filepath.Dir(absPath)
	if realParent, err := filepath.EvalSymlinks(parent); err == nil {
		realWd, wdErr := filepath.EvalSymlinks(wdClean)
		if wdErr != nil {
			realWd = wdClean
		}
		realWd = filepath.Clean(realWd)
		realParent = filepath.Clean(realParent)
		if !strings.HasPrefix(realParent, realWd+string(filepath.Separator)) && realParent != realWd {
			return "", fmt.Errorf("path %q parent resolves via symlink to %q which is outside working directory", path, realParent)
		}
	}

	return absPath, nil
}
