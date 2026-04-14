// Package audit provides structured logging for tool execution decisions.
package audit

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"
)

// Entry represents one tool execution decision event.
type Entry struct {
	Timestamp  time.Time
	AgentName  string
	SessionID  string
	ToolName   string
	ToolInput  json.RawMessage
	RiskLevel  string
	Decision   string // "allow" | "deny" | "ask-approved" | "ask-denied"
	DurationMs int64
	Error      string
}

// Logger appends audit entries.
type Logger interface {
	Log(ctx context.Context, entry Entry)
}

// SlogLogger writes structured JSON via slog.
type SlogLogger struct{ Logger *slog.Logger }

func (l *SlogLogger) Log(_ context.Context, e Entry) {
	l.Logger.Info("tool_audit",
		"timestamp", e.Timestamp,
		"agent", e.AgentName,
		"session", e.SessionID,
		"tool", e.ToolName,
		"risk", e.RiskLevel,
		"decision", e.Decision,
		"duration_ms", e.DurationMs,
		"error", e.Error,
	)
}

// NoopLogger discards all entries. Default when no audit logger configured.
type NoopLogger struct{}

func (NoopLogger) Log(_ context.Context, _ Entry) {}

// MultiLogger fans out to multiple loggers.
type MultiLogger []Logger

func (m MultiLogger) Log(ctx context.Context, e Entry) {
	for _, l := range m {
		l.Log(ctx, e)
	}
}
