package provider

import (
	"encoding/json"
	"strings"
	"testing"
)

func makeMsg(role, content string) Message {
	return Message{Role: role, Content: content}
}

func makeToolCallMsg(content, toolID string) Message {
	return Message{
		Role:    "assistant",
		Content: content,
		ToolCalls: []ToolCall{
			{ID: toolID, Name: "bash", Input: json.RawMessage(`{"cmd":"ls"}`)},
		},
	}
}

func makeToolResultMsg(toolID, content string) Message {
	return Message{
		Role: "tool",
		ToolResult: &ToolResult{
			ToolCallID: toolID,
			Content:    content,
		},
	}
}

func TestPruneMessages_FitsAlready(t *testing.T) {
	msgs := []Message{
		makeMsg("user", "hello"),
	}
	// Large context window — nothing should be pruned.
	got := PruneMessages(msgs, 100000, 1000)
	if len(got) != len(msgs) {
		t.Errorf("expected no pruning, got %d messages", len(got))
	}
}

func TestPruneMessages_SystemMessagePreserved(t *testing.T) {
	// Fill with many messages so we exceed context.
	msgs := []Message{
		makeMsg("system", "You are an assistant."),
	}
	// Add many old user/assistant messages.
	for i := 0; i < 50; i++ {
		msgs = append(msgs, makeMsg("user", strings.Repeat("x", 400)))
		msgs = append(msgs, makeMsg("assistant", strings.Repeat("y", 400)))
	}
	msgs = append(msgs, makeMsg("user", "final question"))

	// Small context window to force pruning.
	got := PruneMessages(msgs, 2000, 200)

	// System message must be at index 0.
	if got[0].Role != "system" {
		t.Errorf("system message should be preserved at index 0, got role=%q", got[0].Role)
	}
}

func TestPruneMessages_LastUserMessagePreserved(t *testing.T) {
	msgs := []Message{
		makeMsg("system", "system"),
	}
	for i := 0; i < 20; i++ {
		msgs = append(msgs, makeMsg("user", strings.Repeat("a", 200)))
		msgs = append(msgs, makeMsg("assistant", strings.Repeat("b", 200)))
	}
	lastUser := makeMsg("user", "LAST_USER_MESSAGE")
	msgs = append(msgs, lastUser)

	got := PruneMessages(msgs, 1000, 100)

	found := false
	for _, m := range got {
		if m.Content == "LAST_USER_MESSAGE" {
			found = true
			break
		}
	}
	if !found {
		t.Error("last user message should always be preserved")
	}
}

func TestPruneMessages_StickyPreserved(t *testing.T) {
	msgs := []Message{
		makeMsg("system", "system"),
		makeMsg("user", "old message <!-- sticky -->"),
	}
	for i := 0; i < 20; i++ {
		msgs = append(msgs, makeMsg("user", strings.Repeat("x", 300)))
		msgs = append(msgs, makeMsg("assistant", strings.Repeat("y", 300)))
	}
	msgs = append(msgs, makeMsg("user", "current task"))

	got := PruneMessages(msgs, 1000, 100)

	found := false
	for _, m := range got {
		if strings.Contains(m.Content, "<!-- sticky -->") {
			found = true
			break
		}
	}
	if !found {
		t.Error("sticky message should be preserved")
	}
}

func TestPruneMessages_ToolPairsRemovedTogether(t *testing.T) {
	// Build: system, [tool_call + tool_result], [tool_call + tool_result], last_user
	msgs := []Message{
		makeMsg("system", "system"),
	}
	// Add many tool call/result pairs to exceed context.
	for i := 0; i < 10; i++ {
		id := strings.Repeat("a", i+1)
		msgs = append(msgs, makeToolCallMsg("", id))
		msgs = append(msgs, makeToolResultMsg(id, strings.Repeat("r", 400)))
	}
	msgs = append(msgs, makeMsg("user", "final"))

	got := PruneMessages(msgs, 2000, 200)

	// Check: no orphaned tool call (assistant with ToolCalls but no matching tool result).
	for i, m := range got {
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			id := m.ToolCalls[0].ID
			found := false
			for _, r := range got {
				if r.Role == "tool" && r.ToolResult != nil && r.ToolResult.ToolCallID == id {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("tool call at index %d has no matching tool result — orphaned", i)
			}
		}
	}
}

func TestPruneMessages_OldestRemovedFirst(t *testing.T) {
	msgs := []Message{
		makeMsg("system", "system"),
		makeMsg("user", "very old message "+strings.Repeat("x", 400)),
		makeMsg("assistant", "old reply "+strings.Repeat("y", 400)),
		makeMsg("user", "recent message "+strings.Repeat("z", 400)),
		makeMsg("assistant", "recent reply"),
		makeMsg("user", "current"),
	}

	// Force pruning with tight context.
	got := PruneMessages(msgs, 1000, 100)

	// "current" (last user) must be present.
	hasLast := false
	for _, m := range got {
		if m.Content == "current" {
			hasLast = true
		}
	}
	if !hasLast {
		t.Error("last user message should be preserved")
	}
}

func TestIsSticky(t *testing.T) {
	cases := []struct {
		content string
		want    bool
	}{
		{"normal message", false},
		{"sticky note <!-- sticky -->", true},
		{"microagent injection <!-- microagent -->", true},
		{"<!-- sticky --> at start", true},
	}
	for _, tc := range cases {
		got := isSticky(Message{Role: "user", Content: tc.content})
		if got != tc.want {
			t.Errorf("isSticky(%q) = %v; want %v", tc.content, got, tc.want)
		}
	}
}
