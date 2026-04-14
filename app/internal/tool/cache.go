package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// ToolCache caches deterministic tool results keyed by tool+args+file-mtime.
// Cacheable: read, glob, grep, ls (deterministic given same inputs + filesystem state).
// Not cacheable: bash, write, edit, patch, web_fetch, web_search.
type ToolCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	maxSize int
	ttl     time.Duration
}

type cacheEntry struct {
	result   *Result
	path     string // primary path this entry is associated with (for invalidation)
	cachedAt time.Time
	ttl      time.Duration
}

// NewToolCache creates a cache with given max entries and TTL.
func NewToolCache(maxSize int, ttl time.Duration) *ToolCache {
	return &ToolCache{
		entries: make(map[string]*cacheEntry, maxSize),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

// CacheKey builds a lookup key from tool name, raw args JSON, and file mtime.
// path may be empty for tools without a primary file (e.g. glob).
func CacheKey(toolName string, args json.RawMessage, path string) string {
	var mtime int64
	if path != "" {
		if fi, err := os.Stat(path); err == nil {
			mtime = fi.ModTime().UnixNano()
		}
	}
	h := sha256.New()
	h.Write([]byte(toolName))
	h.Write(args)
	h.Write([]byte(fmt.Sprintf("%d", mtime)))
	return hex.EncodeToString(h.Sum(nil))
}

// Get returns a cached result if it exists and hasn't expired.
func (c *ToolCache) Get(key string) (*Result, bool) {
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Since(e.cachedAt) > e.ttl {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return nil, false
	}
	return e.result, true
}

// Set stores a result. If cache is full, evicts a random entry (simple strategy).
func (c *ToolCache) Set(key, path string, result *Result) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.maxSize {
		// simple eviction: delete first entry found
		for k := range c.entries {
			delete(c.entries, k)
			break
		}
	}
	c.entries[key] = &cacheEntry{
		result:   result,
		path:     path,
		cachedAt: time.Now(),
		ttl:      c.ttl,
	}
}

// Invalidate removes all cached entries whose path equals the given file path.
// Call this after any write/edit to a file.
func (c *ToolCache) Invalidate(filePath string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, e := range c.entries {
		if e.path == filePath {
			delete(c.entries, k)
		}
	}
}

// InvalidateAll clears all cache entries.
func (c *ToolCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*cacheEntry, c.maxSize)
}
