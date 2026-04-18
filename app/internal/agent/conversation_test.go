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

func TestTurnBoundaries(t *testing.T) {
	t.Run("TurnCount zero inicialmente", func(t *testing.T) {
		conv := NewConversation()
		if conv.TurnCount() != 0 {
			t.Errorf("expected 0 turns, got %d", conv.TurnCount())
		}
	})

	t.Run("TurnCount incrementa após user+assistant", func(t *testing.T) {
		conv := NewConversation()
		conv.AddUser("q1")
		conv.AddAssistant(provider.Message{Role: "assistant", Content: "a1"})
		if conv.TurnCount() != 1 {
			t.Errorf("expected 1 turn, got %d", conv.TurnCount())
		}

		conv.AddUser("q2")
		conv.AddAssistant(provider.Message{Role: "assistant", Content: "a2"})
		if conv.TurnCount() != 2 {
			t.Errorf("expected 2 turns, got %d", conv.TurnCount())
		}
	})

	t.Run("AddAssistant sem AddUser não abre turno", func(t *testing.T) {
		conv := NewConversation()
		conv.AddAssistant(provider.Message{Role: "assistant", Content: "orphan"})
		if conv.TurnCount() != 0 {
			t.Errorf("expected 0 turns, got %d", conv.TurnCount())
		}
	})

	t.Run("TurnUserContent e TurnAssistantContent corretos", func(t *testing.T) {
		conv := NewConversation()
		conv.AddUser("pergunta")
		conv.AddAssistant(provider.Message{Role: "assistant", Content: "resposta"})

		if got := conv.TurnUserContent(0); got != "pergunta" {
			t.Errorf("TurnUserContent(0) = %q; want %q", got, "pergunta")
		}
		if got := conv.TurnAssistantContent(0); got != "resposta" {
			t.Errorf("TurnAssistantContent(0) = %q; want %q", got, "resposta")
		}
	})

	t.Run("TurnUserContent/AssistantContent out-of-range retorna empty", func(t *testing.T) {
		conv := NewConversation()
		if got := conv.TurnUserContent(0); got != "" {
			t.Errorf("expected empty, got %q", got)
		}
		if got := conv.TurnAssistantContent(5); got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("tool messages incluídas no turno", func(t *testing.T) {
		conv := NewConversation()
		conv.AddUser("task")
		conv.AddToolResult("c1", "tool output", false)
		conv.AddAssistant(provider.Message{Role: "assistant", Content: "done"})

		// All 3 messages present
		msgs := conv.Messages()
		if len(msgs) != 3 {
			t.Fatalf("expected 3 messages, got %d", len(msgs))
		}
		if conv.TurnCount() != 1 {
			t.Errorf("expected 1 turn, got %d", conv.TurnCount())
		}
	})
}

func TestBuildMessagesSubstitution(t *testing.T) {
	t.Run("sem dismissed turns: mensagens inalteradas", func(t *testing.T) {
		conv := NewConversation()
		conv.AddUser("q1")
		conv.AddAssistant(provider.Message{Role: "assistant", Content: "a1"})
		conv.AddUser("q2")
		conv.AddAssistant(provider.Message{Role: "assistant", Content: "a2"})

		msgs := conv.Messages()
		if len(msgs) != 4 {
			t.Fatalf("expected 4 messages, got %d", len(msgs))
		}
	})

	t.Run("turno dismissed sem summary: não substitui", func(t *testing.T) {
		conv := NewConversation()
		conv.AddUser("q1")
		conv.AddAssistant(provider.Message{Role: "assistant", Content: "a1"})

		conv.SetMark(0, TurnMarkDismissed, "") // no summary → no substitution

		msgs := conv.Messages()
		if len(msgs) != 2 {
			t.Fatalf("expected 2 messages (no substitution), got %d", len(msgs))
		}
		if msgs[0].Content != "q1" {
			t.Errorf("expected original user content, got %q", msgs[0].Content)
		}
	})

	t.Run("turno dismissed com summary: substitui com placeholders", func(t *testing.T) {
		conv := NewConversation()
		conv.AddUser("long user question")
		conv.AddAssistant(provider.Message{Role: "assistant", Content: "long assistant response"})
		conv.AddUser("follow up")
		conv.AddAssistant(provider.Message{Role: "assistant", Content: "follow up response"})

		conv.SetMark(0, TurnMarkDismissed, "user asked something, got an answer")

		msgs := conv.Messages()
		// 2 placeholders for turn 0 + 2 original for turn 1
		if len(msgs) != 4 {
			t.Fatalf("expected 4 messages, got %d", len(msgs))
		}
		if msgs[0].Content != "[turn dismissed]" {
			t.Errorf("expected '[turn dismissed]', got %q", msgs[0].Content)
		}
		if msgs[1].Role != "assistant" {
			t.Errorf("expected assistant role for placeholder, got %q", msgs[1].Role)
		}
		if msgs[2].Content != "follow up" {
			t.Errorf("expected original turn 1 user msg, got %q", msgs[2].Content)
		}
	})

	t.Run("tool messages omitidas em turno dismissed", func(t *testing.T) {
		conv := NewConversation()
		conv.AddUser("task")
		conv.AddToolResult("c1", "tool output", false)
		conv.AddAssistant(provider.Message{Role: "assistant", Content: "done"})
		conv.AddUser("next")
		conv.AddAssistant(provider.Message{Role: "assistant", Content: "ok"})

		conv.SetMark(0, TurnMarkDismissed, "task completed with tool")

		msgs := conv.Messages()
		// Turn 0 had 3 messages → replaced by 2 placeholders
		// Turn 1 has 2 messages → kept
		if len(msgs) != 4 {
			t.Fatalf("expected 4 messages (2 placeholders + 2 real), got %d", len(msgs))
		}
		if msgs[0].Content != "[turn dismissed]" {
			t.Errorf("expected placeholder, got %q", msgs[0].Content)
		}
		if msgs[2].Content != "next" {
			t.Errorf("expected 'next', got %q", msgs[2].Content)
		}
	})
}

func TestConversationMarkRoundtrip(t *testing.T) {
	t.Run("SetMark e GetMark roundtrip", func(t *testing.T) {
		conv := NewConversation()
		conv.AddUser("q1")
		conv.AddAssistant(provider.Message{Role: "assistant", Content: "a1"})

		conv.SetMark(0, TurnMarkUseful, "very helpful")
		meta := conv.GetMark(0)
		if meta.Mark != TurnMarkUseful {
			t.Errorf("expected TurnMarkUseful, got %v", meta.Mark)
		}
		if meta.Summary != "very helpful" {
			t.Errorf("expected summary 'very helpful', got %q", meta.Summary)
		}
	})

	t.Run("GetMark out-of-range retorna TurnMarkNone", func(t *testing.T) {
		conv := NewConversation()
		meta := conv.GetMark(99)
		if meta.Mark != TurnMarkNone {
			t.Errorf("expected TurnMarkNone, got %v", meta.Mark)
		}
	})

	t.Run("SetMark out-of-range não panic", func(t *testing.T) {
		conv := NewConversation()
		// Should not panic
		conv.SetMark(99, TurnMarkDismissed, "summary")
	})
}
