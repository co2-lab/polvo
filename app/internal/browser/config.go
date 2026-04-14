// Package browser provides browser automation via @playwright/mcp (stdio JSON-RPC).
package browser

// Config holds browser tool configuration.
type Config struct {
	Enabled        bool           `yaml:"enabled"`          // opt-in; requires Node.js + @playwright/mcp
	Mode           string         `yaml:"mode"`             // "snapshot" (default) | "vision"
	Context        string         `yaml:"context"`          // "isolated" (default) | "persistent"
	StorageState   string         `yaml:"storage_state"`    // path to auth JSON (optional)
	TimeoutMS      int            `yaml:"timeout_ms"`       // per-action timeout, default 30000
	AllowedDomains []string       `yaml:"allowed_domains"`  // extra allowed domains beyond conversation URLs
	Security       SecurityConfig `yaml:"security"`
}

// SecurityConfig controls security features of the browser tool.
type SecurityConfig struct {
	URLAllowlist       bool `yaml:"url_allowlist"`        // only navigate to URLs seen in conversation (default true)
	InjectionDetection bool `yaml:"injection_detection"`  // detect prompt injection in page content (default true)
	BlockFileUpload    bool `yaml:"block_file_upload"`    // block browser_file_upload (default true)
	BlockJSEval        bool `yaml:"block_js_eval"`        // block browser_evaluate (default true)
}

// DefaultConfig returns sensible defaults for the browser tool.
func DefaultConfig() Config {
	return Config{
		Enabled:   false,
		Mode:      "snapshot",
		Context:   "isolated",
		TimeoutMS: 30000,
		Security: SecurityConfig{
			URLAllowlist:       true,
			InjectionDetection: true,
			BlockFileUpload:    true,
			BlockJSEval:        true,
		},
	}
}
