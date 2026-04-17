package repomap

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/gob"
	"fmt"
	"time"
)

const symCacheSchema = `
CREATE TABLE IF NOT EXISTS repomap_cache (
    path      TEXT    PRIMARY KEY,
    mtime     INTEGER NOT NULL,
    symbols   BLOB    NOT NULL,
    rank      REAL    NOT NULL DEFAULT 0.0,
    cached_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_repomap_cache_path ON repomap_cache(path);
`

// SymCache caches extracted RichSymbols keyed by (path, mtime) in SQLite.
// It reuses the same *sql.DB as ChunkIndex.
type SymCache struct {
	db *sql.DB
}

// NewSymCache creates the repomap_cache table if needed and returns a SymCache.
func NewSymCache(db *sql.DB) (*SymCache, error) {
	if _, err := db.ExecContext(context.Background(), symCacheSchema); err != nil {
		return nil, fmt.Errorf("symcache: create table: %w", err)
	}
	return &SymCache{db: db}, nil
}

// Get returns the cached symbols for path at the given mtime.
// Returns (nil, nil) on cache miss (mtime changed or path unknown).
func (c *SymCache) Get(path string, mtime int64) ([]RichSymbol, error) {
	var blob []byte
	err := c.db.QueryRowContext(context.Background(),
		`SELECT symbols FROM repomap_cache WHERE path = ? AND mtime = ?`,
		path, mtime,
	).Scan(&blob)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("symcache get: %w", err)
	}
	return decodeSymbols(blob)
}

// Put stores (or replaces) symbols for path at the given mtime.
func (c *SymCache) Put(path string, mtime int64, syms []RichSymbol) error {
	blob, err := encodeSymbols(syms)
	if err != nil {
		return fmt.Errorf("symcache put encode: %w", err)
	}
	_, err = c.db.ExecContext(context.Background(),
		`INSERT OR REPLACE INTO repomap_cache (path, mtime, symbols, rank, cached_at)
		 VALUES (?, ?, ?, 0.0, ?)`,
		path, mtime, blob, time.Now().UnixNano(),
	)
	return err
}

// SetRank updates the rank score for a cached entry.
func (c *SymCache) SetRank(path string, rank float64) error {
	_, err := c.db.ExecContext(context.Background(),
		`UPDATE repomap_cache SET rank = ? WHERE path = ?`,
		rank, path,
	)
	return err
}

// Prune removes entries older than 7 days.
func (c *SymCache) Prune(_ context.Context) error {
	cutoff := time.Now().Add(-7 * 24 * time.Hour).UnixNano()
	_, err := c.db.ExecContext(context.Background(),
		`DELETE FROM repomap_cache WHERE cached_at < ?`, cutoff,
	)
	return err
}

func encodeSymbols(syms []RichSymbol) ([]byte, error) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(syms); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeSymbols(data []byte) ([]RichSymbol, error) {
	var syms []RichSymbol
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&syms); err != nil {
		return nil, err
	}
	return syms, nil
}
