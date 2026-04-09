package config

// DefaultConfig returns the default configuration values.
func DefaultConfig() *Config {
	return &Config{
		Review: ReviewConfig{
			Gates:      []string{"lint", "best-practices"},
			MaxRetries: 3,
			AutoMerge:  true,
		},
		Git: GitConfig{
			BranchPrefix: "polvo/",
			PRLabels:     []string{"polvo", "automated"},
			TargetBranch: "main",
		},
		Settings: SettingsConfig{
			DebounceMs:  500,
			ReportDir:   ".polvo/reports",
			LogLevel:    "info",
			MaxParallel: 2,
		},
	}
}
