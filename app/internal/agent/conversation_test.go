package agent

import (
	"testing"

	"github.com/co2-lab/polvo/internal/provider"
)

func TestConversation(t *testing.T) {
	t.Run("NewConversation vazia", func(t *testing.T) {
		conv := NewConversation()
		if msgs := conv.Messages(); len(msgs) != 0 {
			t.Errorf("expected empty messages, got %d", len(msgs))
		}
	})

	t.Run("AddUser adiciona mensagem com Role=user", func(t *testing.T) {
		conv := NewConversation()
		conv.AddUser("hello")
		msgs := conv.Messages()
		if len(msgs) != 1 {
			t.Fatalf("expected 1 message, got %d", len(msgs))
		}
		if msgs[0].Role != "user" {
			t.Errorf("expected Role='user', got %q", msgs[0].Role)
		}
		if msgs[0].Content != "hello" {
			t.Errorf("expected Content='hello', got %q", msgs[0].Content)
		}
	})

	t.Run("AddAssistant adiciona mensagem com Role=assistant", func(t *testing.T) {
		conv := NewConversation()
		msg := provider.Message{Role: "assistant", Content: "I can help"}
		conv.AddAssistant(msg)
		msgs := conv.Messages()
		if len(msgs) != 1 {
			t.Fatalf("expected 1 message, got %d", len(msgs))
		}
		if msgs[0].Role != "assistant" {
			t.Errorf("expected Role='assistant', got %q", msgs[0].Role)
		}
	})

	t.Run("AddToolResult adiciona mensagem com Role=tool", func(t *testing.T) {
		conv := NewConversation()
		conv.AddToolResult("call-1", "file contents here", false)
		msgs := conv.Messages()
		if len(msgs) != 1 {
			t.Fatalf("expected 1 message, got %d", len(msgs))
		}
		if msgs[0].Role != "tool" {
			t.Errorf("expected Role='tool', got %q", msgs[0].Role)
		}
		if msgs[0].ToolResult == nil {
			t.Fatal("expected ToolResult to be set")
		}
		if msgs[0].ToolResult.ToolCallID != "call-1" {
			t.Errorf("expected ToolCallID='call-1', got %q", msgs[0].ToolResult.ToolCallID)
		}
		if msgs[0].ToolResult.Content != "file contents here" {
			t.Errorf("expected Content='file contents here', got %q", msgs[0].ToolResult.Content)
		}
		if msgs[0].ToolResult.IsError != false {
			t.Error("expected IsError=false")
		}
	})

	t.Run("sequência user→assistant→tool→user em ordem", func(t *testing.T) {
		conv := NewConversation()
		conv.AddUser("task")
		conv.AddAssistant(provider.Message{Role: "assistant", Content: "thinking..."})
		conv.AddToolResult("call-1", "result", false)
		conv.AddUser("continue")

		msgs := conv.Messages()
		if len(msgs) != 4 {
			t.Fatalf("expected 4 messages, got %d", len(msgs))
		}
		roles := []string{"user", "assistant", "tool", "user"}
		for i, expected := range roles {
			if msgs[i].Role != expected {
				t.Errorf("msgs[%d].Role = %q; want %q", i, msgs[i].Role, expected)
			}
		}
	})

	t.Run("Messages retorna cópia (não referência)", func(t *testing.T) {
		conv := NewConversation()
		conv.AddUser("original")
		msgs1 := conv.Messages()
		msgs1[0].Content = "modified"
		msgs2 := conv.Messages()
		if msgs2[0].Content != "original" {
			t.Error("Messages() should return a copy, not a reference to internal state")
		}
	})
}
