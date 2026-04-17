package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"
	goyaml "gopkg.in/yaml.v3"

	"github.com/co2-lab/polvo/internal/assets"
)

// Config is the top-level Polvo configuration.
type Config struct {
	Project     ProjectConfig                   `koanf:"project"`
	Providers   map[string]ProviderConfig       `koanf:"providers"`
	Interfaces  map[string]InterfaceGroupConfig  `koanf:"interfaces"`
	Watchers    map[string]WatcherConfig         `koanf:"watchers"`
	Agents      AgentsConfig                    `koanf:"agents"`
	Guides      map[string]GuideConfig           `koanf:"guides"`
	Chain       ChainConfig                     `koanf:"chain"`
	Review      ReviewConfig                    `koanf:"review"`
	Git         GitConfig                       `koanf:"git"`
	Settings    SettingsConfig                  `koanf:"settings"`
	Permissions PermissionsConfig               `koanf:"permissions"`
	Hooks       HooksConfig                     `koanf:"hooks"`
}

// ProjectConfig holds project metadata.
type ProjectConfig struct {
	Name  string `koanf:"name"`
	Color string `koanf:"color"`
	Icon  string `koanf:"icon"`
}

// ModelRoleConfig configures a model for a specific semantic role.
type ModelRoleConfig struct {
	Model                  string `koanf:"model"`
	MaxInputTokens         int    `koanf:"max_input_tokens"`
	ReservedOutputTokens   int    `koanf:"reserved_output_tokens"`
	ContextFallbackModel   string `koanf:"context_fallback"`  // model with larger context window
	ErrorFallbackProvider  string `koanf:"error_fallback"`    // provider name to use on API error
}

// ProviderRoles maps semantic roles to model configs.
type ProviderRoles struct {
	Primary ModelRoleConfig `koanf:"primary"`
	Review  ModelRoleConfig `koanf:"review"`
	Summary ModelRoleConfig `koanf:"summary"`
	Embed   ModelRoleConfig `koanf:"embed"`
}

// ContextFallbackConfig configures the context window fallback cascade.
type ContextFallbackConfig struct {
	// Map: model name → ordered list of fallback models with larger context.
	ContextWindowFallbacks map[string][]string `koanf:"context_window_fallbacks"`
	// MaxFallbackDepth is the maximum number of fallback hops (default 3).
	MaxFallbackDepth int `koanf:"max_fallback_depth"`
	// MinOutputTokens is the minimum token budget reserved for output (default 1000).
	MinOutputTokens int `koanf:"min_output_tokens"`
}

// ProviderConfig defines an LLM provider.
type ProviderConfig struct {
	Type            string                `koanf:"type" validate:"required,oneof=ollama claude openai gemini deepseek groq mistral openrouter xai openai-compatible glm minimax kimi"`
	APIKey          string                `koanf:"api_key"`
	BaseURL         string                `koanf:"base_url"`
	DefaultModel    string                `koanf:"default_model"` // used when no role config set
	Roles           ProviderRoles         `koanf:"roles"`
	RetryMax        int                   `koanf:"retry_max"`      // default 3
	RetryMinWait    int                   `koanf:"retry_min_wait"` // seconds, default 2
	RetryMaxWait    int                   `koanf:"retry_max_wait"` // seconds, default 30
	ContextFallback ContextFallbackConfig `koanf:"context_fallback"`
}

// Default templates for derived files.
const (
	DefaultSpecTemplate     = "{{dir}}/{{name}}.spec.md"
	DefaultFeaturesTemplate = "{{dir}}/{{name}}.feature"
	DefaultTestsTemplate    = "{{dir}}/{{name}}.test.{{ext}}"
)

// InterfaceGroupConfig defines a group of interfaces with their own settings.
type InterfaceGroupConfig struct {
	Patterns []string               `koanf:"patterns" validate:"required"`
	Derived  DerivedConfig          `koanf:"derived"`
	Provider string                 `koanf:"provider"`
	Model    string                 `koanf:"model"`
	Guides   map[string]GuideConfig `koanf:"guides"`
}

// GetGuideConfig returns the merged guide configuration for this group.
func (g *InterfaceGroupConfig) GetGuideConfig(guideName string, globalGuides map[string]GuideConfig) GuideConfig {
	cfg := globalGuides[guideName] // Start with global config

	if g.Guides != nil {
		if groupCfg, ok := g.Guides[guideName]; ok {
			// Override fields if set in group
			if groupCfg.Mode != "" {
				cfg.Mode = groupCfg.Mode
			}
			if groupCfg.File != "" {
				cfg.File = groupCfg.File
			}
			if groupCfg.Provider != "" {
				cfg.Provider = groupCfg.Provider
			}
			if groupCfg.Model != "" {
				cfg.Model = groupCfg.Model
			}
			if groupCfg.Prompt != "" {
				cfg.Prompt = groupCfg.Prompt
			}
			if groupCfg.Role != "" {
				cfg.Role = groupCfg.Role
			}
			if groupCfg.UseTools {
				cfg.UseTools = true
			}
		}
	}

	// Apply group-level provider/model if guide doesn't have its own
	if cfg.Provider == "" {
		cfg.Provider = g.Provider
	}
	if cfg.Model == "" {
		cfg.Model = g.Model
	}

	return cfg
}

// ResolveSpecPath returns the path to the spec file for the given file.
func (g *InterfaceGroupConfig) ResolveSpecPath(filePath string) string {
	tmpl := g.Derived.Spec
	if tmpl == "" {
		tmpl = DefaultSpecTemplate
	}
	return g.ResolveDerived(filePath, tmpl)
}

// ResolveFeaturesPath returns the path to the features file for the given file.
func (g *InterfaceGroupConfig) ResolveFeaturesPath(filePath string) string {
	tmpl := g.Derived.Features
	if tmpl == "" {
		tmpl = DefaultFeaturesTemplate
	}
	return g.ResolveDerived(filePath, tmpl)
}

// ResolveTestsPath returns the path to the tests file for the given file.
func (g *InterfaceGroupConfig) ResolveTestsPath(filePath string) string {
	tmpl := g.Derived.Tests
	if tmpl == "" {
		tmpl = DefaultTestsTemplate
	}
	return g.ResolveDerived(filePath, tmpl)
}

// ResolveDerived resolves a derived path template for a given file.
func (g *InterfaceGroupConfig) ResolveDerived(filePath string, tmpl string) string {
	if tmpl == "" {
		return ""
	}
	dir := filepath.Dir(filePath)
	ext := filepath.Ext(filePath)
	name := strings.TrimSuffix(filepath.Base(filePath), ext)

	r := strings.NewReplacer(
		"{{dir}}", dir,
		"{{name}}", name,
		"{{ext}}", strings.TrimPrefix(ext, "."),
	)
	return r.Replace(tmpl)
}

// DerivedConfig defines naming conventions for derived files.
type DerivedConfig struct {
	Spec     string `koanf:"spec"`
	Features string `koanf:"features"`
	Tests    string `koanf:"tests"`
}

// ArchitectEditorConfig configures the two-phase architect/editor loop for a guide.
type ArchitectEditorConfig struct {
	Enabled           bool   `koanf:"enabled"`
	ArchitectRole     string `koanf:"architect_role"`
	EditorRole        string `koanf:"editor_role"`
	ArchitectModel    string `koanf:"architect_model"`
	EditorModel       string `koanf:"editor_model"`
	MaxArchitectTurns int    `koanf:"max_architect_turns"`
	MaxEditorTurns    int    `koanf:"max_editor_turns"`
}

// GuideConfig configures a single guide.
type GuideConfig struct {
	Mode            string                `koanf:"mode" validate:"omitempty,oneof=extend replace"`
	File            string                `koanf:"file"`
	Provider        string                `koanf:"provider"`
	Model           string                `koanf:"model"`
	Prompt          string                `koanf:"prompt"`
	Role            string                `koanf:"role" validate:"omitempty,oneof=author reviewer"`
	UseTools        bool                  `koanf:"use_tools"`
	ArchitectEditor ArchitectEditorConfig `koanf:"architect_editor"`
}

// ChainConfig defines the reaction chain.
type ChainConfig struct {
	Steps []ChainStep `koanf:"steps"`
}

// ChainStep defines a single step in the chain.
type ChainStep struct {
	Trigger string   `koanf:"trigger"`
	Agent   string   `koanf:"agent"`
	Role    string   `koanf:"role"`
	Gates   []string `koanf:"gates"`
	Next    string   `koanf:"next"`
}

// ReviewConfig controls the review process.
type ReviewConfig struct {
	Gates      []string `koanf:"gates"`
	MaxRetries int      `koanf:"max_retries"`
	AutoMerge  bool     `koanf:"auto_merge"`
}

// GitConfig holds git settings.
type GitConfig struct {
	BranchPrefix    string   `koanf:"branch_prefix"`
	PRLabels        []string `koanf:"pr_labels"`
	TargetBranch    string   `koanf:"target_branch"`
	AutoCommit      bool     `koanf:"auto_commit"`
	DirtyCommit     bool     `koanf:"dirty_commit"`
	BranchPerRun    bool     `koanf:"branch_per_run"`
	BranchTemplate  string   `koanf:"branch_template"`
	Attribution     string   `koanf:"attribution"`
	PushAfterCommit bool     `koanf:"push_after_commit"` // push to remote after auto-commit
	PROnCompletion  bool     `koanf:"pr_on_completion"`  // open PR after push
	PRDraft         bool     `koanf:"pr_draft"`          // default true (conservative)
	PRBase          string   `koanf:"pr_base"`           // default "main"
	PRTitleTemplate string   `koanf:"pr_title_template"` // "{{agent}}: {{task_short}}"
	RemoteName      string   `koanf:"remote_name"`       // default "origin"
}

// AuditConfig holds audit storage settings.
type AuditConfig struct {
	Enabled       bool   `koanf:"enabled"`
	DBPath        string `koanf:"db_path"`        // default ".polvo/audit.db"
	RetentionDays int    `koanf:"retention_days"` // 0 = no pruning
}

// WatcherConfig defines a named file watcher.
type WatcherConfig struct {
	Path    string   `koanf:"path"`
	Pattern []string `koanf:"pattern"` // supports "!" negation prefix
	Agents  []string `koanf:"agents"`
}

// StuckDetectionConfig configures stuck detection in the agent loop.
type StuckDetectionConfig struct {
	Enabled    bool `koanf:"enabled"`
	WindowSize int  `koanf:"window_size"` // default 6
	Threshold  int  `koanf:"threshold"`   // default 3
}

// ReflectionConfig configures the internal reflection loop.
type ReflectionConfig struct {
	Enabled    bool     `koanf:"enabled"`
	MaxRetries int      `koanf:"max_retries"` // default 3
	Commands   []string `koanf:"commands"`
}

// AgentRunConfig holds per-agent loop settings.
type AgentRunConfig struct {
	Autonomy   string           `koanf:"autonomy" validate:"omitempty,oneof=full supervised plan"`
	RunTimeout int              `koanf:"run_timeout"` // seconds, 0 = use global
	MaxTurns   int              `koanf:"max_turns"`   // 0 = use global
	Reflection ReflectionConfig `koanf:"reflection"`
}

// SettingsConfig holds global settings.
type SettingsConfig struct {
	DebounceMs          int                  `koanf:"debounce_ms"`
	ReportDir           string               `koanf:"report_dir"`
	LogLevel            string               `koanf:"log_level" validate:"omitempty,oneof=debug info warn error"`
	MaxParallel         int                  `koanf:"max_parallel"`
	BraveAPIKey         string               `koanf:"brave_api_key"`
	Autonomy            string               `koanf:"autonomy" validate:"omitempty,oneof=full supervised plan"` // global default
	RunTimeout          int                  `koanf:"run_timeout"` // seconds, default 1800
	ToolTimeout         int                  `koanf:"tool_timeout"` // seconds per tool, default 120
	PersistentBashSession bool               `koanf:"persistent_bash_session"` // when true each agent uses a long-lived bash session
	BashMaxCPUSecs    int                  `koanf:"bash_max_cpu_secs"`    // 0 = disabled
	BashMaxMemMB      int                  `koanf:"bash_max_mem_mb"`       // 0 = disabled
	BashMaxFileSizeMB int                  `koanf:"bash_max_file_size_mb"` // 0 = disabled
	StuckDetection               StuckDetectionConfig `koanf:"stuck_detection"`
	SupervisorModel               string  `koanf:"supervisor_model"`
	SupervisorConfidenceThreshold float64 `koanf:"supervisor_confidence_threshold"`
	// SummaryModel is a cheap model used for on-demand summarisation (turn marks,
	// session work items, context fallback). When empty, the main model generates
	// inline summaries via <summary> tags instead.
	SummaryModel string `koanf:"summary_model"`
}

// AgentsConfig holds per-agent overrides.
type AgentsConfig map[string]AgentRunConfig

// PermissionsConfig controls tool execution permissions.
type PermissionsConfig struct {
	Rules     []PermissionRule `koanf:"rules"`
	Blocklist []string         `koanf:"blocklist"` // command prefix/exact patterns to block in bash
	Audit     AuditConfig      `koanf:"audit"`
}

// HooksConfig configures git hook behavior and agent lifecycle hooks.
type HooksConfig struct {
	PreCommit PreCommitHookConfig `koanf:"pre_commit"`
	// Agent lifecycle hooks — shell commands executed at various agent loop points.
	// Each command receives JSON on stdin.
	BeforeToolCall  []string `koanf:"before_tool_call"`
	AfterToolCall   []string `koanf:"after_tool_call"`
	BeforeModelCall []string `koanf:"before_model_call"`
	AfterModelCall  []string `koanf:"after_model_call"`
	OnAgentStart    []string `koanf:"on_agent_start"`
	OnAgentDone     []string `koanf:"on_agent_done"`
}

// PreCommitHookConfig configures the pre-commit hook.
type PreCommitHookConfig struct {
	// Enabled controls whether the hook runs at all (default: true).
	Enabled bool `koanf:"enabled"`
	// CheckPolvoYAML blocks commits with hardcoded api_key in polvo.yaml (default: true).
	CheckPolvoYAML bool `koanf:"check_polvo_yaml"`
	// SecretsScan runs regex + entropy scan on the full staged diff (default: true).
	SecretsScan bool `koanf:"secrets_scan"`
	// SecretsScanIgnore is a list of path glob patterns (fnmatch-style) that are
	// excluded from the secrets scan. Useful for test fixtures, generated files,
	// or any path that intentionally contains high-entropy non-secret data.
	// Example: ["testdata/**", "**/*_test.go", "**/*.lock"]
	SecretsScanIgnore []string `koanf:"secrets_scan_ignore"`
	// AIScan sends the staged diff to an LLM for secret/credential detection (default: false).
	// More thorough but slower — requires a configured provider.
	AIScan bool `koanf:"ai_scan"`
	// AIProvider is the provider name to use for AI scan (uses default if empty).
	AIProvider string `koanf:"ai_provider"`
}

// PermissionRule maps a tool to a permission level.
type PermissionRule struct {
	Tool  string `koanf:"tool"`
	Level string `koanf:"level" validate:"omitempty,oneof=allow ask deny"`
}

// APIKeyWarning is returned (alongside the stripped data) when api_key fields
// were found and removed from the project config.
type APIKeyWarning struct {
	Providers []string // provider names that had api_key stripped
}

func (w *APIKeyWarning) Error() string {
	return fmt.Sprintf("api_key found in project config for providers: %s — "+
		"credentials were ignored. Move api_key to ~/.polvo/config.yaml",
		strings.Join(w.Providers, ", "))
}

// stripAPIKeys parses a project config YAML and removes any api_key fields
// from provider entries. Returns the stripped YAML and a non-nil *APIKeyWarning
// if any keys were removed — credentials belong in ~/.polvo/config.yaml only.
func stripAPIKeys(data []byte) ([]byte, *APIKeyWarning, error) {
	var raw map[string]any
	if err := goyaml.Unmarshal(data, &raw); err != nil {
		return nil, nil, err
	}
	var warned []string
	if providers, ok := raw["providers"].(map[string]any); ok {
		for name, v := range providers {
			if p, ok := v.(map[string]any); ok {
				if val, exists := p["api_key"]; exists && val != "" {
					delete(p, "api_key")
					warned = append(warned, name)
				}
			}
		}
	}
	out, err := goyaml.Marshal(raw)
	if err != nil {
		return nil, nil, err
	}
	if len(warned) > 0 {
		return out, &APIKeyWarning{Providers: warned}, nil
	}
	return out, nil, nil
}

// UserConfigPath returns the path to the user-level config file: ~/.polvo/config.yaml.
// This file holds personal settings (providers, API keys) shared across all projects.
func UserConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".polvo", "config.yaml")
}

// Load reads the base config and merges with the project config file.
// If projectPath is empty, only the base config is loaded.
// Use LoadWithUser to also merge the user-level config (~/.polvo/config.yaml).
func Load(projectPath string) (*Config, error) {
	return load(projectPath, false)
}

// LoadWithUser reads configs in order: base (embedded) → user (~/.polvo/config.yaml) → project (polvo.yaml).
// User config provides providers and personal settings; project config provides project-specific overrides.
// Either file may be absent — only the base config is required.
func LoadWithUser(projectPath string) (*Config, error) {
	return load(projectPath, true)
}

func load(projectPath string, includeUser bool) (*Config, error) {
	k := koanf.New(".")

	// 1. Base config from embedded assets (always present).
	baseData, err := assets.ReadConfig()
	if err != nil {
		return nil, fmt.Errorf("reading base config: %w", err)
	}
	if err := k.Load(rawbytes.Provider(baseData), yaml.Parser()); err != nil {
		return nil, fmt.Errorf("parsing base config: %w", err)
	}

	// 2. User config: ~/.polvo/config.yaml (optional, holds providers/api keys).
	if includeUser {
		userPath := UserConfigPath()
		if userPath != "" {
			if userData, readErr := os.ReadFile(userPath); readErr == nil {
				expanded := os.ExpandEnv(string(userData))
				if parseErr := k.Load(rawbytes.Provider([]byte(expanded)), yaml.Parser()); parseErr != nil {
					return nil, fmt.Errorf("parsing user config %s: %w", userPath, parseErr)
				}
			}
			// Absent user config is not an error.
		}
	}

	// 3. Project config: polvo.yaml (optional, holds project-specific settings).
	// api_key is stripped from the project config — credentials belong only in
	// the user config (~/.polvo/config.yaml) and must never be committed to git.
	if projectPath != "" {
		projectData, err := os.ReadFile(projectPath)
		if err != nil {
			return nil, fmt.Errorf("reading project config %s: %w", projectPath, err)
		}
		expanded := os.ExpandEnv(string(projectData))
		stripped, warn, stripErr := stripAPIKeys([]byte(expanded))
		if stripErr != nil {
			return nil, fmt.Errorf("processing project config: %w", stripErr)
		}
		if warn != nil {
			// Surface the warning via the logger — config loading does not own stderr.
			fmt.Fprintf(os.Stderr, "warning: %s\n", warn)
		}
		if err := k.Load(rawbytes.Provider(stripped), yaml.Parser()); err != nil {
			return nil, fmt.Errorf("parsing project config: %w", err)
		}
	}

	cfg := DefaultConfig()
	if err := k.Unmarshal("", cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	if err := Validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks the config for required fields and valid values.
func Validate(cfg *Config) error {
	v := validator.New()
	if err := v.Struct(cfg); err != nil {
		return fmt.Errorf("config validation: %w", err)
	}
	return nil
}

// AllInterfacePatterns returns all patterns from all groups.
func (c *Config) AllInterfacePatterns() []string {
	var patterns []string
	for _, group := range c.Interfaces {
		patterns = append(patterns, group.Patterns...)
	}
	return patterns
}

// FindInterfaceGroup returns the group that matches the given file path.
func (c *Config) FindInterfaceGroup(path string) (string, *InterfaceGroupConfig) {
	for name, group := range c.Interfaces {
		for _, pattern := range group.Patterns {
			matched, _ := filepath.Match(pattern, path)
			if matched {
				return name, &group
			}
			matched, _ = filepath.Match(pattern, filepath.Base(path))
			if matched {
				return name, &group
			}
		}
	}
	return "", nil
}
