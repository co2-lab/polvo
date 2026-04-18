package repomap

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func openTestSymCache(t *testing.T) *SymCache {
	t.Helper()
	c, err := NewSymCache(openTestDB(t))
	if err != nil {
		t.Fatalf("NewSymCache: %v", err)
	}
	return c
}

func TestSymCache_HitOnSameMtime(t *testing.T) {
	c := openTestSymCache(t)
	syms := []RichSymbol{
		{Name: "Foo", Kind: "function", IsDef: true, Line: 1, FilePath: "foo.go"},
		{Name: "Bar", Kind: "method", IsDef: true, Line: 5, FilePath: "foo.go"},
	}
	if err := c.Put("foo.go", 12345, syms); err != nil {
		t.Fatal(err)
	}
	got, err := c.Get("foo.go", 12345)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(syms) {
		t.Fatalf("expected %d symbols, got %d", len(syms), len(got))
	}
	if got[0].Name != "Foo" || got[1].Name != "Bar" {
		t.Errorf("unexpected symbols: %v", got)
	}
}

func TestSymCache_MissOnChangedMtime(t *testing.T) {
	c := openTestSymCache(t)
	_ = c.Put("foo.go", 1, []RichSymbol{{Name: "Old", IsDef: true}})

	got, err := c.Get("foo.go", 2) // different mtime
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Error("expected nil on mtime miss")
	}
}

func TestSymCache_MissOnUnknownPath(t *testing.T) {
	c := openTestSymCache(t)
	got, err := c.Get("nonexistent.go", 999)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Error("expected nil for unknown path")
	}
}

func TestSymCache_PutIdempotent(t *testing.T) {
	c := openTestSymCache(t)
	syms1 := []RichSymbol{{Name: "First", IsDef: true}}
	syms2 := []RichSymbol{{Name: "Second", IsDef: true}}

	_ = c.Put("foo.go", 1, syms1)
	_ = c.Put("foo.go", 1, syms2) // same path+mtime, replace

	got, _ := c.Get("foo.go", 1)
	if len(got) != 1 || got[0].Name != "Second" {
		t.Errorf("expected Second after second Put, got %v", got)
	}
}

func TestSymCache_SetRank(t *testing.T) {
	c := openTestSymCache(t)
	_ = c.Put("foo.go", 1, []RichSymbol{{Name: "X", IsDef: true}})
	if err := c.SetRank("foo.go", 0.75); err != nil {
		t.Fatal(err)
	}

	var rank float64
	err := c.db.QueryRowContext(context.Background(),
		`SELECT rank FROM repomap_cache WHERE path = ?`, "foo.go",
	).Scan(&rank)
	if err != nil {
		t.Fatal(err)
	}
	if rank != 0.75 {
		t.Errorf("rank = %v, want 0.75", rank)
	}
}

func TestSymCache_Prune(t *testing.T) {
	c := openTestSymCache(t)

	// Insert old entry (8 days ago).
	oldTime := time.Now().Add(-8 * 24 * time.Hour).UnixNano()
	_, err := c.db.ExecContext(context.Background(),
		`INSERT INTO repomap_cache (path, mtime, symbols, rank, cached_at) VALUES (?, ?, ?, 0.0, ?)`,
		"old.go", 1, []byte{0x07, 0x01, 0x00, 0x00}, oldTime,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Insert fresh entry.
	_ = c.Put("fresh.go", 2, []RichSymbol{{Name: "Fresh", IsDef: true}})

	if err := c.Prune(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Old entry should be gone.
	got, _ := c.Get("old.go", 1)
	if got != nil {
		t.Error("old entry should have been pruned")
	}

	// Fresh entry should survive (but Get mtime must match).
	var count int
	_ = c.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM repomap_cache WHERE path = 'fresh.go'`,
	).Scan(&count)
	if count != 1 {
		t.Error("fresh entry should survive prune")
	}
}

func TestSymCache_GobRoundTrip(t *testing.T) {
	syms := []RichSymbol{
		{Name: "Alpha", Kind: "function", IsDef: true, Line: 10, FilePath: "a.go"},
		{Name: "beta", Kind: "ref", IsDef: false, Line: 15, FilePath: "a.go"},
	}
	encoded, err := encodeSymbols(syms)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeSymbols(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 2 {
		t.Fatalf("expected 2 symbols, got %d", len(decoded))
	}
	if decoded[0].Name != "Alpha" || decoded[0].Line != 10 {
		t.Errorf("round-trip mismatch: %+v", decoded[0])
	}
	if decoded[1].IsDef {
		t.Error("second symbol should be IsDef=false")
	}
}
