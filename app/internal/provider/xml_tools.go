package provider

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// xmlToolCallRe matches a complete <tool_call>…</tool_call> block produced by
// a model that uses XML tool calling (i.e. no native function-call API support).
var xmlToolCallRe = regexp.MustCompile(`(?s)<tool_call>\s*<name>(.*?)</name>\s*<parameters>(.*?)</parameters>\s*</tool_call>`)

// XMLToolPrompt generates the system-prompt section that teaches a model to
// use tools via XML when the provider has no native function-calling API.
func XMLToolPrompt(tools []ToolDef) string {
	if len(tools) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n\n# Tool Use Instructions\n\n")
	sb.WriteString("You have access to tools. To call a tool, respond ONLY with the following XML block (nothing else on those lines):\n\n")
	sb.WriteString("<tool_call>\n<name>TOOL_NAME</name>\n<parameters>\n{\"param\": \"value\"}\n</parameters>\n</tool_call>\n\n")
	sb.WriteString("Rules:\n")
	sb.WriteString("- Use one tool call per response.\n")
	sb.WriteString("- Wait for the tool result before calling another tool.\n")
	sb.WriteString("- When you have enough information, respond normally without any XML.\n\n")
	sb.WriteString("## Available Tools\n\n")
	for _, t := range tools {
		fmt.Fprintf(&sb, "### %s\n%s\n\nInput schema:\n```json\n%s\n```\n\n", t.Name, t.Description, string(t.InputSchema))
	}
	return sb.String()
}

// ParseXMLToolCall extracts a tool call from model-generated text.
// Returns nil if no valid tool call is found.
func ParseXMLToolCall(text string) *ToolCall {
	m := xmlToolCallRe.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	name := strings.TrimSpace(m[1])
	params := strings.TrimSpace(m[2])
	if name == "" {
		return nil
	}
	// Ensure params is valid JSON; fall back to empty object.
	if !json.Valid([]byte(params)) {
		params = "{}"
	}
	return &ToolCall{
		ID:    fmt.Sprintf("xml-%d", time.Now().UnixNano()),
		Name:  name,
		Input: json.RawMessage(params),
	}
}
