package audit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/co2-lab/polvo/internal/audit"
)

func sampleEntry() audit.Entry {
	return audit.Entry{
		Timestamp:  time.Now(),
		AgentName:  "test-agent",
		SessionID:  "sess-123",
		ToolName:   "write",
		ToolInput:  json.RawMessage(`{"path":"foo.txt","content":"hello"}`),
		RiskLevel:  "low",
		Decision:   "allow",
		DurationMs: 5,
		Error:      "",
	}
}

func TestSlogLogger_LogsEntry(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	logger := &audit.SlogLogger{Logger: slog.New(handler)}

	logger.Log(context.Background(), sampleEntry())

	output := buf.String()
	if !strings.Contains(output, "tool_audit") {
		t.Errorf("expected 'tool_audit' in log output, got: %s", output)
	}
	if !strings.Contains(output, "write") {
		t.Errorf("expected tool name 'write' in log output, got: %s", output)
	}
	if !strings.Contains(output, "test-agent") {
		t.Errorf("expected agent name 'test-agent' in log output, got: %s", output)
	}
}

func TestNoopLogger_DoesNotPanic(t *testing.T) {
	logger := audit.NoopLogger{}
	// Must not panic
	logger.Log(context.Background(), sampleEntry())
}

func TestMultiLogger_FansOut(t *testing.T) {
	type captureLogger struct {
		entries []audit.Entry
	}

	type capLog struct {
		entries *[]audit.Entry
	}
	log1entries := []audit.Entry{}
	log2entries := []audit.Entry{}

	capLogger := func(entries *[]audit.Entry) audit.Logger {
		return &capLogImpl{entries: entries}
	}

	multi := audit.MultiLogger{
		capLogger(&log1entries),
		capLogger(&log2entries),
	}

	e := sampleEntry()
	multi.Log(context.Background(), e)

	if len(log1entries) != 1 {
		t.Errorf("logger 1: expected 1 entry, got %d", len(log1entries))
	}
	if len(log2entries) != 1 {
		t.Errorf("logger 2: expected 1 entry, got %d", len(log2entries))
	}
	if log1entries[0].ToolName != "write" {
		t.Errorf("logger 1: expected ToolName 'write', got %q", log1entries[0].ToolName)
	}
}

// capLogImpl is a test-local capturing logger.
type capLogImpl struct {
	entries *[]audit.Entry
}

func (c *capLogImpl) Log(_ context.Context, e audit.Entry) {
	*c.entries = append(*c.entries, e)
}
