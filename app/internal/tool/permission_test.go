package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// TestDefaultPermissionRulesContent
// ---------------------------------------------------------------------------

// TestDefaultPermissionRulesContent protege contra drift silencioso entre o spec
// (agent-runtime.draft.md § Tools) e a implementação.
// Spec: read=allow, glob=allow, grep=allow, ls=allow, write=ask, edit=ask, bash=ask
func TestDefaultPermissionRulesContent(t *testing.T) {
	want := map[string]PermissionLevel{
		"read":  PermAllow,
		"glob":  PermAllow,
		"grep":  PermAllow,
		"ls":    PermAllow,
		"write": PermAsk,
		"edit":  PermAsk,
		"bash":  PermAsk,
	}

	rules := DefaultPermissionRules()
	got := make(map[string]PermissionLevel, len(rules))
	for _, r := range rules {
		got[r.Tool] = r.Level
	}

	for tool, wantLevel := range want {
		if gotLevel, ok := got[tool]; !ok {
			t.Errorf("tool %q ausente em DefaultPermissionRules()", tool)
		} else if gotLevel != wantLevel {
			t.Errorf("tool %q: nível = %q, want %q", tool, gotLevel, wantLevel)
		}
	}

	// GAP: browser=ask não está em DefaultPermissionRules() porque browser tool não implementado.
	t.Log("GAP: browser tool não implementado — ausente de DefaultPermissionRules()")
}

// ---------------------------------------------------------------------------
// TestPermissionChecker_Check
// ---------------------------------------------------------------------------

func TestPermissionChecker_Check(t *testing.T) {
	rules := []PermissionRule{
		{Tool: "read", Level: PermAllow},
		{Tool: "write", Level: PermAsk},
		{Tool: "bash", Level: PermDeny},
	}
	checker := NewPermissionChecker(rules)

	cases := []struct {
		tool      string
		wantLevel PermissionLevel
	}{
		{"read", PermAllow},
		{"write", PermAsk},
		{"bash", PermDeny},
		{"unknown_tool", PermAsk}, // default para ferramenta não mapeada
		{"", PermAsk},             // tool vazia → default
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.tool, func(t *testing.T) {
			got := checker.Check(tc.tool)
			if got != tc.wantLevel {
				t.Errorf("Check(%q) = %q, want %q", tc.tool, got, tc.wantLevel)
			}
		})
	}
}

func TestPermissionChecker_EmptyRules(t *testing.T) {
	checker := NewPermissionChecker(nil)
	if got := checker.Check("bash"); got != PermAsk {
		t.Errorf("empty rules: Check(bash) = %q, want %q", got, PermAsk)
	}
}

// ---------------------------------------------------------------------------
// TestGuardedRegistry_FlowAskAllowDeny
// ---------------------------------------------------------------------------

// TestGuardedRegistry_FlowAskAllowDeny replica os 3 fluxos do Cline Task.ask.test.ts:
// yesButtonClicked (allow), noButtonClicked (deny), e erro na interação (error).
func TestGuardedRegistry_FlowAskAllowDeny(t *testing.T) {
	reg := NewRegistry()
	reg.Register(NewThink()) // think não tem side effects

	rules := []PermissionRule{
		{Tool: "think", Level: PermAsk},
	}
	input := json.RawMessage(`{"thought":"test reasoning"}`)

	t.Run("ask_then_allow", func(t *testing.T) {
		// Usuário aprova (yesButtonClicked no Cline) → tool executa normalmente
		askFn := func(name string, _ json.RawMessage) (bool, error) {
			return true, nil
		}
		g := NewGuardedRegistry(reg, rules, askFn)
		res, err := g.Execute(context.Background(), "think", input)
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Errorf("expected successful execution, got error: %s", res.Content)
		}
	})

	t.Run("ask_then_deny", func(t *testing.T) {
		// Usuário nega (noButtonClicked no Cline) → ErrorResult com "denied"
		askFn := func(name string, _ json.RawMessage) (bool, error) {
			return false, nil
		}
		g := NewGuardedRegistry(reg, rules, askFn)
		res, err := g.Execute(context.Background(), "think", input)
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsError {
			t.Error("expected IsError=true when user denies")
		}
		if !strings.Contains(res.Content, "denied") {
			t.Errorf("expected 'denied' in message, got %q", res.Content)
		}
	})

	t.Run("ask_then_error", func(t *testing.T) {
		// Erro na interação → ErrorResult "permission check error", não "denied by user"
		// Padrão Cline: "Cline instance aborted" quando task é abortada durante ask
		askFn := func(name string, _ json.RawMessage) (bool, error) {
			return false, fmt.Errorf("user interaction failed")
		}
		g := NewGuardedRegistry(reg, rules, askFn)
		res, err := g.Execute(context.Background(), "think", input)
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsError {
			t.Error("expected IsError=true on askFn error")
		}
		if !strings.Contains(res.Content, "permission check error") {
			t.Errorf("expected 'permission check error' in message, got %q", res.Content)
		}
		// Distinção crítica: erro na interação != negação do usuário
		if strings.Contains(res.Content, "denied by user") {
			t.Errorf("'denied by user' não deve aparecer quando houve erro — got %q", res.Content)
		}
	})
}

// ---------------------------------------------------------------------------
// TestGuardedRegistry_Execute
// ---------------------------------------------------------------------------

func TestGuardedRegistry_Execute(t *testing.T) {
	reg := NewRegistry()
	reg.Register(NewThink())
	input := json.RawMessage(`{"thought":"test"}`)

	t.Run("perm_deny_nao_executa", func(t *testing.T) {
		rules := []PermissionRule{{Tool: "think", Level: PermDeny}}
		g := NewGuardedRegistry(reg, rules, nil)
		res, err := g.Execute(context.Background(), "think", input)
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsError {
			t.Error("expected IsError=true for denied tool")
		}
		if !strings.Contains(res.Content, "denied") {
			t.Errorf("expected 'denied' in message, got %q", res.Content)
		}
	})

	t.Run("perm_allow_executa", func(t *testing.T) {
		rules := []PermissionRule{{Tool: "think", Level: PermAllow}}
		g := NewGuardedRegistry(reg, rules, nil)
		res, err := g.Execute(context.Background(), "think", input)
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Errorf("expected success for allowed tool, got: %s", res.Content)
		}
	})

	t.Run("perm_ask_sem_askfn_executa", func(t *testing.T) {
		// askFn == nil → executa sem bloqueio (comportamento atual documentado)
		rules := []PermissionRule{{Tool: "think", Level: PermAsk}}
		g := NewGuardedRegistry(reg, rules, nil)
		res, err := g.Execute(context.Background(), "think", input)
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Errorf("expected success when askFn is nil, got: %s", res.Content)
		}
	})

	t.Run("tool_desconhecida", func(t *testing.T) {
		g := NewGuardedRegistry(reg, nil, nil)
		res, err := g.Execute(context.Background(), "nao_existe", input)
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsError {
			t.Error("expected IsError=true for unknown tool")
		}
		if !strings.Contains(res.Content, "unknown tool") {
			t.Errorf("expected 'unknown tool' in message, got %q", res.Content)
		}
	})

	t.Run("askfn_chamada_exatamente_uma_vez", func(t *testing.T) {
		// Padrão Cline: contador de chamadas para garantir que askFn não é invocada múltiplas vezes
		called := 0
		askFn := func(name string, _ json.RawMessage) (bool, error) {
			called++
			return true, nil
		}
		rules := []PermissionRule{{Tool: "think", Level: PermAsk}}
		g := NewGuardedRegistry(reg, rules, askFn)
		_, _ = g.Execute(context.Background(), "think", input)
		if called != 1 {
			t.Errorf("askFn called %d times, want exactly 1", called)
		}
	})
}

// ---------------------------------------------------------------------------
// TestGuardedRegistry_UnknownTool
// ---------------------------------------------------------------------------

// TestGuardedRegistry_UnknownTool verifica que executar uma tool inexistente
// retorna IsError com mensagem "unknown tool".
func TestGuardedRegistry_UnknownTool(t *testing.T) {
	reg := NewRegistry()
	g := NewGuardedRegistry(reg, nil, nil)
	res, err := g.Execute(context.Background(), "nao_existe", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected IsError=true for unknown tool")
	}
	if !strings.Contains(res.Content, "unknown tool") {
		t.Errorf("expected 'unknown tool' in message, got %q", res.Content)
	}
}

// ---------------------------------------------------------------------------
// TestGuardedRegistry_PermDeny
// ---------------------------------------------------------------------------

// TestGuardedRegistry_PermDeny verifica que uma tool com PermDeny nunca executa.
func TestGuardedRegistry_PermDeny(t *testing.T) {
	reg := NewRegistry()
	reg.Register(NewThink())
	rules := []PermissionRule{{Tool: "think", Level: PermDeny}}
	g := NewGuardedRegistry(reg, rules, nil)
	res, err := g.Execute(context.Background(), "think", json.RawMessage(`{"thought":"test"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected IsError=true for denied tool")
	}
	if !strings.Contains(res.Content, "denied") {
		t.Errorf("expected 'denied' in message, got %q", res.Content)
	}
}

// ---------------------------------------------------------------------------
// TestGuardedRegistry_AskFnCalledOnce
// ---------------------------------------------------------------------------

// TestGuardedRegistry_AskFnCalledOnce verifica que askFn é chamada exatamente
// uma vez por Execute — padrão Cline contador de chamadas.
func TestGuardedRegistry_AskFnCalledOnce(t *testing.T) {
	reg := NewRegistry()
	reg.Register(NewThink())

	called := 0
	askFn := func(name string, _ json.RawMessage) (bool, error) {
		called++
		return true, nil
	}
	rules := []PermissionRule{{Tool: "think", Level: PermAsk}}
	g := NewGuardedRegistry(reg, rules, askFn)
	_, err := g.Execute(context.Background(), "think", json.RawMessage(`{"thought":"test"}`))
	if err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Errorf("askFn called %d times, want exactly 1", called)
	}
}

// ---------------------------------------------------------------------------
// TestGuardedRegistry_NoSessionCache
// ---------------------------------------------------------------------------

// TestGuardedRegistry_NoSessionCache verifica que GuardedRegistry NÃO tem session cache.
// Contraste com Cline, que acumula aprovações em sessão: no Polvo, askFn é consultado
// em TODA chamada a Execute, sem memória entre chamadas.
func TestGuardedRegistry_NoSessionCache(t *testing.T) {
	// Cline acumula aprovações em sessão — usuário aprova uma vez e comandos similares
	// passam automaticamente. Polvo NÃO implementa esse modelo: askFn é consultado
	// em TODA chamada a Execute, sem memória entre chamadas.

	callCount := 0
	askFn := func(name string, _ json.RawMessage) (bool, error) {
		callCount++
		return true, nil
	}

	reg := NewRegistry()
	reg.Register(NewThink())
	guarded := NewGuardedRegistry(reg, []PermissionRule{
		{Tool: "think", Level: PermAsk},
	}, askFn)

	ctx := context.Background()
	input := json.RawMessage(`{"thought":"test"}`)

	const executions = 3
	for i := 0; i < executions; i++ {
		_, err := guarded.Execute(ctx, "think", input)
		if err != nil {
			t.Fatalf("Execute %d: unexpected error: %v", i+1, err)
		}
	}

	if callCount != executions {
		t.Errorf("askFn chamado %d vezes, want %d — GuardedRegistry não deve ter session cache", callCount, executions)
	}
}
