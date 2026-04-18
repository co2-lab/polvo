package agent

import "github.com/co2-lab/polvo/internal/provider"

// turnBoundary records the message index span for one completed turn.
type turnBoundary struct {
	start int // index of the user message that opened this turn
	end   int // index of the assistant message that closed this turn (inclusive)
}

// Conversation manages a multi-turn message history with optional turn marks.
type Conversation struct {
	messages       []provider.Message
	turnBoundaries []turnBoundary
	openTurnStart  int // start index of the currently-open turn (-1 if none)
	marks          []TurnMetadata
}

// NewConversation creates an empty conversation.
func NewConversation() *Conversation {
	return &Conversation{openTurnStart: -1}
}

// AddUser appends a user message and opens a new turn boundary.
func (c *Conversation) AddUser(content string) {
	c.openTurnStart = len(c.messages)
	c.messages = append(c.messages, provider.Message{
		Role:    "user",
		Content: content,
	})
}

// AddAssistant appends an assistant message and closes the current open turn.
func (c *Conversation) AddAssistant(msg provider.Message) {
	idx := len(c.messages)
	c.messages = append(c.messages, msg)
	if c.openTurnStart >= 0 {
		c.turnBoundaries = append(c.turnBoundaries, turnBoundary{
			start: c.openTurnStart,
			end:   idx,
		})
		c.marks = append(c.marks, TurnMetadata{Index: len(c.turnBoundaries) - 1})
		c.openTurnStart = -1
	}
}

// AddToolResult appends a tool result message (within the current open turn).
func (c *Conversation) AddToolResult(toolCallID, content string, isError bool) {
	c.messages = append(c.messages, provider.Message{
		Role: "tool",
		ToolResult: &provider.ToolResult{
			ToolCallID: toolCallID,
			Content:    content,
			IsError:    isError,
		},
	})
}

// TurnCount returns the number of completed turns.
func (c *Conversation) TurnCount() int {
	return len(c.turnBoundaries)
}

// SetMark sets the mark and optional summary for the turn at idx.
func (c *Conversation) SetMark(idx int, mark TurnMark, summary string) {
	if idx < 0 || idx >= len(c.marks) {
		return
	}
	c.marks[idx].Mark = mark
	c.marks[idx].Summary = summary
}

// GetMark returns the TurnMetadata for turn idx.
func (c *Conversation) GetMark(idx int) TurnMetadata {
	if idx < 0 || idx >= len(c.marks) {
		return TurnMetadata{Index: idx}
	}
	return c.marks[idx]
}

// TurnUserContent returns the user message text for turn idx.
func (c *Conversation) TurnUserContent(idx int) string {
	if idx < 0 || idx >= len(c.turnBoundaries) {
		return ""
	}
	b := c.turnBoundaries[idx]
	if b.start < len(c.messages) {
		return c.messages[b.start].Content
	}
	return ""
}

// TurnAssistantContent returns the assistant message text for turn idx.
func (c *Conversation) TurnAssistantContent(idx int) string {
	if idx < 0 || idx >= len(c.turnBoundaries) {
		return ""
	}
	b := c.turnBoundaries[idx]
	if b.end < len(c.messages) {
		return c.messages[b.end].Content
	}
	return ""
}

// ReplaceMessages replaces the internal message slice with the given messages,
// clearing all turn boundaries and marks. Used by /compact to install a
// condensed history produced by a Condenser.
func (c *Conversation) ReplaceMessages(msgs []provider.Message) {
	c.messages = make([]provider.Message, len(msgs))
	copy(c.messages, msgs)
	c.turnBoundaries = nil
	c.marks = nil
	c.openTurnStart = -1
}

// Messages returns the message history, substituting dismissed turns with
// compact placeholder pairs to reduce context sent to the model.
func (c *Conversation) Messages() []provider.Message {
	// Fast path: no turn marks yet.
	hasDismissed := false
	for i := range c.marks {
		if c.marks[i].Mark == TurnMarkDismissed && c.marks[i].Summary != "" {
			hasDismissed = true
			break
		}
	}
	if !hasDismissed {
		msgs := make([]provider.Message, len(c.messages))
		copy(msgs, c.messages)
		return msgs
	}

	// Build a skip-set: message indices belonging to dismissed turns.
	// Also record the replacement pair to insert at the turn's start index.
	type replacement struct {
		atIdx     int
		user      provider.Message
		assistant provider.Message
	}
	skipSet := make(map[int]bool, len(c.messages))
	var replacements []replacement

	for i, b := range c.turnBoundaries {
		if i < len(c.marks) && c.marks[i].Mark == TurnMarkDismissed && c.marks[i].Summary != "" {
			for j := b.start; j <= b.end; j++ {
				skipSet[j] = true
			}
			replacements = append(replacements, replacement{
				atIdx:     b.start,
				user:      provider.Message{Role: "user", Content: "[turn dismissed]"},
				assistant: provider.Message{Role: "assistant", Content: "[summarized: " + c.marks[i].Summary + "]"},
			})
		}
	}

	// Build an insertion map: index → replacement to inject before that index.
	insertAt := make(map[int]replacement, len(replacements))
	for _, r := range replacements {
		insertAt[r.atIdx] = r
	}

	out := make([]provider.Message, 0, len(c.messages))
	for i, msg := range c.messages {
		if r, ok := insertAt[i]; ok {
			out = append(out, r.user, r.assistant)
		}
		if !skipSet[i] {
			out = append(out, msg)
		}
	}
	return out
}
