// Package session manages work items (tasks and questions) for TUI sessions.
// Each work item gets a unique sequential ID (task#01, question#02, etc.),
// is persisted in SQLite, and can be referenced via @@task[id] syntax.
package session

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Kind is the type of work item.
type Kind string

const (
	KindTask     Kind = "task"
	KindQuestion Kind = "question"
)

// WorkItem represents a single task or question.
type WorkItem struct {
	ID        string    // e.g. "task#01", "question#02"
	Kind      Kind      // task | question
	Seq       int       // sequential number across all kinds
	Prompt    string    // original user prompt
	Summary   string    // async LLM-generated summary (may be empty while pending)
	StartedAt time.Time
	EndedAt   time.Time // zero if still active
}

// Manager persists and retrieves work items.
type Manager struct {
	db      *sql.DB
	mu      sync.Mutex
	nextSeq int // next seq to assign, loaded from DB on open
}

// Open opens (or creates) the work_items table in the given SQLite database.
// db must already be open (shared with memory.Store or standalone).
func Open(db *sql.DB) (*Manager, error) {
	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("session migrate: %w", err)
	}
	m := &Manager{db: db}
	if err := m.loadNextSeq(); err != nil {
		return nil, fmt.Errorf("session loadNextSeq: %w", err)
	}
	return m, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS work_items (
    id         TEXT    PRIMARY KEY,   -- "task#01"
    kind       TEXT    NOT NULL,      -- "task" | "question"
    seq        INTEGER NOT NULL,
    prompt     TEXT    NOT NULL,
    summary    TEXT    NOT NULL DEFAULT '',
    started_at INTEGER NOT NULL,
    ended_at   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_work_items_seq ON work_items(seq);
`)
	return err
}

func (m *Manager) loadNextSeq() error {
	row := m.db.QueryRow(`SELECT COALESCE(MAX(seq), 0) FROM work_items`)
	var maxSeq int
	if err := row.Scan(&maxSeq); err != nil {
		return err
	}
	m.nextSeq = maxSeq + 1
	return nil
}

// Start creates a new work item and returns it.
// Thread-safe; safe to call from goroutines.
func (m *Manager) Start(ctx context.Context, kind Kind, prompt string) (*WorkItem, error) {
	m.mu.Lock()
	seq := m.nextSeq
	m.nextSeq++
	m.mu.Unlock()

	id := fmt.Sprintf("%s#%02d", kind, seq)
	now := time.Now()

	_, err := m.db.ExecContext(ctx,
		`INSERT INTO work_items (id, kind, seq, prompt, started_at) VALUES (?, ?, ?, ?, ?)`,
		id, string(kind), seq, prompt, now.UnixNano(),
	)
	if err != nil {
		return nil, fmt.Errorf("inserting work item: %w", err)
	}

	return &WorkItem{
		ID:        id,
		Kind:      kind,
		Seq:       seq,
		Prompt:    prompt,
		StartedAt: now,
	}, nil
}

// Finish marks a work item as ended.
func (m *Manager) Finish(ctx context.Context, id string) error {
	_, err := m.db.ExecContext(ctx,
		`UPDATE work_items SET ended_at = ? WHERE id = ?`,
		time.Now().UnixNano(), id,
	)
	return err
}

// SetSummary persists an async-generated summary for the given work item.
func (m *Manager) SetSummary(ctx context.Context, id, summary string) error {
	_, err := m.db.ExecContext(ctx,
		`UPDATE work_items SET summary = ? WHERE id = ?`,
		summary, id,
	)
	return err
}

// Get returns the work item with the given ID, or (nil, nil) if not found.
func (m *Manager) Get(ctx context.Context, id string) (*WorkItem, error) {
	row := m.db.QueryRowContext(ctx,
		`SELECT id, kind, seq, prompt, summary, started_at, ended_at FROM work_items WHERE id = ?`,
		id,
	)
	return scanWorkItem(row)
}

// ListRecent returns the most recent n work items, newest first.
func (m *Manager) ListRecent(ctx context.Context, n int) ([]*WorkItem, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT id, kind, seq, prompt, summary, started_at, ended_at FROM work_items ORDER BY seq DESC LIMIT ?`,
		n,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*WorkItem
	for rows.Next() {
		wi, err := scanWorkItem(rows)
		if err != nil {
			return nil, err
		}
		if wi != nil {
			items = append(items, wi)
		}
	}
	return items, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanWorkItem(s scanner) (*WorkItem, error) {
	var wi WorkItem
	var kindStr string
	var startedNano, endedNano int64
	err := s.Scan(&wi.ID, &kindStr, &wi.Seq, &wi.Prompt, &wi.Summary, &startedNano, &endedNano)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	wi.Kind = Kind(kindStr)
	wi.StartedAt = time.Unix(0, startedNano)
	if endedNano > 0 {
		wi.EndedAt = time.Unix(0, endedNano)
	}
	return &wi, nil
}
