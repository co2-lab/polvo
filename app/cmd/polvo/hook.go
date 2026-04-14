package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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
	cwd, err := os.Getwd()
	if err != nil {
		return nil // can't determine cwd, skip silently
	}

	cfg, _ := config.LoadWithUser("polvo.yaml")

	hook := config.PreCommitHookConfig{
		Enabled:        true,
		CheckPolvoYAML: true,
		SecretsScan:    true,
		AIScan:         false,
	}
	if cfg != nil {
		hook = cfg.Hooks.PreCommit
	}

	if !hook.Enabled {
		return nil
	}

	_ = cwd

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
		if err := checkSecretsInDiff(diff); err != nil {
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

// checkSecretsInDiff runs regex + entropy scan on added lines in the diff.
func checkSecretsInDiff(diff string) error {
	// Only scan added lines (lines starting with '+' but not '+++').
	var added strings.Builder
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			added.WriteString(line[1:]) // strip leading '+'
			added.WriteByte('\n')
		}
	}

	content := added.String()
	if content == "" {
		return nil
	}

	result := secrets.MaskSecretsDetailed(content)
	if len(result.Redactions) == 0 {
		return nil
	}

	// Build a summary of what was found.
	seen := make(map[string]bool)
	var findings []string
	for _, r := range result.Redactions {
		if !seen[r.Pattern] {
			seen[r.Pattern] = true
			findings = append(findings, r.Pattern)
		}
	}

	return fmt.Errorf(`
polvo: commit blocked — possible secrets detected in staged changes

  Detected patterns: %s

  Review your changes and remove any credentials before committing.
  If this is a false positive, use: git commit --no-verify`,
		strings.Join(findings, ", "))
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
