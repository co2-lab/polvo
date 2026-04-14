// Package mcp provides MCP (Model Context Protocol) server management.
package mcp

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"regexp"
)

// MCPServerConfig holds configuration for a single MCP server.
type MCPServerConfig struct {
	Name        string            `json:"name"`        // key from mcpServers map (populated on load)
	Command     string            `json:"command"`     // e.g. "npx"
	Args        []string          `json:"args"`
	Env         map[string]string `json:"env"`         // supports ${env:VAR} expansion
	Transport   string            `json:"transport"`   // "stdio" (default) | "streamable-http" | "sse"
	URL         string            `json:"url"`         // for HTTP transports
	Headers     map[string]string `json:"headers"`
	FilterTools string            `json:"filterTools"` // regex to filter tools
	Disabled    bool              `json:"disabled"`
}

// MCPPermissions holds allow/ask/deny rules for MCP tools.
type MCPPermissions struct {
	Allow []string `json:"allow"` // patterns like "mcp__server__tool*"
	Ask   []string `json:"ask"`
	Deny  []string `json:"deny"`
}

// MCPConfig is the top-level MCP configuration.
type MCPConfig struct {
	MCPServers  map[string]MCPServerConfig `json:"mcpServers"`
	Permissions MCPPermissions             `json:"permissions"`
}

// LoadMCPConfig loads MCP configuration from a JSON file at path.
// Returns an empty config (not an error) if the file does not exist.
func LoadMCPConfig(path string) (*MCPConfig, error) {
	cfg := &MCPConfig{
		MCPServers: make(map[string]MCPServerConfig),
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// Populate the Name field from the map key and expand env vars.
	expanded := make(map[string]MCPServerConfig, len(cfg.MCPServers))
	for name, srv := range cfg.MCPServers {
		srv.Name = name
		srv.Command = ExpandEnvVars(srv.Command)
		for i, arg := range srv.Args {
			srv.Args[i] = ExpandEnvVars(arg)
		}
		for k, v := range srv.Env {
			srv.Env[k] = ExpandEnvVars(v)
		}
		for k, v := range srv.Headers {
			srv.Headers[k] = ExpandEnvVars(v)
		}
		expanded[name] = srv
	}
	cfg.MCPServers = expanded

	return cfg, nil
}

// envVarPattern matches ${env:VAR_NAME} placeholders.
var envVarPattern = regexp.MustCompile(`\$\{env:([^}]+)\}`)

// ExpandEnvVars replaces ${env:VAR_NAME} placeholders with the actual
// environment variable value. If a variable is not set, it is replaced
// with an empty string and a warning is logged.
// The expanded value is never logged to prevent credential leakage.
func ExpandEnvVars(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		sub := envVarPattern.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		varName := sub[1]
		val, ok := os.LookupEnv(varName)
		if !ok {
			slog.Warn("mcp: environment variable not set", "var", varName)
			return ""
		}
		return val
	})
}
