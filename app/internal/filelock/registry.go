// Package filelock provides file-level read/write locking for concurrent agent access.
package filelock

import (
	"context"
	"hash/fnv"
	"path/filepath"
	"sort"
	"sync"
)

// Registry is the interface for file-level read/write locking.
// Implementations may be in-process only (FileLockRegistry) or cross-process
// (FlockRegistry, using OS-level flock(2)).
type Registry interface {
	// LockRead acquires a shared read lock for path.
	// Returns an unlock func suitable for use with defer.
	LockRead(ctx context.Context, path string) (func(), error)

	// LockWrite acquires an exclusive write lock for path.
	// Returns an unlock func suitable for use with defer.
	LockWrite(ctx context.Context, path string) (func(), error)
}

// NewGlobalRegistry returns a Registry implementation.
// If crossProcess is true and the OS supports flock(2) (darwin/linux), a
// FlockRegistry is returned; otherwise the in-process FileLockRegistry is used.
func NewGlobalRegistry(crossProcess bool) Registry {
	return newGlobalRegistry(crossProcess)
}

// FileLockRegistry is a strip-lock registry using 256 buckets.
// Each path hashes to a bucket index, avoiding single global mutex contention.
type FileLockRegistry struct {
	buckets [256]bucket
}

type bucket struct {
	mu    sync.Mutex
	locks map[string]*sync.RWMutex
}

// Global is the singleton registry used by all tools.
var Global = &FileLockRegistry{}

func init() {
	for i := range Global.buckets {
		Global.buckets[i].locks = make(map[string]*sync.RWMutex)
	}
}

// normalizePath returns a cleaned absolute-like path.
// For paths that are already absolute, this just cleans them.
// For relative paths, it cleans without making them absolute
// (callers should pass absolute paths).
func normalizePath(path string) string {
	return filepath.Clean(path)
}

// hashBucket returns the bucket index for a path using FNV-1a hash.
func hashBucket(path string) uint8 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(path))
	return uint8(h.Sum32() % 256)
}

// getMutex retrieves or creates the RWMutex for the given path within its bucket.
func (r *FileLockRegistry) getMutex(path string) *sync.RWMutex {
	idx := hashBucket(path)
	b := &r.buckets[idx]
	b.mu.Lock()
	defer b.mu.Unlock()
	l, ok := b.locks[path]
	if !ok {
		l = &sync.RWMutex{}
		b.locks[path] = l
	}
	return l
}

// LockWrite acquires an exclusive write lock for path.
// It respects context cancellation/timeout.
// Returns an unlock func suitable for use with defer.
func (r *FileLockRegistry) LockWrite(ctx context.Context, path string) (func(), error) {
	path = normalizePath(path)
	l := r.getMutex(path)

	done := make(chan struct{})
	go func() {
		l.Lock()
		close(done)
	}()

	select {
	case <-done:
		var once sync.Once
		return func() { once.Do(l.Unlock) }, nil
	case <-ctx.Done():
		// The goroutine will eventually acquire and hold the lock — we must
		// release it to avoid permanently leaking it.
		go func() {
			<-done
			l.Unlock()
		}()
		return nil, ctx.Err()
	}
}

// LockRead acquires a shared read lock for path.
// Multiple concurrent readers are allowed.
// Returns an unlock func suitable for use with defer.
func (r *FileLockRegistry) LockRead(ctx context.Context, path string) (func(), error) {
	path = normalizePath(path)
	l := r.getMutex(path)

	done := make(chan struct{})
	go func() {
		l.RLock()
		close(done)
	}()

	select {
	case <-done:
		var once sync.Once
		return func() { once.Do(l.RUnlock) }, nil
	case <-ctx.Done():
		go func() {
			<-done
			l.RUnlock()
		}()
		return nil, ctx.Err()
	}
}

// LockMultiple acquires locks on multiple paths in lexicographic order,
// preventing deadlocks from lock-order inversions.
// write=true acquires exclusive write locks; write=false acquires shared read locks.
// Returns a single unlock func that releases all locks in reverse order.
func (r *FileLockRegistry) LockMultiple(ctx context.Context, paths []string, write bool) (func(), error) {
	// Normalize and deduplicate
	seen := make(map[string]bool, len(paths))
	normalized := make([]string, 0, len(paths))
	for _, p := range paths {
		n := normalizePath(p)
		if !seen[n] {
			seen[n] = true
			normalized = append(normalized, n)
		}
	}

	// Sort lexicographically to prevent deadlocks
	sort.Strings(normalized)

	unlocks := make([]func(), 0, len(normalized))

	for _, p := range normalized {
		var unlock func()
		var err error
		if write {
			unlock, err = r.LockWrite(ctx, p)
		} else {
			unlock, err = r.LockRead(ctx, p)
		}
		if err != nil {
			// Release already-acquired locks in reverse order
			for i := len(unlocks) - 1; i >= 0; i-- {
				unlocks[i]()
			}
			return nil, err
		}
		unlocks = append(unlocks, unlock)
	}

	return func() {
		for i := len(unlocks) - 1; i >= 0; i-- {
			unlocks[i]()
		}
	}, nil
}
