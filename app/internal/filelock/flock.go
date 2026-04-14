//go:build darwin || linux

package filelock

import (
	"context"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"
)

// FlockRegistry provides cross-process file locking using flock(2).
// Combines in-process RWMutex (for goroutine safety within a single process)
// with OS-level flock (for cross-process safety).
type FlockRegistry struct {
	mu    sync.Mutex
	locks map[string]*flockEntry
}

type flockEntry struct {
	inproc sync.RWMutex // goroutine-level lock
	fd     *os.File     // lock file descriptor
}

// NewFlockRegistry creates a new FlockRegistry.
func NewFlockRegistry() *FlockRegistry {
	return &FlockRegistry{
		locks: make(map[string]*flockEntry),
	}
}

// lockFilePath returns the path of the sidecar lock file for a given resource path.
func lockFilePath(path string) string {
	return path + ".polvo.lock"
}

// getEntry retrieves or creates the flockEntry for the given (normalized) path.
// The lock file is opened/created on first use.
func (r *FlockRegistry) getEntry(path string) (*flockEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if e, ok := r.locks[path]; ok {
		return e, nil
	}

	lockPath := lockFilePath(path)
	fd, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("opening lock file %s: %w", lockPath, err)
	}

	e := &flockEntry{fd: fd}
	r.locks[path] = e
	return e, nil
}

// flockWithContext attempts a non-blocking flock in a tight retry loop,
// yielding to ctx.Done() between attempts.
func flockWithContext(ctx context.Context, fd *os.File, how int) error {
	for {
		err := syscall.Flock(int(fd.Fd()), how|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if err != syscall.EWOULDBLOCK {
			return fmt.Errorf("flock: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// LockRead acquires a shared (read) lock for path.
// It first acquires the in-process RLock, then applies an OS-level LOCK_SH.
// Returns an unlock func that releases both layers.
func (r *FlockRegistry) LockRead(ctx context.Context, path string) (func(), error) {
	path = normalizePath(path)

	entry, err := r.getEntry(path)
	if err != nil {
		return nil, err
	}

	// Acquire in-process shared lock (respects context via goroutine + select).
	inprocDone := make(chan struct{})
	go func() {
		entry.inproc.RLock()
		close(inprocDone)
	}()
	select {
	case <-inprocDone:
	case <-ctx.Done():
		go func() { <-inprocDone; entry.inproc.RUnlock() }()
		return nil, ctx.Err()
	}

	// Acquire OS-level shared lock.
	if err := flockWithContext(ctx, entry.fd, syscall.LOCK_SH); err != nil {
		entry.inproc.RUnlock()
		return nil, err
	}

	var once sync.Once
	unlock := func() {
		once.Do(func() {
			_ = syscall.Flock(int(entry.fd.Fd()), syscall.LOCK_UN)
			entry.inproc.RUnlock()
		})
	}
	return unlock, nil
}

// LockWrite acquires an exclusive (write) lock for path.
// It first acquires the in-process Lock, then applies an OS-level LOCK_EX.
// Returns an unlock func that releases both layers.
func (r *FlockRegistry) LockWrite(ctx context.Context, path string) (func(), error) {
	path = normalizePath(path)

	entry, err := r.getEntry(path)
	if err != nil {
		return nil, err
	}

	// Acquire in-process exclusive lock.
	inprocDone := make(chan struct{})
	go func() {
		entry.inproc.Lock()
		close(inprocDone)
	}()
	select {
	case <-inprocDone:
	case <-ctx.Done():
		go func() { <-inprocDone; entry.inproc.Unlock() }()
		return nil, ctx.Err()
	}

	// Acquire OS-level exclusive lock.
	if err := flockWithContext(ctx, entry.fd, syscall.LOCK_EX); err != nil {
		entry.inproc.Unlock()
		return nil, err
	}

	var once sync.Once
	unlock := func() {
		once.Do(func() {
			_ = syscall.Flock(int(entry.fd.Fd()), syscall.LOCK_UN)
			entry.inproc.Unlock()
		})
	}
	return unlock, nil
}

// newGlobalRegistry is the platform-specific constructor called by NewGlobalRegistry.
func newGlobalRegistry(crossProcess bool) Registry {
	if crossProcess {
		return NewFlockRegistry()
	}
	r := &FileLockRegistry{}
	for i := range r.buckets {
		r.buckets[i].locks = make(map[string]*sync.RWMutex)
	}
	return r
}
