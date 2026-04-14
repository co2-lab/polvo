package filelock

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newRegistry returns a fully-initialized FileLockRegistry for testing.
func newRegistry() *FileLockRegistry {
	r := &FileLockRegistry{}
	for i := range r.buckets {
		r.buckets[i].locks = make(map[string]*sync.RWMutex)
	}
	return r
}

func TestMultipleConcurrentReaders(t *testing.T) {
	t.Parallel()
	r := &FileLockRegistry{}
	for i := range r.buckets {
		r.buckets[i].locks = make(map[string]*sync.RWMutex)
	}

	const path = "/tmp/test-file-readers.txt"
	const numReaders = 10

	var active int32
	var wg sync.WaitGroup
	errors := make(chan error, numReaders)

	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			unlock, err := r.LockRead(ctx, path)
			if err != nil {
				errors <- err
				return
			}
			atomic.AddInt32(&active, 1)
			// All readers should be able to hold the lock simultaneously
			time.Sleep(50 * time.Millisecond)
			atomic.AddInt32(&active, -1)
			unlock()
		}()
	}

	wg.Wait()
	close(errors)
	for err := range errors {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWriterBlocksReader(t *testing.T) {
	t.Parallel()
	r := &FileLockRegistry{}
	for i := range r.buckets {
		r.buckets[i].locks = make(map[string]*sync.RWMutex)
	}

	const path = "/tmp/test-file-writer-blocks.txt"

	// Acquire write lock
	writeCtx, writeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer writeCancel()
	writeUnlock, err := r.LockWrite(writeCtx, path)
	if err != nil {
		t.Fatalf("acquiring write lock: %v", err)
	}

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

	// Reader should not have acquired the lock yet
	select {
	case <-readAcquired:
		t.Error("reader acquired lock while writer holds it")
	case <-time.After(100 * time.Millisecond):
		// Expected: reader is blocked
	}

	// Release write lock
	writeUnlock()

	// Now reader should proceed
	select {
	case <-readAcquired:
		// Expected
	case <-time.After(2 * time.Second):
		t.Error("reader did not acquire lock after writer released it")
	}
}

func TestContextCancellation(t *testing.T) {
	t.Parallel()
	r := &FileLockRegistry{}
	for i := range r.buckets {
		r.buckets[i].locks = make(map[string]*sync.RWMutex)
	}

	const path = "/tmp/test-file-cancel.txt"

	// Acquire write lock to block the next attempt
	holdCtx, holdCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer holdCancel()
	holdUnlock, err := r.LockWrite(holdCtx, path)
	if err != nil {
		t.Fatalf("acquiring hold lock: %v", err)
	}
	defer holdUnlock()

	// Try to acquire write lock with a very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = r.LockWrite(ctx, path)
	if err == nil {
		t.Error("expected context cancellation error, got nil")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("expected DeadlineExceeded, got: %v", err)
	}
}

func TestLockMultipleNoDeadlock(t *testing.T) {
	t.Parallel()
	r := &FileLockRegistry{}
	for i := range r.buckets {
		r.buckets[i].locks = make(map[string]*sync.RWMutex)
	}

	// Paths in reverse alphabetical order — LockMultiple should sort them
	paths := []string{"/tmp/zzz.txt", "/tmp/mmm.txt", "/tmp/aaa.txt"}
	reversePaths := []string{"/tmp/aaa.txt", "/tmp/mmm.txt", "/tmp/zzz.txt"}

	var wg sync.WaitGroup
	errors := make(chan error, 2)

	for _, pp := range [][]string{paths, reversePaths} {
		pp := pp
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			unlock, err := r.LockMultiple(ctx, pp, true)
			if err != nil {
				errors <- err
				return
			}
			time.Sleep(10 * time.Millisecond)
			unlock()
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// No deadlock
	case <-time.After(10 * time.Second):
		t.Error("deadlock detected: LockMultiple did not complete within timeout")
	}

	close(errors)
	for err := range errors {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// New gap-filling tests
// ---------------------------------------------------------------------------

// TestRegistry_WriterBlocksWriter verifies that two concurrent writers on the
// same path achieve mutual exclusion: the second writer must wait until the
// first releases.
func TestRegistry_WriterBlocksWriter(t *testing.T) {
	t.Parallel()
	r := newRegistry()

	const path = "/tmp/test-writer-blocks-writer.txt"

	// Acquire the first write lock.
	ctx1, cancel1 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel1()
	unlock1, err := r.LockWrite(ctx1, path)
	if err != nil {
		t.Fatalf("acquiring first write lock: %v", err)
	}

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

	// Second writer must not acquire while the first holds the lock.
	select {
	case <-secondAcquired:
		t.Error("second writer acquired lock while first writer holds it — mutual exclusion violated")
	case <-time.After(100 * time.Millisecond):
		// Expected: second writer is blocked.
	}

	// Release first lock; second writer should now proceed.
	unlock1()

	select {
	case <-secondAcquired:
		// Expected: second writer acquired after first released.
	case <-time.After(2 * time.Second):
		t.Error("second writer did not acquire lock after first writer released it")
	}
}

// TestRegistry_UnlockTwice verifies that calling the unlock function returned
// by LockWrite twice is safe (the second call is a no-op, not a fatal crash).
func TestRegistry_UnlockTwice(t *testing.T) {
	t.Parallel()
	r := newRegistry()

	const path = "/tmp/test-unlock-twice-safe.txt"
	ctx := context.Background()

	unlock, err := r.LockWrite(ctx, path)
	if err != nil {
		t.Fatalf("acquiring write lock: %v", err)
	}

	panicked := false
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				panicked = true
			}
		}()
		unlock() // first call — must release the lock
		unlock() // second call — must be a no-op, not a panic
	}()

	if panicked {
		t.Error("calling unlock twice caused a panic; expected idempotent (no-op) second call")
	}
}

// TestRegistry_LockMultipleWithReadLocks verifies that LockMultiple with
// write=false acquires shared read locks, allowing multiple concurrent readers.
func TestRegistry_LockMultipleWithReadLocks(t *testing.T) {
	t.Parallel()
	r := newRegistry()

	paths := []string{"/tmp/multi-read-a.txt", "/tmp/multi-read-b.txt"}

	var wg sync.WaitGroup
	errs := make(chan error, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel2()
			unlock, err := r.LockMultiple(ctx2, paths, false)
			if err != nil {
				errs <- err
				return
			}
			time.Sleep(10 * time.Millisecond)
			unlock()
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// No deadlock; concurrent read locks succeeded.
	case <-time.After(5 * time.Second):
		t.Error("LockMultiple with write=false deadlocked or timed out")
	}

	close(errs)
	for err := range errs {
		t.Errorf("unexpected error: %v", err)
	}
}
