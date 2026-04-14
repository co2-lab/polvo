package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestToolCache_HitAndMiss(t *testing.T) {
	cache := NewToolCache(10, time.Minute)
	result := &Result{Content: "hello"}

	key := "testkey"
	_, ok := cache.Get(key)
	if ok {
		t.Fatal("expected cache miss on empty cache")
	}

	cache.Set(key, "/some/path", result)
	got, ok := cache.Get(key)
	if !ok {
		t.Fatal("expected cache hit after Set")
	}
	if got.Content != result.Content {
		t.Errorf("cache returned wrong content: got %q, want %q", got.Content, result.Content)
	}
}

func TestToolCache_TTLExpiry(t *testing.T) {
	ttl := 50 * time.Millisecond
	cache := NewToolCache(10, ttl)
	key := "expiring"
	cache.Set(key, "", &Result{Content: "will expire"})

	// Should hit immediately.
	if _, ok := cache.Get(key); !ok {
		t.Fatal("expected cache hit before TTL expiry")
	}

	// Wait for TTL to expire.
	time.Sleep(ttl + 10*time.Millisecond)

	if _, ok := cache.Get(key); ok {
		t.Fatal("expected cache miss after TTL expiry")
	}
}

func TestToolCache_Invalidate(t *testing.T) {
	cache := NewToolCache(10, time.Minute)
	path := "/project/file.go"

	cache.Set("key1", path, &Result{Content: "v1"})
	cache.Set("key2", path, &Result{Content: "v2"})
	cache.Set("key3", "/other/file.go", &Result{Content: "other"})

	cache.Invalidate(path)

	if _, ok := cache.Get("key1"); ok {
		t.Error("key1 should have been invalidated")
	}
	if _, ok := cache.Get("key2"); ok {
		t.Error("key2 should have been invalidated")
	}
	if _, ok := cache.Get("key3"); !ok {
		t.Error("key3 should still be in cache (different path)")
	}
}

func TestToolCache_MaxSizeEviction(t *testing.T) {
	maxSize := 3
	cache := NewToolCache(maxSize, time.Minute)

	for i := 0; i < maxSize+2; i++ {
		key := string(rune('a' + i))
		cache.Set(key, "", &Result{Content: key})
	}

	cache.mu.RLock()
	size := len(cache.entries)
	cache.mu.RUnlock()

	// After inserting maxSize+2 items, the cache should never grow beyond maxSize.
	if size > maxSize {
		t.Errorf("cache size %d exceeds maxSize %d", size, maxSize)
	}
}

func TestCacheKey_ChangesOnMtime(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.go")

	// Write initial content.
	if err := os.WriteFile(path, []byte("v1"), 0644); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	args := json.RawMessage(`{"path":"file.go"}`)
	key1 := CacheKey("read", args, path)

	// Ensure mtime changes by sleeping briefly and rewriting.
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(path, []byte("v2"), 0644); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	key2 := CacheKey("read", args, path)

	if key1 == key2 {
		t.Error("cache key should differ when file mtime changes")
	}
}

func TestToolCache_InvalidateAll(t *testing.T) {
	cache := NewToolCache(10, time.Minute)
	cache.Set("k1", "/a", &Result{Content: "1"})
	cache.Set("k2", "/b", &Result{Content: "2"})

	cache.InvalidateAll()

	if _, ok := cache.Get("k1"); ok {
		t.Error("k1 should be gone after InvalidateAll")
	}
	if _, ok := cache.Get("k2"); ok {
		t.Error("k2 should be gone after InvalidateAll")
	}
}

// Integration test: read tool with cache — verifies hit, miss, and write invalidation.
func TestReadToolCache_Integration(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(filePath, []byte("original\n"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	cache := NewToolCache(100, time.Minute)
	readTool := NewReadWithCache(dir, nil, cache)
	writeTool := NewWriteWithCache(dir, nil, cache)

	ctx := context.Background()

	// First read — should miss cache and populate it.
	input := mustJSON(t, map[string]any{"path": "data.txt"})
	res1, err := readTool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("first read: %v", err)
	}
	if res1.IsError {
		t.Fatalf("first read error: %s", res1.Content)
	}
	if res1.Content == "" {
		t.Fatal("first read returned empty content")
	}

	// Verify cache was populated by building the key.
	absPath := filepath.Join(dir, "data.txt")
	key := CacheKey("read", input, absPath)
	if _, ok := cache.Get(key); !ok {
		t.Error("cache should be populated after first read")
	}

	// Second read — should return from cache (same result pointer).
	res2, err := readTool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	if res2 != res1 {
		t.Error("second read should return the exact same *Result pointer (cache hit)")
	}

	// Write new content — should invalidate cache.
	writeInput := mustJSON(t, map[string]any{
		"path":          "data.txt",
		"content":       "updated\n",
		"security_risk": "low",
	})
	wres, err := writeTool.Execute(ctx, writeInput)
	if err != nil || wres.IsError {
		t.Fatalf("write failed: %v / %s", err, wres.Content)
	}

	// After write, old cache key should be gone (file mtime changed).
	// The key was computed before the write, so it will no longer be present.
	if _, ok := cache.Get(key); ok {
		t.Error("cache should be invalidated after write")
	}

	// Third read — fresh content after cache invalidation.
	res3, err := readTool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("third read: %v", err)
	}
	if res3.IsError {
		t.Fatalf("third read error: %s", res3.Content)
	}
	// The key changed because mtime changed, so res3 should reflect updated content.
	if res3.Content == res1.Content {
		// mtime-based key: the old key no longer applies. This is acceptable as long
		// as the read returns fresh content (not from the stale cache entry).
		// If content is same as res1 it means the file wasn't changed — which shouldn't
		// happen given we wrote "updated\n" above. Flag as error only if content is stale.
		t.Error("third read returned same content as first read after file was updated")
	}
}
