package clidetect

import (
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

// CLI representa um CLI de IA detectado no sistema.
type CLI struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Command string `json:"command"`
}

// KnownCLIs é a lista de CLIs de IA conhecidos para detecção.
var KnownCLIs = []CLI{
	{ID: "claude", Label: "Claude", Command: "claude"},
	{ID: "gemini", Label: "Gemini", Command: "gemini"},
	{ID: "copilot", Label: "Copilot", Command: "copilot"},
	{ID: "aider", Label: "Aider", Command: "aider"},
	{ID: "continue", Label: "Continue", Command: "continue"},
}

// extraPaths returns additional directories to search beyond the process PATH.
// Covers tools installed via nvm, homebrew, cargo, etc. that may be missing
// from the Tauri sidecar's restricted environment.
func extraPaths() []string {
	var paths []string

	if u, err := user.Current(); err == nil {
		home := u.HomeDir

		if runtime.GOOS == "windows" {
			// Windows: nvm-windows stores versions under %APPDATA%\nvm
			appData := os.Getenv("APPDATA")
			if appData != "" {
				nvmBase := filepath.Join(appData, "nvm")
				if entries, err := os.ReadDir(nvmBase); err == nil {
					for _, e := range entries {
						if e.IsDir() {
							paths = append(paths, filepath.Join(nvmBase, e.Name()))
						}
					}
				}
			}
			// npm global bin (node installed directly, not via nvm)
			if appData != "" {
				paths = append(paths, filepath.Join(appData, "npm"))
			}
			paths = append(paths,
				filepath.Join(home, "go", "bin"),
				filepath.Join(home, ".cargo", "bin"),
				`C:\Program Files\nodejs`,
			)
		} else {
			// macOS / Linux
			// nvm: ~/.nvm/versions/node/*/bin
			nvmBase := filepath.Join(home, ".nvm", "versions", "node")
			if entries, err := os.ReadDir(nvmBase); err == nil {
				for _, e := range entries {
					if e.IsDir() {
						paths = append(paths, filepath.Join(nvmBase, e.Name(), "bin"))
					}
				}
			}
			paths = append(paths,
				filepath.Join(home, ".local", "bin"),
				filepath.Join(home, "go", "bin"),
				filepath.Join(home, ".cargo", "bin"),
				"/usr/local/bin",
				"/opt/homebrew/bin",  // macOS Apple Silicon
				"/opt/homebrew/sbin",
				"/usr/local/sbin",
			)
		}
	}

	return paths
}

// findCommand resolves the full path of a command, checking both the process
// PATH and the extra paths above. On Windows it also tries the .exe suffix.
func findCommand(cmd string) (string, bool) {
	if p, err := exec.LookPath(cmd); err == nil {
		return p, true
	}
	candidates := []string{cmd}
	if runtime.GOOS == "windows" && !strings.HasSuffix(cmd, ".exe") {
		candidates = append(candidates, cmd+".exe")
	}
	for _, dir := range extraPaths() {
		for _, name := range candidates {
			full := filepath.Join(dir, name)
			if info, err := os.Stat(full); err == nil && !info.IsDir() {
				return full, true
			}
		}
	}
	return "", false
}

// Detect retorna a lista de CLIs disponíveis no sistema.
func Detect() []CLI {
	// Augment the process PATH so exec.LookPath also benefits from extra dirs
	current := os.Getenv("PATH")
	extra := strings.Join(extraPaths(), string(os.PathListSeparator))
	_ = os.Setenv("PATH", current+string(os.PathListSeparator)+extra)

	var found []CLI
	for _, cli := range KnownCLIs {
		if full, ok := findCommand(cli.Command); ok {
			entry := cli
			entry.Command = full
			found = append(found, entry)
		}
	}
	return found
}
