package audit

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func openTestLogger(t *testing.T) *SQLiteLogger {
	t.Helper()
	l, err := OpenSQLiteLogger(t.TempDir())
	if err != nil {
		t.Fatalf("OpenSQLiteLogger: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l
}

func TestSQLiteLogger_Log_StoresEntry(t *testing.T) {
	l := openTestLogger(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond)

	e := Entry{
		Timestamp:  now,
		AgentName:  "agent1",
		SessionID:  "sess1",
		ToolName:   "bash",
		ToolInput:  json.RawMessage(`{"command":"ls"}`),
		RiskLevel:  "medium",
		Decision:   "allow",
		DurationMs: 42,
		Error:      "",
	}
	l.Log(ctx, e)

	entries, err := l.Query(AuditFilter{SessionID: "sess1"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	got := entries[0]
	if got.AgentName != "agent1" {
		t.Errorf("AgentName = %q, want %q", got.AgentName, "agent1")
	}
	if got.ToolName != "bash" {
		t.Errorf("ToolName = %q, want %q", got.ToolName, "bash")
	}
	if got.Decision != "allow" {
		t.Errorf("Decision = %q, want %q", got.Decision, "allow")
	}
	if got.DurationMs != 42 {
		t.Errorf("DurationMs = %d, want 42", got.DurationMs)
	}
}

func TestSQLiteLogger_Query_FilterByDecision(t *testing.T) {
	l := openTestLogger(t)
	ctx := context.Background()

	for _, dec := range []string{"allow", "deny", "allow"} {
		l.Log(ctx, Entry{
			Timestamp: time.Now(),
			SessionID: "s",
			ToolName:  "read",
			Decision:  dec,
		})
	}

	allows, err := l.Query(AuditFilter{Decision: "allow"})
	if err != nil {
		t.Fatal(err)
	}
	if len(allows) != 2 {
		t.Errorf("expected 2 allow entries, got %d", len(allows))
	}

	denies, err := l.Query(AuditFilter{Decision: "deny"})
	if err != nil {
		t.Fatal(err)
	}
	if len(denies) != 1 {
		t.Errorf("expected 1 deny entry, got %d", len(denies))
	}
}

func TestSQLiteLogger_Query_FilterByTimeRange(t *testing.T) {
	l := openTestLogger(t)
	ctx := context.Background()

	t0 := time.Now()
	l.Log(ctx, Entry{Timestamp: t0.Add(-2 * time.Hour), SessionID: "s", ToolName: "read", Decision: "allow"})
	l.Log(ctx, Entry{Timestamp: t0.Add(-1 * time.Hour), SessionID: "s", ToolName: "read", Decision: "allow"})
	l.Log(ctx, Entry{Timestamp: t0.Add(1 * time.Hour), SessionID: "s", ToolName: "read", Decision: "allow"})

	recent, err := l.Query(AuditFilter{Since: t0.Add(-90 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 2 {
		t.Errorf("expected 2 recent entries, got %d", len(recent))
	}
}

func TestSQLiteLogger_Stats_CountsByTool(t *testing.T) {
	l := openTestLogger(t)
	ctx := context.Background()

	tools := []string{"bash", "read", "bash", "write", "bash"}
	for _, tool := range tools {
		l.Log(ctx, Entry{
			Timestamp: time.Now(),
			SessionID: "sess",
			ToolName:  tool,
			Decision:  "allow",
		})
	}

	stats, err := l.Stats("sess")
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalCalls != 5 {
		t.Errorf("TotalCalls = %d, want 5", stats.TotalCalls)
	}
	if stats.ByTool["bash"] != 3 {
		t.Errorf("ByTool[bash] = %d, want 3", stats.ByTool["bash"])
	}
	if stats.ByTool["read"] != 1 {
		t.Errorf("ByTool[read] = %d, want 1", stats.ByTool["read"])
	}
}

func TestSQLiteLogger_Query_FilterByRisk(t *testing.T) {
	l := openTestLogger(t)
	ctx := context.Background()

	for _, risk := range []string{"low", "high", "low", "critical"} {
		l.Log(ctx, Entry{
			Timestamp: time.Now(),
			ToolName:  "bash",
			Decision:  "allow",
			RiskLevel: risk,
		})
	}

	lows, err := l.Query(AuditFilter{RiskLevel: "low"})
	if err != nil {
		t.Fatal(err)
	}
	if len(lows) != 2 {
		t.Errorf("expected 2 low entries, got %d", len(lows))
	}
}

func TestSQLiteLogger_MultipleAgents(t *testing.T) {
	l := openTestLogger(t)
	ctx := context.Background()

	l.Log(ctx, Entry{Timestamp: time.Now(), AgentName: "alice", SessionID: "s1", ToolName: "read", Decision: "allow"})
	l.Log(ctx, Entry{Timestamp: time.Now(), AgentName: "bob", SessionID: "s2", ToolName: "write", Decision: "deny"})
	l.Log(ctx, Entry{Timestamp: time.Now(), AgentName: "alice", SessionID: "s1", ToolName: "bash", Decision: "allow"})

	aliceEntries, err := l.Query(AuditFilter{AgentName: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if len(aliceEntries) != 2 {
		t.Errorf("alice: expected 2, got %d", len(aliceEntries))
	}

	s2Entries, err := l.Query(AuditFilter{SessionID: "s2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(s2Entries) != 1 {
		t.Errorf("sess s2: expected 1, got %d", len(s2Entries))
	}
}

func TestSQLiteLogger_ToolInputHash(t *testing.T) {
	l := openTestLogger(t)
	ctx := context.Background()

	input := json.RawMessage(`{"command":"ls -la"}`)
	l.Log(ctx, Entry{
		Timestamp: time.Now(),
		ToolName:  "bash",
		Decision:  "allow",
		ToolInput: input,
	})
	l.Log(ctx, Entry{
		Timestamp: time.Now(),
		ToolName:  "bash",
		Decision:  "allow",
		ToolInput: input, // same input
	})

	entries, err := l.Query(AuditFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 2 {
		t.Fatal("expected at least 2 entries")
	}
	h1 := entries[0].ToolInputHash
	h2 := entries[1].ToolInputHash
	if h1 == "" {
		t.Error("ToolInputHash should not be empty")
	}
	if h1 != h2 {
		t.Errorf("same input should produce same hash: %q != %q", h1, h2)
	}
}
