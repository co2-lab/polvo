package audit

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const auditSchema = `
PRAGMA journal_mode=WAL;
PRAGMA synchronous=NORMAL;

CREATE TABLE IF NOT EXISTS audit_entries (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp        INTEGER NOT NULL,
    session_id       TEXT    NOT NULL DEFAULT '',
    agent_name       TEXT    NOT NULL DEFAULT '',
    tool_name        TEXT    NOT NULL DEFAULT '',
    tool_input_hash  TEXT    NOT NULL DEFAULT '',
    risk_level       TEXT    NOT NULL DEFAULT 'low',
    decision         TEXT    NOT NULL DEFAULT 'allow',
    duration_ms      INTEGER NOT NULL DEFAULT 0,
    error            TEXT    NOT NULL DEFAULT '',
    replay_id        TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_audit_session   ON audit_entries(session_id);
CREATE INDEX IF NOT EXISTS idx_audit_agent     ON audit_entries(agent_name);
CREATE INDEX IF NOT EXISTS idx_audit_tool      ON audit_entries(tool_name);
CREATE INDEX IF NOT EXISTS idx_audit_decision  ON audit_entries(decision);
CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_entries(timestamp);
CREATE INDEX IF NOT EXISTS idx_audit_risk      ON audit_entries(risk_level);
`

// AuditFilter specifies query constraints for SQLiteLogger.Query.
type AuditFilter struct {
	SessionID string
	AgentName string
	ToolName  string
	Decision  string
	RiskLevel string
	Since     time.Time
	Until     time.Time
	Limit     int
}

// AuditStats summarises audit activity for a session.
type AuditStats struct {
	TotalCalls   int
	AllowedCalls int
	DeniedCalls  int
	ByTool       map[string]int
	ByRisk       map[string]int
}

// SQLiteLogger persists audit entries to .polvo/audit.db.
// It implements the Logger interface.
type SQLiteLogger struct {
	db *sql.DB
}

// OpenSQLiteLogger creates or opens .polvo/audit.db under root.
func OpenSQLiteLogger(root string) (*SQLiteLogger, error) {
	dir := filepath.Join(root, ".polvo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("audit: mkdir: %w", err)
	}
	dbPath := filepath.Join(dir, "audit.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("audit: open db: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(auditSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("audit: migrate: %w", err)
	}
	return &SQLiteLogger{db: db}, nil
}

// Close releases the database connection.
func (l *SQLiteLogger) Close() error { return l.db.Close() }

// Log inserts an audit entry into the database.
func (l *SQLiteLogger) Log(_ context.Context, e Entry) {
	hash := inputHash(e.ToolInput)
	_, _ = l.db.Exec(
		`INSERT INTO audit_entries
		 (timestamp, session_id, agent_name, tool_name, tool_input_hash,
		  risk_level, decision, duration_ms, error, replay_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Timestamp.UnixNano(),
		e.SessionID,
		e.AgentName,
		e.ToolName,
		hash,
		e.RiskLevel,
		e.Decision,
		e.DurationMs,
		e.Error,
		e.ReplayID,
	)
}

// Query returns audit entries matching the filter, ordered by timestamp desc.
func (l *SQLiteLogger) Query(f AuditFilter) ([]Entry, error) {
	q := `SELECT timestamp, session_id, agent_name, tool_name, tool_input_hash,
	             risk_level, decision, duration_ms, error, replay_id
	      FROM audit_entries WHERE 1=1`
	var args []any

	if f.SessionID != "" {
		q += " AND session_id = ?"
		args = append(args, f.SessionID)
	}
	if f.AgentName != "" {
		q += " AND agent_name = ?"
		args = append(args, f.AgentName)
	}
	if f.ToolName != "" {
		q += " AND tool_name = ?"
		args = append(args, f.ToolName)
	}
	if f.Decision != "" {
		q += " AND decision = ?"
		args = append(args, f.Decision)
	}
	if f.RiskLevel != "" {
		q += " AND risk_level = ?"
		args = append(args, f.RiskLevel)
	}
	if !f.Since.IsZero() {
		q += " AND timestamp >= ?"
		args = append(args, f.Since.UnixNano())
	}
	if !f.Until.IsZero() {
		q += " AND timestamp <= ?"
		args = append(args, f.Until.UnixNano())
	}
	q += " ORDER BY timestamp DESC"
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", f.Limit)
	}

	rows, err := l.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		var ts int64
		var hash, replayID string
		if err := rows.Scan(&ts, &e.SessionID, &e.AgentName, &e.ToolName,
			&hash, &e.RiskLevel, &e.Decision, &e.DurationMs, &e.Error, &replayID); err != nil {
			return nil, err
		}
		e.Timestamp = time.Unix(0, ts)
		e.ToolInputHash = hash
		e.ReplayID = replayID
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// Stats returns aggregate counts for a session (empty sessionID = all sessions).
func (l *SQLiteLogger) Stats(sessionID string) (AuditStats, error) {
	var stats AuditStats
	stats.ByTool = make(map[string]int)
	stats.ByRisk = make(map[string]int)

	cond := ""
	var args []any
	if sessionID != "" {
		cond = "WHERE session_id = ?"
		args = append(args, sessionID)
	}

	row := l.db.QueryRow(
		`SELECT COUNT(*),
		        SUM(CASE WHEN decision IN ('allow','ask-approved') THEN 1 ELSE 0 END),
		        SUM(CASE WHEN decision IN ('deny','ask-denied') THEN 1 ELSE 0 END)
		 FROM audit_entries `+cond, args...)
	_ = row.Scan(&stats.TotalCalls, &stats.AllowedCalls, &stats.DeniedCalls)

	rows, err := l.db.Query(`SELECT tool_name, COUNT(*) FROM audit_entries `+cond+` GROUP BY tool_name`, args...)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var tool string
			var cnt int
			if rows.Scan(&tool, &cnt) == nil {
				stats.ByTool[tool] = cnt
			}
		}
	}

	rows2, err := l.db.Query(`SELECT risk_level, COUNT(*) FROM audit_entries `+cond+` GROUP BY risk_level`, args...)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var risk string
			var cnt int
			if rows2.Scan(&risk, &cnt) == nil {
				stats.ByRisk[risk] = cnt
			}
		}
	}

	return stats, nil
}

// inputHash returns the SHA-256 hex digest of the canonical JSON input.
func inputHash(input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	h := sha256.Sum256(input)
	return hex.EncodeToString(h[:])
}
