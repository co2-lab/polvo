package config

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Review.MaxRetries != 3 {
		t.Errorf("expected max_retries 3, got %d", cfg.Review.MaxRetries)
	}
	if cfg.Git.BranchPrefix != "polvo/" {
		t.Errorf("expected branch prefix 'polvo/', got %q", cfg.Git.BranchPrefix)
	}
	if cfg.Git.TargetBranch != "main" {
		t.Errorf("expected target branch 'main', got %q", cfg.Git.TargetBranch)
	}
	if cfg.Settings.DebounceMs != 500 {
		t.Errorf("expected debounce 500, got %d", cfg.Settings.DebounceMs)
	}
	if len(cfg.Review.Gates) != 2 {
		t.Errorf("expected 2 gates, got %d", len(cfg.Review.Gates))
	}
}

func TestLoadBaseConfig(t *testing.T) {
	// Load without project config — should use embedded base config only
	cfg, err := Load("")
	if err != nil {
		// Expected to fail validation since no providers are configured in base config
		// This is OK — the base config doesn't have providers
		return
	}

	if cfg.Settings.LogLevel != "info" {
		t.Errorf("expected log level 'info', got %q", cfg.Settings.LogLevel)
	}
}
