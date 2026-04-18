package policy

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/co2-lab/polvo/internal/risk"
)

func newStore(t *testing.T) *PolicyStore {
	t.Helper()
	return NewPolicyStore(filepath.Join(t.TempDir(), "policies.json"))
}

func TestPolicyStore_EvaluateMatchesAgentAndTool(t *testing.T) {
	ps := newStore(t)
	_ = ps.Upsert(Policy{
		Scope:    PolicyScope{AgentName: "agent1", ToolName: "bash"},
		Decision: PolicyAllow,
		TTL:      TTLSession,
	})
	dec, ok := ps.Evaluate("agent1", "bash", risk.RiskMedium)
	if !ok {
		t.Fatal("expected policy match")
	}
	if dec != PolicyAllow {
		t.Errorf("expected PolicyAllow, got %v", dec)
	}
}

func TestPolicyStore_EvaluateAgentOnlyScope(t *testing.T) {
	ps := newStore(t)
	_ = ps.Upsert(Policy{
		Scope:    PolicyScope{AgentName: "agent1"},
		Decision: PolicyAllow,
		TTL:      TTLSession,
	})
	dec, ok := ps.Evaluate("agent1", "any_tool", risk.RiskLow)
	if !ok {
		t.Fatal("expected match for agent-only scope")
	}
	if dec != PolicyAllow {
		t.Errorf("expected PolicyAllow, got %v", dec)
	}
}

func TestPolicyStore_EvaluateGlobalScope(t *testing.T) {
	ps := newStore(t)
	_ = ps.Upsert(Policy{
		Scope:    PolicyScope{},
		Decision: PolicyDeny,
		TTL:      TTLSession,
	})
	dec, ok := ps.Evaluate("any_agent", "any_tool", risk.RiskLow)
	if !ok {
		t.Fatal("expected global match")
	}
	if dec != PolicyDeny {
		t.Errorf("expected PolicyDeny, got %v", dec)
	}
}

func TestPolicyStore_PrecedenceOrder(t *testing.T) {
	ps := newStore(t)
	// Global: deny
	_ = ps.Upsert(Policy{Scope: PolicyScope{}, Decision: PolicyDeny, TTL: TTLSession})
	// Tool-only: deny
	_ = ps.Upsert(Policy{Scope: PolicyScope{ToolName: "bash"}, Decision: PolicyDeny, TTL: TTLSession})
	// Agent-only: allow
	_ = ps.Upsert(Policy{Scope: PolicyScope{AgentName: "agent1"}, Decision: PolicyAllow, TTL: TTLSession})
	// Agent+Tool: deny
	_ = ps.Upsert(Policy{
		Scope:    PolicyScope{AgentName: "agent1", ToolName: "bash"},
		Decision: PolicyDeny,
		TTL:      TTLSession,
	})

	// Most specific (agent+tool) should win.
	dec, ok := ps.Evaluate("agent1", "bash", risk.RiskMedium)
	if !ok {
		t.Fatal("expected match")
	}
	if dec != PolicyDeny {
		t.Errorf("expected PolicyDeny (most specific), got %v", dec)
	}
}

func TestPolicyStore_ExpiredPoliciesIgnored(t *testing.T) {
	ps := newStore(t)
	_ = ps.Upsert(Policy{
		Scope:     PolicyScope{ToolName: "bash"},
		Decision:  PolicyAllow,
		TTL:       TTLTimed,
		ExpiresAt: time.Now().Add(-1 * time.Hour), // already expired
	})
	_, ok := ps.Evaluate("any", "bash", risk.RiskMedium)
	if ok {
		t.Error("expired policy should not match")
	}
}

func TestPolicyStore_SessionPoliciesNotPersisted(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "policies.json")
	ps := NewPolicyStore(filePath)

	_ = ps.Upsert(Policy{
		Scope:    PolicyScope{ToolName: "bash"},
		Decision: PolicyAllow,
		TTL:      TTLSession,
	})
	_ = ps.Upsert(Policy{
		Scope:    PolicyScope{ToolName: "write"},
		Decision: PolicyDeny,
		TTL:      TTLPermanent,
	})

	// Reload from disk.
	ps2 := NewPolicyStore(filePath)
	policies := ps2.All()
	for _, p := range policies {
		if p.TTL == TTLSession {
			t.Error("session policy should not be persisted")
		}
	}
	// The permanent one should survive.
	_, ok := ps2.Evaluate("any", "write", risk.RiskLow)
	if !ok {
		t.Error("permanent policy should survive reload")
	}
}

func TestPolicyStore_AtomicSave(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "policies.json")
	ps := NewPolicyStore(filePath)

	_ = ps.Upsert(Policy{
		Scope:    PolicyScope{AgentName: "a", ToolName: "t"},
		Decision: PolicyAllow,
		TTL:      TTLPermanent,
	})

	// Reload and verify round-trip.
	ps2 := NewPolicyStore(filePath)
	dec, ok := ps2.Evaluate("a", "t", risk.RiskLow)
	if !ok {
		t.Fatal("expected policy after reload")
	}
	if dec != PolicyAllow {
		t.Errorf("expected PolicyAllow after reload, got %v", dec)
	}
}

func TestPolicyStore_PurgeRemovesExpired(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "policies.json")
	ps := NewPolicyStore(filePath)

	_ = ps.Upsert(Policy{
		Scope:     PolicyScope{ToolName: "expired"},
		Decision:  PolicyAllow,
		TTL:       TTLTimed,
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	})
	_ = ps.Upsert(Policy{
		Scope:    PolicyScope{ToolName: "active"},
		Decision: PolicyAllow,
		TTL:      TTLPermanent,
	})

	ps.Purge()

	all := ps.All()
	if len(all) != 1 {
		t.Errorf("expected 1 active policy after purge, got %d", len(all))
	}
	if all[0].Scope.ToolName != "active" {
		t.Errorf("wrong policy survived purge: %v", all[0].Scope.ToolName)
	}
}

func TestPolicyStore_UpsertReplacesExistingScope(t *testing.T) {
	ps := newStore(t)
	scope := PolicyScope{ToolName: "bash"}
	_ = ps.Upsert(Policy{Scope: scope, Decision: PolicyAllow, TTL: TTLSession})
	_ = ps.Upsert(Policy{Scope: scope, Decision: PolicyDeny, TTL: TTLSession})

	all := ps.All()
	if len(all) != 1 {
		t.Errorf("expected upsert to replace, got %d policies", len(all))
	}
	if all[0].Decision != PolicyDeny {
		t.Errorf("expected PolicyDeny after upsert, got %v", all[0].Decision)
	}
}

func TestPolicyStore_Race(t *testing.T) {
	ps := newStore(t)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = ps.Upsert(Policy{
				Scope:    PolicyScope{ToolName: "bash"},
				Decision: PolicyAllow,
				TTL:      TTLSession,
			})
			ps.Evaluate("any", "bash", risk.RiskLow)
		}(i)
	}
	wg.Wait()
}
