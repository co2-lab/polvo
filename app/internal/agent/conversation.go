package agent

import "github.com/co2-lab/polvo/internal/provider"

// Conversation manages a multi-turn message history.
type Conversation struct {
	messages []provider.Message
}

// NewConversation creates an empty conversation.
func NewConversation() *Conversation {
	return &Conversation{}
}

// AddUser appends a user message.
func (c *Conversation) AddUser(content string) {
	c.messages = append(c.messages, provider.Message{
		Role:    "user",
		Content: content,
	})
}

// AddAssistant appends an assistant message (preserving tool calls).
func (c *Conversation) AddAssistant(msg provider.Message) {
	c.messages = append(c.messages, msg)
}

// AddToolResult appends a tool result message.
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

// Messages returns a copy of the message history.
func (c *Conversation) Messages() []provider.Message {
	msgs := make([]provider.Message, len(c.messages))
	copy(msgs, c.messages)
	return msgs
}
