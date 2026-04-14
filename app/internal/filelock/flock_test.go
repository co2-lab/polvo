//go:build darwin || linux

package filelock

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newFlockRegistry creates a fresh FlockRegistry for testing.
// Each test uses its own instance so lock state does not leak between tests.
func newFlockRegistry(t *testing.T) *FlockRegistry {
	t.Helper()
	return NewFlockRegistry()
}

// uniquePath returns a test-specific temp path that differs between tests to
// avoid cross-test lock file collisions when running in parallel.
func uniquePath(t *testing.T, suffix string) string {
	t.Helper()
	return t.TempDir() + "/test" + suffix + ".dat"
}

func TestFlockRegistry_ExclusiveLock(t *testing.T) {
	t.Parallel()
	r := newFlockRegistry(t)
	path := uniquePath(t, "-exclusive")

	// Acquire the first write lock.
	ctx1, cancel1 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel1()

	unlock1, err := r.LockWrite(ctx1, path)
	if err != nil {
		t.Fatalf("first LockWrite: %v", err)
	}

	// A second goroutine tries to acquire the same write lock.
	secondAcquired := make(chan struct{})
	go func() {
		ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel2()
		unlock2, err := r.LockWrite(ctx2, path)
		if err != nil {
			return
		}
		close(secondAcquired)
		unlock2()
	}()

	// Second writer must not acquire while the first holds it.
	select {
	case <-secondAcquired:
		t.Error("second writer acquired exclusive lock while first still holds it")
	case <-time.After(100 * time.Millisecond):
		// Expected: blocked.
	}

	// Release first lock; second writer should now proceed.
	unlock1()

	select {
	case <-secondAcquired:
		// Expected.
	case <-time.After(3 * time.Second):
		t.Error("second writer did not acquire lock after first released it")
	}
}

func TestFlockRegistry_SharedLocks(t *testing.T) {
	t.Parallel()
	r := newFlockRegistry(t)
	path := uniquePath(t, "-shared")

	const numReaders = 5

	var active int32
	var maxObserved int32
	var wg sync.WaitGroup
	errs := make(chan error, numReaders)

	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			unlock, err := r.LockRead(ctx, path)
			if err != nil {
				errs <- err
				return
			}
			// Track concurrent active readers.
			n := atomic.AddInt32(&active, 1)
			for {
				cur := atomic.LoadInt32(&maxObserved)
				if n <= cur || atomic.CompareAndSwapInt32(&maxObserved, cur, n) {
					break
				}
			}
			time.Sleep(30 * time.Millisecond)
			atomic.AddInt32(&active, -1)
			unlock()
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("unexpected error: %v", err)
	}

	// All readers should have run concurrently (max observed > 1 for numReaders >= 2).
	if numReaders >= 2 && atomic.LoadInt32(&maxObserved) < 2 {
		t.Errorf("expected concurrent readers (max observed %d), but they appear to have been serialized", atomic.LoadInt32(&maxObserved))
	}
}

func TestFlockRegistry_WriterBlocksReaders(t *testing.T) {
	t.Parallel()
	r := newFlockRegistry(t)
	path := uniquePath(t, "-writer-blocks-readers")

	// Acquire an exclusive write lock first.
	writeCtx, writeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer writeCancel()

	writeUnlock, err := r.LockWrite(writeCtx, path)
	if err != nil {
		t.Fatalf("acquiring write lock: %v", err)
	}

	// A reader attempts to acquire while the write lock is held.
	readAcquired := make(chan struct{})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		unlock, err := r.LockRead(ctx, path)
		if err != nil {
			return
		}
		close(readAcquired)
		unlock()
	}()

	// Reader must be blocked.
	select {
	case <-readAcquired:
		t.Error("reader acquired lock while exclusive writer holds it")
	case <-time.After(100 * time.Millisecond):
		// Expected.
	}

	// Release write lock; reader should now proceed.
	writeUnlock()

	select {
	case <-readAcquired:
		// Expected.
	case <-time.After(3 * time.Second):
		t.Error("reader did not acquire lock after writer released it")
	}
}

func TestFlockRegistry_ContextCancellation(t *testing.T) {
	t.Parallel()
	r := newFlockRegistry(t)
	path := uniquePath(t, "-ctx-cancel")

	// Hold an exclusive write lock to block the next attempt.
	holdCtx, holdCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer holdCancel()

	holdUnlock, err := r.LockWrite(holdCtx, path)
	if err != nil {
		t.Fatalf("acquiring hold lock: %v", err)
	}
	defer holdUnlock()

	// Attempt to acquire write lock with a very short timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = r.LockWrite(ctx, path)
	if err == nil {
		t.Fatal("expected an error due to context cancellation, got nil")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got: %v", err)
	}
}
