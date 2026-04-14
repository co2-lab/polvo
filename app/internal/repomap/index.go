package repomap

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

const indexSchema = `
PRAGMA journal_mode=WAL;
PRAGMA synchronous=NORMAL;

CREATE TABLE IF NOT EXISTS code_chunks (
    id         TEXT    PRIMARY KEY,
    path       TEXT    NOT NULL,
    start_line INTEGER NOT NULL,
    end_line   INTEGER NOT NULL,
    symbol     TEXT,
    content    TEXT    NOT NULL,
    file_hash  TEXT    NOT NULL,
    indexed_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_code_chunks_path ON code_chunks(path);
CREATE INDEX IF NOT EXISTS idx_code_chunks_file_hash ON code_chunks(file_hash);

CREATE VIRTUAL TABLE IF NOT EXISTS code_fts USING fts5(
    chunk_id UNINDEXED,
    content,
    tokenize = 'porter unicode61'
);
`

// ChunkIndex stores chunks in SQLite and supports BM25 full-text search.
type ChunkIndex struct {
	db *sql.DB
}

// OpenChunkIndex opens or creates a SQLite DB at the given path.
func OpenChunkIndex(dbPath string) (*ChunkIndex, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening chunk index: %w", err)
	}

	// Single writer to avoid SQLITE_BUSY under WAL
	db.SetMaxOpenConns(1)

	if _, err := db.ExecContext(context.Background(), indexSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("applying chunk index schema: %w", err)
	}

	return &ChunkIndex{db: db}, nil
}

// Upsert inserts or replaces a chunk and updates the FTS index.
func (idx *ChunkIndex) Upsert(c Chunk) error {
	tx, err := idx.db.Begin()
	if err != nil {
		return fmt.Errorf("begin upsert tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := upsertInTx(tx, c); err != nil {
		return err
	}
	return tx.Commit()
}

// UpsertBatch upserts multiple chunks in a single transaction.
func (idx *ChunkIndex) UpsertBatch(chunks []Chunk) error {
	if len(chunks) == 0 {
		return nil
	}

	tx, err := idx.db.Begin()
	if err != nil {
		return fmt.Errorf("begin upsert batch tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	for _, c := range chunks {
		if err := upsertInTx(tx, c); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// upsertInTx performs an upsert for a single chunk within an existing transaction.
func upsertInTx(tx *sql.Tx, c Chunk) error {
	now := time.Now().UnixNano()

	// Insert or replace the chunk record
	_, err := tx.Exec(`
		INSERT INTO code_chunks (id, path, start_line, end_line, symbol, content, file_hash, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		    path       = excluded.path,
		    start_line = excluded.start_line,
		    end_line   = excluded.end_line,
		    symbol     = excluded.symbol,
		    content    = excluded.content,
		    file_hash  = excluded.file_hash,
		    indexed_at = excluded.indexed_at
	`, c.ID, c.Path, c.StartLine, c.EndLine, nullableStr(c.Symbol), c.Content, c.FileHash, now)
	if err != nil {
		return fmt.Errorf("upsert code_chunks: %w", err)
	}

	// Remove stale FTS entry (if any) then re-insert
	if _, err := tx.Exec(`DELETE FROM code_fts WHERE chunk_id = ?`, c.ID); err != nil {
		return fmt.Errorf("delete stale fts: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO code_fts (chunk_id, content) VALUES (?, ?)`, c.ID, c.Content); err != nil {
		return fmt.Errorf("insert fts: %w", err)
	}

	return nil
}

// DeleteByPath removes all chunks for a given file path.
func (idx *ChunkIndex) DeleteByPath(path string) error {
	tx, err := idx.db.Begin()
	if err != nil {
		return fmt.Errorf("begin delete tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Collect IDs to delete from FTS
	rows, err := tx.Query(`SELECT id FROM code_chunks WHERE path = ?`, path)
	if err != nil {
		return fmt.Errorf("selecting chunk ids: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, id := range ids {
		if _, err := tx.Exec(`DELETE FROM code_fts WHERE chunk_id = ?`, id); err != nil {
			return fmt.Errorf("delete fts for %s: %w", id, err)
		}
	}

	if _, err := tx.Exec(`DELETE FROM code_chunks WHERE path = ?`, path); err != nil {
		return fmt.Errorf("delete code_chunks: %w", err)
	}

	return tx.Commit()
}

// SearchBM25 performs full-text search and returns the top-k matching chunks.
func (idx *ChunkIndex) SearchBM25(query string, topK int) ([]Chunk, error) {
	if topK <= 0 {
		topK = 10
	}

	rows, err := idx.db.Query(`
		SELECT c.id, c.path, c.start_line, c.end_line,
		       COALESCE(c.symbol, ''), c.content, c.file_hash
		FROM code_fts f
		JOIN code_chunks c ON c.id = f.chunk_id
		WHERE code_fts MATCH ?
		ORDER BY bm25(code_fts)
		LIMIT ?
	`, query, topK)
	if err != nil {
		return nil, fmt.Errorf("bm25 search: %w", err)
	}
	defer rows.Close()

	return scanChunks(rows)
}

// GetByPath returns all chunks for the given file path.
func (idx *ChunkIndex) GetByPath(path string) ([]Chunk, error) {
	rows, err := idx.db.Query(`
		SELECT id, path, start_line, end_line,
		       COALESCE(symbol, ''), content, file_hash
		FROM code_chunks
		WHERE path = ?
		ORDER BY start_line
	`, path)
	if err != nil {
		return nil, fmt.Errorf("get by path: %w", err)
	}
	defer rows.Close()

	return scanChunks(rows)
}

// Close closes the underlying database connection.
func (idx *ChunkIndex) Close() error {
	return idx.db.Close()
}

// scanChunks reads rows from a query that returns (id, path, start_line, end_line, symbol, content, file_hash).
func scanChunks(rows *sql.Rows) ([]Chunk, error) {
	var chunks []Chunk
	for rows.Next() {
		var c Chunk
		if err := rows.Scan(&c.ID, &c.Path, &c.StartLine, &c.EndLine, &c.Symbol, &c.Content, &c.FileHash); err != nil {
			return nil, err
		}
		chunks = append(chunks, c)
	}
	return chunks, rows.Err()
}

// nullableStr converts an empty string to nil for SQL nullable TEXT columns.
func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
