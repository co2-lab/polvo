package tool

import (
	"fmt"
	"path/filepath"
	"strings"
)

// securePath resolves path relative to workdir, rejecting traversal.
func securePath(workdir, path string) (string, error) {
	if filepath.IsAbs(path) {
		// Absolute path must be under workdir
		clean := filepath.Clean(path)
		wdClean := filepath.Clean(workdir)
		if !strings.HasPrefix(clean, wdClean+string(filepath.Separator)) && clean != wdClean {
			return "", fmt.Errorf("path %q is outside working directory", path)
		}
		return clean, nil
	}

	// Relative path — join with workdir
	joined := filepath.Join(workdir, path)
	clean := filepath.Clean(joined)
	wdClean := filepath.Clean(workdir)

	if !strings.HasPrefix(clean, wdClean+string(filepath.Separator)) && clean != wdClean {
		return "", fmt.Errorf("path %q escapes working directory", path)
	}

	return clean, nil
}
