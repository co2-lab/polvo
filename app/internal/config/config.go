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

	"github.com/co2-lab/polvo/internal/assets"
)

// Config is the top-level Polvo configuration.
type Config struct {
	Project     ProjectConfig                 `koanf:"project"`
	Providers   map[string]ProviderConfig     `koanf:"providers" validate:"required,min=1"`
	Interfaces  map[string]InterfaceGroupConfig `koanf:"interfaces"`
	Guides      map[string]GuideConfig        `koanf:"guides"`
	Chain       ChainConfig                   `koanf:"chain"`
	Review      ReviewConfig                  `koanf:"review"`
	Git         GitConfig                     `koanf:"git"`
	Settings    SettingsConfig                `koanf:"settings"`
	Permissions PermissionsConfig             `koanf:"permissions"`
}

// ProjectConfig holds project metadata.
type ProjectConfig struct {
	Name  string `koanf:"name"`
	Color string `koanf:"color"`
	Icon  string `koanf:"icon"`
}

// ProviderConfig defines an LLM provider.
type ProviderConfig struct {
	Type         string `koanf:"type" validate:"required,oneof=ollama claude openai gemini openai-compatible"`
	APIKey       string `koanf:"api_key"`
	BaseURL      string `koanf:"base_url"`
	DefaultModel string `koanf:"default_model"`
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

// GuideConfig configures a single guide.
type GuideConfig struct {
	Mode     string `koanf:"mode" validate:"omitempty,oneof=extend replace"`
	File     string `koanf:"file"`
	Provider string `koanf:"provider"`
	Model    string `koanf:"model"`
	Prompt   string `koanf:"prompt"`
	Role     string `koanf:"role" validate:"omitempty,oneof=author reviewer"`
	UseTools bool   `koanf:"use_tools"`
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
	BranchPrefix string   `koanf:"branch_prefix"`
	PRLabels     []string `koanf:"pr_labels"`
	TargetBranch string   `koanf:"target_branch"`
}

// SettingsConfig holds global settings.
type SettingsConfig struct {
	DebounceMs  int    `koanf:"debounce_ms"`
	ReportDir   string `koanf:"report_dir"`
	LogLevel    string `koanf:"log_level" validate:"omitempty,oneof=debug info warn error"`
	MaxParallel int    `koanf:"max_parallel"`
}

// PermissionsConfig controls tool execution permissions.
type PermissionsConfig struct {
	Rules []PermissionRule `koanf:"rules"`
}

// PermissionRule maps a tool to a permission level.
type PermissionRule struct {
	Tool  string `koanf:"tool"`
	Level string `koanf:"level" validate:"omitempty,oneof=allow ask deny"`
}

// Load reads the base config and merges with the project config file.
// If projectPath is empty, only the base config is loaded.
func Load(projectPath string) (*Config, error) {
	k := koanf.New(".")

	// Load base config from embedded assets
	baseData, err := assets.ReadConfig()
	if err != nil {
		return nil, fmt.Errorf("reading base config: %w", err)
	}
	if err := k.Load(rawbytes.Provider(baseData), yaml.Parser()); err != nil {
		return nil, fmt.Errorf("parsing base config: %w", err)
	}

	// Load project config if provided
	if projectPath != "" {
		projectData, err := os.ReadFile(projectPath)
		if err != nil {
			return nil, fmt.Errorf("reading project config %s: %w", projectPath, err)
		}
		// Expand environment variables
		expanded := os.ExpandEnv(string(projectData))
		if err := k.Load(rawbytes.Provider([]byte(expanded)), yaml.Parser()); err != nil {
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
