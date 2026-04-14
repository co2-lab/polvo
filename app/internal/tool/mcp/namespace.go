package mcp

import "strings"

const mcpPrefix = "mcp__"
const namespaceSep = "__"

// NamespacedToolName converts a server name and tool name into a namespaced
// MCP tool name using the mcp__server__tool convention.
// Example: "filesystem" + "read_file" → "mcp__filesystem__read_file"
func NamespacedToolName(serverName, toolName string) string {
	return mcpPrefix + serverName + namespaceSep + toolName
}

// ParseNamespacedTool extracts the server name and tool name from a namespaced
// MCP tool name of the form "mcp__server__tool".
// Returns ("", "", false) if the name is not a valid namespaced MCP tool name.
func ParseNamespacedTool(name string) (server, toolName string, ok bool) {
	if !strings.HasPrefix(name, mcpPrefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(name, mcpPrefix)
	// rest must contain at least one "__" separator
	idx := strings.Index(rest, namespaceSep)
	if idx < 1 {
		return "", "", false
	}
	server = rest[:idx]
	toolName = rest[idx+len(namespaceSep):]
	if server == "" || toolName == "" {
		return "", "", false
	}
	return server, toolName, true
}
