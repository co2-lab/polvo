package agent

// Testes para as regras de turn marks tratadas nas sessões de desenvolvimento:
//
//  1. Marks persistem entre runs (turnMarks é TUI-global, conv é per-run)
//  2. Dismiss sem summary não substitui mensagens no contexto
//  3. Dismiss com summary substitui e comprime o contexto
//  4. Toggle de dismiss (d+d) remove o mark
//  5. Toggle de useful (u+u) remove o mark
//  6. Useful não colapsa o turn no contexto
//  7. Tool messages de um turn dismissed são omitidas junto com o turn
//  8. Múltiplos turns dismissed independentes
//  9. Dismiss seguido de summary atualiza a substituição no contexto
// 10. ExtractInlineSummary: remove tag do conteúdo exibido
// 11. summaryDeltaProxy: suprime <summary>…</summary> no streaming
// 12. summaryDeltaProxy: preserva whitespace que não precede <summary>
// 13. summaryDeltaProxy: não bloqueia tag falsa (ex: <summaryX>)
// 14. summaryDeltaProxy: Flush emite conteúdo pendente não-tag
// 15. Compressão de contexto com offset de run (turns de runs anteriores)

import (
	"strings"
	"testing"

	"github.com/co2-lab/polvo/internal/provider"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func buildConv(turns []struct{ user, assistant string }) *Conversation {
	conv := NewConversation()
	for _, t := range turns {
		conv.AddUser(t.user)
		conv.AddAssistant(provider.Message{Role: "assistant", Content: t.assistant})
	}
	return conv
}

func buildConvWithTools(user, toolResult, assistant string) *Conversation {
	conv := NewConversation()
	conv.AddUser(user)
	conv.AddToolResult("c1", toolResult, false)
	conv.AddAssistant(provider.Message{Role: "assistant", Content: assistant})
	return conv
}

// ── 1. Marks persistem: SetMark/GetMark roundtrip ────────────────────────────

func TestTurnMarkPersistence(t *testing.T) {
	t.Run("SetMark dismissed persiste no GetMark", func(t *testing.T) {
		conv := buildConv([]struct{ user, assistant string }{{"q", "a"}})
		conv.SetMark(0, TurnMarkDismissed, "resumo aqui")
		meta := conv.GetMark(0)
		if meta.Mark != TurnMarkDismissed {
			t.Errorf("esperado TurnMarkDismissed, obtido %v", meta.Mark)
		}
		if meta.Summary != "resumo aqui" {
			t.Errorf("esperado summary 'resumo aqui', obtido %q", meta.Summary)
		}
	})

	t.Run("SetMark useful persiste no GetMark", func(t *testing.T) {
		conv := buildConv([]struct{ user, assistant string }{{"q", "a"}})
		conv.SetMark(0, TurnMarkUseful, "")
		meta := conv.GetMark(0)
		if meta.Mark != TurnMarkUseful {
			t.Errorf("esperado TurnMarkUseful, obtido %v", meta.Mark)
		}
	})

	t.Run("GetMark índice inexistente retorna TurnMarkNone", func(t *testing.T) {
		conv := NewConversation()
		meta := conv.GetMark(0)
		if meta.Mark != TurnMarkNone {
			t.Errorf("esperado TurnMarkNone para conv vazia, obtido %v", meta.Mark)
		}
		meta = conv.GetMark(99)
		if meta.Mark != TurnMarkNone {
			t.Errorf("esperado TurnMarkNone para índice 99, obtido %v", meta.Mark)
		}
	})

	t.Run("SetMark índice fora do range não panics", func(t *testing.T) {
		conv := NewConversation()
		conv.SetMark(99, TurnMarkDismissed, "não deve panic")
		// nenhum turn: deve ser ignorado silenciosamente
		if conv.TurnCount() != 0 {
			t.Error("TurnCount não deve mudar")
		}
	})
}

// ── 2. Dismiss sem summary: contexto inalterado ───────────────────────────────

func TestDismissWithoutSummaryNoSubstitution(t *testing.T) {
	conv := buildConv([]struct{ user, assistant string }{
		{"pergunta longa", "resposta longa"},
		{"segunda pergunta", "segunda resposta"},
	})
	conv.SetMark(0, TurnMarkDismissed, "") // sem summary → não comprime

	msgs := conv.Messages()
	if len(msgs) != 4 {
		t.Fatalf("esperado 4 mensagens (sem substituição), obtido %d", len(msgs))
	}
	if msgs[0].Content != "pergunta longa" {
		t.Errorf("conteúdo original deve ser preservado, obtido %q", msgs[0].Content)
	}
}

// ── 3. Dismiss com summary: comprime para 2 placeholders ─────────────────────

func TestDismissWithSummarySubstitution(t *testing.T) {
	t.Run("turn 0 dismissed comprime para placeholders", func(t *testing.T) {
		conv := buildConv([]struct{ user, assistant string }{
			{"pergunta longa", "resposta longa"},
			{"seguimento", "ok"},
		})
		conv.SetMark(0, TurnMarkDismissed, "usuário perguntou, obteve resposta")

		msgs := conv.Messages()
		// 2 placeholders + 2 msgs originais do turn 1
		if len(msgs) != 4 {
			t.Fatalf("esperado 4 mensagens, obtido %d", len(msgs))
		}
		if msgs[0].Role != "user" || msgs[0].Content != "[turn dismissed]" {
			t.Errorf("msgs[0] deve ser placeholder user, obtido role=%q content=%q", msgs[0].Role, msgs[0].Content)
		}
		if msgs[1].Role != "assistant" || !strings.Contains(msgs[1].Content, "usuário perguntou") {
			t.Errorf("msgs[1] deve ser placeholder assistant com summary, obtido %q", msgs[1].Content)
		}
		if msgs[2].Content != "seguimento" {
			t.Errorf("msgs[2] deve ser turn 1 original, obtido %q", msgs[2].Content)
		}
		if msgs[3].Content != "ok" {
			t.Errorf("msgs[3] deve ser turn 1 original, obtido %q", msgs[3].Content)
		}
	})

	t.Run("placeholder assistant contém o summary no conteúdo", func(t *testing.T) {
		conv := buildConv([]struct{ user, assistant string }{{"q", "a"}, {"q2", "a2"}})
		summary := "resumo compacto do turn"
		conv.SetMark(0, TurnMarkDismissed, summary)

		msgs := conv.Messages()
		if !strings.Contains(msgs[1].Content, summary) {
			t.Errorf("placeholder deve conter o summary, conteúdo: %q", msgs[1].Content)
		}
	})

	t.Run("turn do meio dismissed, outros intactos", func(t *testing.T) {
		conv := buildConv([]struct{ user, assistant string }{
			{"q0", "a0"},
			{"q1", "a1"},
			{"q2", "a2"},
		})
		conv.SetMark(1, TurnMarkDismissed, "turn 1 resumido")

		msgs := conv.Messages()
		// turn 0: 2 msgs + turn 1: 2 placeholders + turn 2: 2 msgs = 6
		if len(msgs) != 6 {
			t.Fatalf("esperado 6 mensagens, obtido %d", len(msgs))
		}
		if msgs[0].Content != "q0" {
			t.Errorf("turn 0 deve estar intacto: %q", msgs[0].Content)
		}
		if msgs[2].Content != "[turn dismissed]" {
			t.Errorf("turn 1 deve ser placeholder: %q", msgs[2].Content)
		}
		if msgs[4].Content != "q2" {
			t.Errorf("turn 2 deve estar intacto: %q", msgs[4].Content)
		}
	})
}

// ── 4. Toggle dismiss (d+d remove o mark) ────────────────────────────────────

func TestDismissToggle(t *testing.T) {
	t.Run("SetMark None após Dismissed restaura conteúdo original", func(t *testing.T) {
		conv := buildConv([]struct{ user, assistant string }{
			{"q", "a"},
			{"q2", "a2"},
		})
		conv.SetMark(0, TurnMarkDismissed, "resumo")

		// confirma que está dismissed
		msgs := conv.Messages()
		if msgs[0].Content != "[turn dismissed]" {
			t.Fatal("esperado placeholder após dismiss")
		}

		// restaura
		conv.SetMark(0, TurnMarkNone, "")
		msgs = conv.Messages()
		if len(msgs) != 4 {
			t.Fatalf("após restaurar esperado 4 msgs, obtido %d", len(msgs))
		}
		if msgs[0].Content != "q" {
			t.Errorf("conteúdo original deve voltar, obtido %q", msgs[0].Content)
		}
	})
}

// ── 5. Toggle useful (u+u remove o mark) ─────────────────────────────────────

func TestUsefulToggle(t *testing.T) {
	t.Run("SetMark None após Useful limpa o mark", func(t *testing.T) {
		conv := buildConv([]struct{ user, assistant string }{{"q", "a"}})
		conv.SetMark(0, TurnMarkUseful, "")
		if conv.GetMark(0).Mark != TurnMarkUseful {
			t.Fatal("esperado TurnMarkUseful após set")
		}
		conv.SetMark(0, TurnMarkNone, "")
		if conv.GetMark(0).Mark != TurnMarkNone {
			t.Errorf("esperado TurnMarkNone após toggle, obtido %v", conv.GetMark(0).Mark)
		}
	})
}

// ── 6. Useful não colapsa no contexto ────────────────────────────────────────

func TestUsefulDoesNotCollapseContext(t *testing.T) {
	conv := buildConv([]struct{ user, assistant string }{
		{"q importante", "a importante"},
		{"q2", "a2"},
	})
	conv.SetMark(0, TurnMarkUseful, "")

	msgs := conv.Messages()
	// useful não substitui: 4 mensagens originais
	if len(msgs) != 4 {
		t.Fatalf("useful não deve comprimir contexto, esperado 4 msgs, obtido %d", len(msgs))
	}
	if msgs[0].Content != "q importante" {
		t.Errorf("conteúdo deve estar intacto, obtido %q", msgs[0].Content)
	}
}

// ── 7. Tool messages omitidas com o turn dismissed ────────────────────────────

func TestToolMessagesOmittedWithDismissedTurn(t *testing.T) {
	t.Run("tool messages do turn dismissed são removidas", func(t *testing.T) {
		conv := buildConvWithTools("task", "saída da ferramenta", "feito")
		conv.AddUser("próxima pergunta")
		conv.AddAssistant(provider.Message{Role: "assistant", Content: "ok"})

		conv.SetMark(0, TurnMarkDismissed, "task completada com ferramenta")

		msgs := conv.Messages()
		// turn 0 tinha 3 msgs (user+tool+assistant) → 2 placeholders
		// turn 1 tem 2 msgs → intactas
		if len(msgs) != 4 {
			t.Fatalf("esperado 4 msgs (2 placeholders + 2 reais), obtido %d", len(msgs))
		}
		for _, m := range msgs {
			if m.Role == "tool" {
				t.Error("tool message não deve aparecer em turn dismissed")
			}
		}
		if msgs[0].Content != "[turn dismissed]" {
			t.Errorf("esperado placeholder, obtido %q", msgs[0].Content)
		}
	})

	t.Run("múltiplas tool messages no turn dismissed todas omitidas", func(t *testing.T) {
		conv := NewConversation()
		conv.AddUser("task complexa")
		conv.AddToolResult("c1", "resultado 1", false)
		conv.AddToolResult("c2", "resultado 2", false)
		conv.AddToolResult("c3", "resultado 3", false)
		conv.AddAssistant(provider.Message{Role: "assistant", Content: "concluído"})

		conv.SetMark(0, TurnMarkDismissed, "task complexa com 3 ferramentas")

		msgs := conv.Messages()
		if len(msgs) != 2 {
			t.Fatalf("esperado 2 placeholders, obtido %d", len(msgs))
		}
		for _, m := range msgs {
			if m.Role == "tool" {
				t.Error("nenhuma tool message deve aparecer")
			}
		}
	})
}

// ── 8. Múltiplos turns dismissed independentes ────────────────────────────────

func TestMultipleDismissedTurns(t *testing.T) {
	t.Run("turns 0 e 2 dismissed, turn 1 intacto", func(t *testing.T) {
		conv := buildConv([]struct{ user, assistant string }{
			{"q0", "a0"},
			{"q1", "a1"},
			{"q2", "a2"},
		})
		conv.SetMark(0, TurnMarkDismissed, "resumo turn 0")
		conv.SetMark(2, TurnMarkDismissed, "resumo turn 2")

		msgs := conv.Messages()
		// turn 0: 2 placeholders + turn 1: 2 reais + turn 2: 2 placeholders = 6
		if len(msgs) != 6 {
			t.Fatalf("esperado 6 msgs, obtido %d", len(msgs))
		}
		if msgs[0].Content != "[turn dismissed]" {
			t.Errorf("turn 0 deve ser placeholder, obtido %q", msgs[0].Content)
		}
		if msgs[2].Content != "q1" {
			t.Errorf("turn 1 deve estar intacto, obtido %q", msgs[2].Content)
		}
		if msgs[4].Content != "[turn dismissed]" {
			t.Errorf("turn 2 deve ser placeholder, obtido %q", msgs[4].Content)
		}
	})

	t.Run("todos os turns dismissed", func(t *testing.T) {
		conv := buildConv([]struct{ user, assistant string }{
			{"q0", "a0"},
			{"q1", "a1"},
		})
		conv.SetMark(0, TurnMarkDismissed, "resumo 0")
		conv.SetMark(1, TurnMarkDismissed, "resumo 1")

		msgs := conv.Messages()
		if len(msgs) != 4 {
			t.Fatalf("esperado 4 placeholders (2 por turn), obtido %d", len(msgs))
		}
		for i, m := range msgs {
			if i%2 == 0 && m.Content != "[turn dismissed]" {
				t.Errorf("msgs[%d] deve ser placeholder user, obtido %q", i, m.Content)
			}
			if i%2 == 1 && m.Role != "assistant" {
				t.Errorf("msgs[%d] deve ser placeholder assistant, obtido role=%q", i, m.Role)
			}
		}
	})
}

// ── 9. Summary atualiza substituição no contexto ─────────────────────────────

func TestDismissSummaryUpdate(t *testing.T) {
	t.Run("SetMark com novo summary atualiza placeholder", func(t *testing.T) {
		conv := buildConv([]struct{ user, assistant string }{
			{"q", "a"},
			{"q2", "a2"},
		})
		conv.SetMark(0, TurnMarkDismissed, "primeiro resumo")
		msgs1 := conv.Messages()
		if !strings.Contains(msgs1[1].Content, "primeiro resumo") {
			t.Fatalf("esperado primeiro resumo, obtido %q", msgs1[1].Content)
		}

		conv.SetMark(0, TurnMarkDismissed, "resumo atualizado")
		msgs2 := conv.Messages()
		if !strings.Contains(msgs2[1].Content, "resumo atualizado") {
			t.Errorf("esperado resumo atualizado, obtido %q", msgs2[1].Content)
		}
		if strings.Contains(msgs2[1].Content, "primeiro resumo") {
			t.Error("resumo antigo não deve aparecer após atualização")
		}
	})
}

// ── 10. ExtractInlineSummary ──────────────────────────────────────────────────

func TestExtractInlineSummary(t *testing.T) {
	t.Run("extrai summary e limpa conteúdo", func(t *testing.T) {
		content := "Aqui está a resposta completa.\n\n<summary>Resposta curta.</summary>"
		summary, cleaned := ExtractInlineSummary(content)
		if summary != "Resposta curta." {
			t.Errorf("summary errado: %q", summary)
		}
		if strings.Contains(cleaned, "<summary>") {
			t.Errorf("conteúdo limpo não deve conter <summary>, obtido: %q", cleaned)
		}
		if !strings.Contains(cleaned, "Aqui está a resposta") {
			t.Errorf("conteúdo principal deve estar preservado, obtido: %q", cleaned)
		}
	})

	t.Run("sem tag: summary vazio e conteúdo inalterado", func(t *testing.T) {
		content := "Resposta sem summary."
		summary, cleaned := ExtractInlineSummary(content)
		if summary != "" {
			t.Errorf("esperado summary vazio, obtido %q", summary)
		}
		if cleaned != content {
			t.Errorf("conteúdo deve ser inalterado, obtido %q", cleaned)
		}
	})

	t.Run("summary com whitespace no início/fim é trimado", func(t *testing.T) {
		content := "Resposta.\n<summary>  resumo com espaços  </summary>"
		summary, _ := ExtractInlineSummary(content)
		if summary != "resumo com espaços" {
			t.Errorf("summary deve ser trimado, obtido %q", summary)
		}
	})

	t.Run("conteúdo após tag é preservado", func(t *testing.T) {
		content := "Resposta.\n<summary>resumo</summary>\n\nalgo depois"
		_, cleaned := ExtractInlineSummary(content)
		if !strings.Contains(cleaned, "algo depois") {
			t.Errorf("conteúdo após tag deve ser mantido, obtido %q", cleaned)
		}
	})

	t.Run("summary multiline extraído corretamente", func(t *testing.T) {
		content := "Resposta.\n<summary>linha um\nlinha dois</summary>"
		summary, cleaned := ExtractInlineSummary(content)
		if !strings.Contains(summary, "linha um") || !strings.Contains(summary, "linha dois") {
			t.Errorf("summary multiline não extraído: %q", summary)
		}
		if strings.Contains(cleaned, "<summary>") {
			t.Error("tag não deve estar no conteúdo limpo")
		}
	})
}

// ── 11. summaryDeltaProxy: suprime <summary>…</summary> no streaming ─────────

func TestSummaryDeltaProxySuppression(t *testing.T) {
	collect := func(inputs ...string) string {
		var out strings.Builder
		p := newSummaryDeltaProxy(func(s string) { out.WriteString(s) })
		for _, s := range inputs {
			p.Write(s)
		}
		p.Flush()
		return out.String()
	}

	t.Run("tag completa em um único delta é suprimida", func(t *testing.T) {
		result := collect("Resposta.\n<summary>conteúdo do summary</summary>")
		if strings.Contains(result, "<summary>") {
			t.Errorf("tag não deve aparecer no output: %q", result)
		}
		if strings.Contains(result, "conteúdo do summary") {
			t.Errorf("conteúdo do summary não deve aparecer: %q", result)
		}
		if !strings.Contains(result, "Resposta.") {
			t.Errorf("conteúdo principal deve ser preservado: %q", result)
		}
	})

	t.Run("whitespace antes de <summary> é suprimido", func(t *testing.T) {
		result := collect("texto", "\n\n", "<summary>resumo</summary>")
		if strings.HasSuffix(result, "\n\n") {
			t.Errorf("whitespace antes do summary não deve aparecer no final: %q", result)
		}
		if strings.Contains(result, "resumo") {
			t.Error("conteúdo do summary não deve aparecer")
		}
		if !strings.Contains(result, "texto") {
			t.Error("conteúdo principal deve ser preservado")
		}
	})

	t.Run("tag chega fragmentada em múltiplos deltas", func(t *testing.T) {
		result := collect("Resp.", "\n", "<sum", "mary>", "resumo", "</sum", "mary>")
		if strings.Contains(result, "resumo") {
			t.Errorf("summary fragmentado não deve aparecer: %q", result)
		}
		if strings.Contains(result, "<summary>") || strings.Contains(result, "</summary>") {
			t.Errorf("tags não devem aparecer: %q", result)
		}
		if !strings.Contains(result, "Resp.") {
			t.Error("conteúdo principal deve ser preservado")
		}
	})

	t.Run("conteúdo sem summary passa intacto", func(t *testing.T) {
		result := collect("Resposta ", "sem ", "nenhuma ", "tag.")
		if result != "Resposta sem nenhuma tag." {
			t.Errorf("conteúdo sem tag deve passar intacto: %q", result)
		}
	})

	t.Run("summary no meio do stream: conteúdo antes e depois preservados", func(t *testing.T) {
		result := collect("antes ", "<summary>oculto</summary>", " depois")
		if strings.Contains(result, "oculto") {
			t.Error("summary não deve aparecer")
		}
		if !strings.Contains(result, "antes") {
			t.Error("conteúdo antes deve ser preservado")
		}
		if !strings.Contains(result, "depois") {
			t.Error("conteúdo depois deve ser preservado")
		}
	})
}

// ── 12. summaryDeltaProxy: whitespace não relacionado a <summary> é emitido ───

func TestSummaryDeltaProxyWhitespacePassthrough(t *testing.T) {
	collect := func(inputs ...string) string {
		var out strings.Builder
		p := newSummaryDeltaProxy(func(s string) { out.WriteString(s) })
		for _, s := range inputs {
			p.Write(s)
		}
		p.Flush()
		return out.String()
	}

	t.Run("whitespace seguido de conteúdo regular é emitido", func(t *testing.T) {
		result := collect("hello\n\nworld")
		if result != "hello\n\nworld" {
			t.Errorf("whitespace entre palavras deve ser emitido: %q", result)
		}
	})

	t.Run("whitespace no Flush é emitido se não preceder tag", func(t *testing.T) {
		var out strings.Builder
		p := newSummaryDeltaProxy(func(s string) { out.WriteString(s) })
		p.Write("texto\n\n")
		p.Flush()
		if !strings.Contains(out.String(), "\n\n") {
			t.Errorf("whitespace final deve ser emitido no Flush: %q", out.String())
		}
	})
}

// ── 13. summaryDeltaProxy: tag falsa não bloqueia output ─────────────────────

func TestSummaryDeltaProxyFalseTag(t *testing.T) {
	collect := func(input string) string {
		var out strings.Builder
		p := newSummaryDeltaProxy(func(s string) { out.WriteString(s) })
		p.Write(input)
		p.Flush()
		return out.String()
	}

	t.Run("<summaryX> não é suprimida", func(t *testing.T) {
		result := collect("texto <summaryX>conteúdo</summaryX>")
		if !strings.Contains(result, "<summaryX>") {
			t.Errorf("tag diferente não deve ser suprimida: %q", result)
		}
	})

	t.Run("<Summary> maiúsculo não é suprimida (case-sensitive)", func(t *testing.T) {
		result := collect("texto <Summary>conteúdo</Summary>")
		if !strings.Contains(result, "<Summary>") {
			t.Errorf("tag case-sensitive não deve ser suprimida: %q", result)
		}
	})

	t.Run("< sem continuação é emitido no Flush", func(t *testing.T) {
		result := collect("texto <")
		if !strings.Contains(result, "<") {
			t.Errorf("< isolado deve ser emitido: %q", result)
		}
	})
}

// ── 14. summaryDeltaProxy: Flush emite pendente, descarta tag incompleta ──────

func TestSummaryDeltaProxyFlush(t *testing.T) {
	t.Run("Flush com tag incompleta: conteúdo descartado", func(t *testing.T) {
		var out strings.Builder
		p := newSummaryDeltaProxy(func(s string) { out.WriteString(s) })
		p.Write("texto\n<summary>início sem fechamento")
		p.Flush()
		// whitespace + tag incompleta devem ser descartados
		if strings.Contains(out.String(), "<summary>") {
			t.Errorf("tag incompleta não deve aparecer: %q", out.String())
		}
		if strings.Contains(out.String(), "início sem fechamento") {
			t.Errorf("conteúdo de tag incompleta não deve aparecer: %q", out.String())
		}
	})

	t.Run("Flush com tBuf parcial emite o conteúdo", func(t *testing.T) {
		var out strings.Builder
		p := newSummaryDeltaProxy(func(s string) { out.WriteString(s) })
		p.Write("texto <abc") // <abc não é prefixo de <summary> após 'a'
		p.Flush()
		if !strings.Contains(out.String(), "<abc") {
			t.Errorf("conteúdo após mismatch deve ser emitido: %q", out.String())
		}
	})
}

// ── 15. Compressão de contexto com offset de run ─────────────────────────────

func TestContextCompressionWithRunOffset(t *testing.T) {
	// Simula o que o TUI faz: múltiplos runs com convTurnOffset.
	// Run 1: turns TUI 0,1 → conv indices 0,1 (offset=0)
	// Run 2: turns TUI 2,3 → conv indices 0,1 (offset=2)
	// Marks do run 1 devem ser propagados ao conv do run 2 via offset.

	t.Run("mark de run anterior propagado com offset correto", func(t *testing.T) {
		// Simula conv do run 1 com 2 turns
		conv1 := buildConv([]struct{ user, assistant string }{
			{"q0", "a0"},
			{"q1", "a1"},
		})
		// Usuário marca turn TUI 1 (conv1 index 1) como dismissed
		conv1.SetMark(1, TurnMarkDismissed, "resumo do turn 1")

		// Agora simula run 2 com novo conv (offset = 2)
		conv2 := buildConv([]struct{ user, assistant string }{
			{"q2", "a2"}, // TUI turn 2 → conv2 index 0
			{"q3", "a3"}, // TUI turn 3 → conv2 index 1
		})

		// Propaga marks do run anterior com offset
		offset := 2
		for tuiIdx := 0; tuiIdx < 2; tuiIdx++ {
			meta := conv1.GetMark(tuiIdx)
			convIdx := tuiIdx - offset // negativo: turns do run 1 não existem no conv2
			if convIdx >= 0 {
				conv2.SetMark(convIdx, meta.Mark, meta.Summary)
			}
		}

		// conv2 não deve ter nenhum mark (turns 0,1 do run 1 ficam com offset negativo)
		msgs := conv2.Messages()
		if len(msgs) != 4 {
			t.Fatalf("conv2 não deve ter marks do run anterior, esperado 4 msgs, obtido %d", len(msgs))
		}
		if msgs[0].Content != "q2" {
			t.Errorf("conv2 deve ter seus próprios turns intactos: %q", msgs[0].Content)
		}
	})

	t.Run("mark no run atual propagado corretamente via offset", func(t *testing.T) {
		// Run 2 com offset=2, usuário marca turn TUI 2 → conv2 index 0
		conv2 := buildConv([]struct{ user, assistant string }{
			{"q2", "a2"}, // conv index 0
			{"q3", "a3"}, // conv index 1
		})

		offset := 2
		tuiIdx := 2
		convIdx := tuiIdx - offset // = 0
		conv2.SetMark(convIdx, TurnMarkDismissed, "resumo q2")

		msgs := conv2.Messages()
		// turn 0 dismissed: 2 placeholders + turn 1: 2 msgs = 4
		if len(msgs) != 4 {
			t.Fatalf("esperado 4 msgs, obtido %d", len(msgs))
		}
		if msgs[0].Content != "[turn dismissed]" {
			t.Errorf("turn com offset correto deve ser dismissed: %q", msgs[0].Content)
		}
		if msgs[2].Content != "q3" {
			t.Errorf("turn 1 deve estar intacto: %q", msgs[2].Content)
		}
	})
}
