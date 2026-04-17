package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/co2-lab/polvo/internal/config"
	"github.com/co2-lab/polvo/internal/provider"
	"github.com/co2-lab/polvo/internal/secrets"
)

func runHook(name string) error {
	switch name {
	case "pre-commit":
		return runPreCommitHook()
	default:
		return fmt.Errorf("unknown hook: %q", name)
	}
}

func runPreCommitHook() error {
	// Resolve the git repo root so we can find polvo.yaml regardless of where
	// the hook process is invoked from.
	rootBytes, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return nil // not in a git repo, skip silently
	}
	repoRoot := strings.TrimSpace(string(rootBytes))
	projectConfig := filepath.Join(repoRoot, "polvo.yaml")

	cfg, _ := config.LoadWithUser(projectConfig)

	// Defaults come from the embedded config.yaml (enabled: true, secrets_scan: true, etc.).
	// If config loading failed entirely, fall back to safe defaults.
	hook := config.PreCommitHookConfig{
		Enabled:        true,
		CheckPolvoYAML: true,
		SecretsScan:    true,
	}
	if cfg != nil {
		hook = cfg.Hooks.PreCommit
	}

	if !hook.Enabled {
		return nil
	}

	// --- Check 1: api_key hardcoded in polvo.yaml ---
	if hook.CheckPolvoYAML {
		if err := checkPolvoYAMLStaged(); err != nil {
			return err
		}
	}

	// Get the full staged diff for the remaining checks.
	diff, err := stagedDiff()
	if err != nil || diff == "" {
		return nil // nothing staged or git unavailable
	}

	// --- Check 2: regex + entropy secrets scan ---
	if hook.SecretsScan {
		if err := checkSecretsInDiff(diff, hook.SecretsScanIgnore); err != nil {
			return err
		}
	}

	// --- Check 3: AI-powered scan (opt-in) ---
	if hook.AIScan {
		if err := checkSecretsWithAI(diff, cfg, hook.AIProvider); err != nil {
			return err
		}
	}

	return nil
}

// checkPolvoYAMLStaged checks if polvo.yaml is staged and contains a hardcoded api_key.
func checkPolvoYAMLStaged() error {
	staged, err := exec.Command("git", "diff", "--cached", "--name-only").Output()
	if err != nil {
		return nil
	}
	if !strings.Contains(string(staged), "polvo.yaml") {
		return nil
	}

	content, err := exec.Command("git", "show", ":polvo.yaml").Output()
	if err != nil {
		return nil
	}

	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "api_key:") {
			continue
		}
		val := strings.TrimSpace(strings.TrimPrefix(trimmed, "api_key:"))
		if val == "" || strings.HasPrefix(val, "$") {
			continue
		}
		return fmt.Errorf(`
polvo: commit blocked — hardcoded api_key in polvo.yaml

  API keys must not be committed to git.
  Move credentials to ~/.polvo/config.yaml (never committed):

    providers:
      <name>:
        api_key: ${YOUR_ENV_VAR}

  To bypass (not recommended): git commit --no-verify`)
	}
	return nil
}

// stagedDiff returns the full staged diff as a string.
func stagedDiff() (string, error) {
	out, err := exec.Command("git", "diff", "--cached", "--unified=3").Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// fileFinding records a secret detection hit with file context.
type fileFinding struct {
	file    string
	lineNum int
	pattern string
	preview string // first 60 chars of the offending line
}

// isSecretScanSkipped reports whether path matches any of the user-configured
// ignore glob patterns. Supports ** (matches any number of path segments),
// * (matches within a single segment), and ? (matches a single character).
func isSecretScanSkipped(path string, ignoreGlobs []string) bool {
	for _, pattern := range ignoreGlobs {
		if globMatch(pattern, path) {
			return true
		}
	}
	return false
}

// globMatch matches path against pattern with support for **.
// ** matches zero or more path segments; * matches within a single segment.
func globMatch(pattern, path string) bool {
	// Normalise separators.
	pattern = filepath.ToSlash(pattern)
	path = filepath.ToSlash(path)

	patParts := strings.Split(pattern, "/")
	pathParts := strings.Split(path, "/")
	return globMatchParts(patParts, pathParts)
}

func globMatchParts(patParts, pathParts []string) bool {
	for len(patParts) > 0 {
		if patParts[0] == "**" {
			patParts = patParts[1:]
			if len(patParts) == 0 {
				return true // ** at end matches everything remaining
			}
			// Try matching the rest of the pattern against every suffix of pathParts.
			for i := range pathParts {
				if globMatchParts(patParts, pathParts[i:]) {
					return true
				}
			}
			return false
		}
		if len(pathParts) == 0 {
			return false
		}
		ok, _ := filepath.Match(patParts[0], pathParts[0])
		if !ok {
			return false
		}
		patParts = patParts[1:]
		pathParts = pathParts[1:]
	}
	return len(pathParts) == 0
}

// checkSecretsInDiff runs regex + entropy scan on added lines in the diff,
// tracking which file and line number each finding comes from.
// ignoreGlobs is the list of path patterns configured via secrets_scan_ignore.
func checkSecretsInDiff(diff string, ignoreGlobs []string) error {
	var findings []fileFinding
	currentFile := ""
	diffLineNum := 0 // line number within the file (from @@ hunk header)

	for _, line := range strings.Split(diff, "\n") {
		// Track current file from diff headers.
		if strings.HasPrefix(line, "+++ b/") {
			currentFile = strings.TrimPrefix(line, "+++ b/")
			diffLineNum = 0
			continue
		}
		// Parse hunk headers to track line numbers: @@ -a,b +c,d @@
		if strings.HasPrefix(line, "@@") {
			// Extract the '+' side start line: @@ -old +new,count @@
			var newStart int
			if _, err := fmt.Sscanf(line, "@@ -%*d,%*d +%d", &newStart); err != nil {
				fmt.Sscanf(line, "@@ -%*d +%d", &newStart) //nolint:errcheck
			}
			diffLineNum = newStart - 1 // will be incremented on first '+' or ' ' line
			continue
		}
		if strings.HasPrefix(line, "-") {
			continue // deleted lines don't count toward new file line numbers
		}
		if strings.HasPrefix(line, " ") {
			diffLineNum++
			continue
		}
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			diffLineNum++
			if isSecretScanSkipped(currentFile, ignoreGlobs) {
				continue
			}
			content := line[1:] // strip leading '+'
			result := secrets.MaskSecretsDetailed(content)
			for _, r := range result.Redactions {
				preview := content
				if len(preview) > 60 {
					preview = preview[:60] + "..."
				}
				findings = append(findings, fileFinding{
					file:    currentFile,
					lineNum: diffLineNum,
					pattern: r.Pattern,
					preview: strings.TrimSpace(preview),
				})
			}
		}
	}

	if len(findings) == 0 {
		return nil
	}

	// Deduplicate: one entry per (file, pattern), keeping lowest line number.
	type key struct{ file, pattern string }
	seen := make(map[key]fileFinding)
	for _, f := range findings {
		k := key{f.file, f.pattern}
		if _, ok := seen[k]; !ok {
			seen[k] = f
		}
	}

	var lines []string
	for _, f := range seen {
		lines = append(lines, fmt.Sprintf("  %s:%d  [%s]  %s", f.file, f.lineNum, f.pattern, f.preview))
	}
	sort.Strings(lines)

	return fmt.Errorf(`
polvo: commit blocked — possible secrets detected in staged changes

%s

  Review your changes and remove any credentials before committing.
  If this is a false positive, use: git commit --no-verify`,
		strings.Join(lines, "\n"))
}

// checkSecretsWithAI sends the staged diff to an LLM for secret detection.
func checkSecretsWithAI(diff string, cfg *config.Config, providerName string) error {
	if cfg == nil || len(cfg.Providers) == 0 {
		fmt.Fprintln(os.Stderr, "polvo: ai_scan enabled but no provider configured — skipping")
		return nil
	}

	registry, err := provider.NewRegistry(cfg.Providers)
	if err != nil {
		fmt.Fprintf(os.Stderr, "polvo: ai_scan: provider error: %v — skipping\n", err)
		return nil
	}

	var p provider.LLMProvider
	if providerName != "" {
		p, err = registry.Get(providerName)
	} else {
		p, err = registry.Default()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "polvo: ai_scan: no provider available (%v) — skipping\n", err)
		return nil
	}

	cp, ok := p.(provider.ChatProvider)
	if !ok {
		fmt.Fprintln(os.Stderr, "polvo: ai_scan: provider does not support chat — skipping")
		return nil
	}

	// Truncate very large diffs to avoid excessive token usage.
	const maxDiffChars = 8000
	scanDiff := diff
	truncated := false
	if len(scanDiff) > maxDiffChars {
		scanDiff = diff[:maxDiffChars]
		truncated = true
	}

	prompt := buildAIScanPrompt(scanDiff, truncated)

	ctx := context.Background()
	msgs := []provider.Message{
		{Role: "user", Content: prompt},
	}

	resp, err := cp.Chat(ctx, provider.ChatRequest{
		Model:     "", // uses provider default
		Messages:  msgs,
		MaxTokens: 512,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "polvo: ai_scan: request failed: %v — skipping\n", err)
		return nil
	}

	answer := strings.TrimSpace(resp.Message.Content)

	// Expected response: first token is SAFE or UNSAFE optionally followed by explanation.
	upper := strings.ToUpper(answer)
	if strings.HasPrefix(upper, "UNSAFE") {
		explanation := strings.TrimSpace(answer[6:])
		if explanation == "" {
			explanation = "(no details provided)"
		}
		return fmt.Errorf(`
polvo: commit blocked — AI scan detected potential secrets

  %s

  Review your staged changes and remove credentials before committing.
  To bypass (not recommended): git commit --no-verify`, explanation)
	}

	return nil
}

func buildAIScanPrompt(diff string, truncated bool) string {
	note := ""
	if truncated {
		note = "\n(diff was truncated to fit context window)"
	}
	return fmt.Sprintf(`You are a security scanner. Analyze the following git diff and determine if it contains any secrets, credentials, or sensitive data such as:
- API keys or tokens (any provider)
- Passwords or passphrases
- Private keys (RSA, EC, SSH, PGP)
- Database connection strings with credentials
- OAuth client secrets
- JWT secrets or signing keys
- Any other hardcoded credentials

Respond with exactly one of:
- SAFE — if no secrets are found
- UNSAFE: <brief explanation> — if secrets are found, describing what was detected

Do not explain your reasoning beyond the single-line response. Do not be cautious — only flag actual secrets, not variable names or placeholder values like "your-api-key-here".%s

Diff:
%s`, note, diff)
}
