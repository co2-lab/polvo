// Package memory provides shared persistent memory for agents via SQLite.
package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/co2-lab/polvo/internal/secrets"
	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

const schema = `
PRAGMA journal_mode=WAL;
PRAGMA synchronous=NORMAL;
PRAGMA foreign_keys=ON;

CREATE TABLE IF NOT EXISTS sessions (
    id         TEXT    PRIMARY KEY,
    agent      TEXT    NOT NULL,
    started_at INTEGER NOT NULL,
    ended_at   INTEGER,
    metadata   TEXT    DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS memory (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT,
    agent      TEXT    NOT NULL,
    file       TEXT,
    type       TEXT    NOT NULL CHECK(type IN ('observation','decision','issue','context','metrics','cost','audit')),
    content    TEXT    NOT NULL,
    timestamp  INTEGER NOT NULL,
    expires_at INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY(session_id) REFERENCES sessions(id)
);

CREATE INDEX IF NOT EXISTS idx_memory_agent      ON memory(agent);
CREATE INDEX IF NOT EXISTS idx_memory_file       ON memory(file);
CREATE INDEX IF NOT EXISTS idx_memory_type       ON memory(type);
CREATE INDEX IF NOT EXISTS idx_memory_session    ON memory(session_id);
CREATE INDEX IF NOT EXISTS idx_memory_timestamp  ON memory(timestamp);
CREATE INDEX IF NOT EXISTS idx_memory_expires_at ON memory(expires_at);
`

// TTLConfig defines how long each entry type lives in memory.
// Zero means no expiry (keep forever).
type TTLConfig struct {
	Observation time.Duration // default: 7 days
	Decision    time.Duration // default: 30 days
	Issue       time.Duration // default: 14 days
	Context     time.Duration // default: 3 days
	Metrics     time.Duration // default: 7 days
	Audit       time.Duration // default: 30 days
}

// DefaultTTLConfig returns a TTLConfig with sensible defaults.
func DefaultTTLConfig() TTLConfig {
	return TTLConfig{
		Observation: 7 * 24 * time.Hour,
		Decision:    30 * 24 * time.Hour,
		Issue:       14 * 24 * time.Hour,
		Context:     3 * 24 * time.Hour,
		Metrics:     7 * 24 * time.Hour,
		Audit:       30 * 24 * time.Hour,
	}
}

// Store is the shared memory store backed by SQLite.
type Store struct {
	db  *sql.DB
	ttl TTLConfig
}

// Entry is a single memory record.
type Entry struct {
	ID        int64
	SessionID string
	Agent     string
	File      string // optional
	Type      string // observation | decision | issue | context | metrics | cost | audit
	Content   string
	Timestamp int64 // unix nano
	ExpiresAt int64 // unix nano; 0 means no expiry
}

// Filter narrows a Read query.
type Filter struct {
	Agent     string
	File      string
	Type      string
	SessionID string
	Limit     int // 0 = no limit
}

// Open opens or creates .polvo/memory.db under root using DefaultTTLConfig.
func Open(root string) (*Store, error) {
	return OpenWithTTL(root, DefaultTTLConfig())
}

// OpenWithTTL opens or creates .polvo/memory.db under root with the given TTL config.
func OpenWithTTL(root string, ttl TTLConfig) (*Store, error) {
	dir := filepath.Join(root, ".polvo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating .polvo dir: %w", err)
	}

	dsn := filepath.Join(dir, "memory.db")
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening memory.db: %w", err)
	}

	// Single writer to avoid SQLITE_BUSY under WAL
	db.SetMaxOpenConns(1)

	if _, err := db.ExecContext(context.Background(), schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("applying memory schema: %w", err)
	}

	// Migrate: add expires_at column if it doesn't exist (for existing DBs).
	if err := migrateExpiresAt(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrating memory schema: %w", err)
	}

	return &Store{db: db, ttl: ttl}, nil
}

// migrateExpiresAt adds the expires_at column to an existing DB if absent.
func migrateExpiresAt(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(memory)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return err
		}
		if name == "expires_at" {
			return nil // already present
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = db.Exec(`ALTER TABLE memory ADD COLUMN expires_at INTEGER NOT NULL DEFAULT 0`)
	return err
}

// expiresAtForType returns the unix-nano expiry for the given entry type, or 0 if no TTL.
func (s *Store) expiresAtForType(entryType string) int64 {
	var d time.Duration
	switch entryType {
	case "observation":
		d = s.ttl.Observation
	case "decision":
		d = s.ttl.Decision
	case "issue":
		d = s.ttl.Issue
	case "context":
		d = s.ttl.Context
	case "metrics":
		d = s.ttl.Metrics
	case "audit":
		d = s.ttl.Audit
	}
	if d == 0 {
		return 0
	}
	return time.Now().Add(d).UnixNano()
}

// Write stores a memory entry.
func (s *Store) Write(entry Entry) error {
	if entry.Timestamp == 0 {
		entry.Timestamp = time.Now().UnixNano()
	}
	// Compute TTL-based expiry only when not already set.
	if entry.ExpiresAt == 0 {
		entry.ExpiresAt = s.expiresAtForType(entry.Type)
	}
	// Mask secrets before persisting.
	entry.Content, _ = secrets.MaskSecrets(entry.Content)
	_, err := s.db.Exec(
		`INSERT INTO memory (session_id, agent, file, type, content, timestamp, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		nullStr(entry.SessionID), entry.Agent, nullStr(entry.File),
		entry.Type, entry.Content, entry.Timestamp, entry.ExpiresAt,
	)
	return err
}

// Read returns entries matching the filter, ordered by timestamp desc.
func (s *Store) Read(filter Filter) ([]Entry, error) {
	query := `SELECT id, COALESCE(session_id,''), agent, COALESCE(file,''), type, content, timestamp, expires_at
	          FROM memory WHERE 1=1`
	var args []any

	if filter.Agent != "" {
		query += " AND agent = ?"
		args = append(args, filter.Agent)
	}
	if filter.File != "" {
		query += " AND file = ?"
		args = append(args, filter.File)
	}
	if filter.Type != "" {
		query += " AND type = ?"
		args = append(args, filter.Type)
	}
	if filter.SessionID != "" {
		query += " AND session_id = ?"
		args = append(args, filter.SessionID)
	}

	query += " ORDER BY timestamp DESC"

	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.SessionID, &e.Agent, &e.File, &e.Type, &e.Content, &e.Timestamp, &e.ExpiresAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// Prune deletes all entries whose expires_at has passed.
// Returns count of deleted entries.
func (s *Store) Prune() (int, error) {
	now := time.Now().UnixNano()
	res, err := s.db.Exec(
		`DELETE FROM memory WHERE expires_at > 0 AND expires_at < ?`,
		now,
	)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// PruneAsync starts a background goroutine that calls Prune every interval.
// Stops when ctx is done.
func (s *Store) PruneAsync(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.Prune() //nolint:errcheck
			}
		}
	}()
}

// SemanticRead performs a combined filter + TF-IDF ranking search.
// Returns topK most relevant entries matching the filter.
func (s *Store) SemanticRead(ctx context.Context, query string, filter Filter, topK int) ([]Entry, error) {
	// cap to avoid too-large corpus
	if filter.Limit == 0 || filter.Limit > 500 {
		filter.Limit = 500
	}
	entries, err := s.Read(filter)
	if err != nil {
		return nil, err
	}
	return TFIDFSearcher{}.Search(entries, query, topK), nil
}

// StartSession records a new agent session and returns its ID.
func (s *Store) StartSession(sessionID, agentName string) error {
	_, err := s.db.Exec(
		`INSERT INTO sessions (id, agent, started_at) VALUES (?, ?, ?)`,
		sessionID, agentName, time.Now().UnixNano(),
	)
	return err
}

// EndSession marks a session as ended.
func (s *Store) EndSession(sessionID string) error {
	_, err := s.db.Exec(
		`UPDATE sessions SET ended_at = ? WHERE id = ?`,
		time.Now().UnixNano(), sessionID,
	)
	return err
}

// WriteMetrics marshals m to JSON and stores it as a "metrics" type entry.
func (s *Store) WriteMetrics(sessionID, agentName string, m interface{}) error {
	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshalling metrics: %w", err)
	}
	return s.Write(Entry{
		SessionID: sessionID,
		Agent:     agentName,
		Type:      "metrics",
		Content:   string(data),
	})
}

// Close closes the underlying database.
func (s *Store) Close() error {
	return s.db.Close()
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
