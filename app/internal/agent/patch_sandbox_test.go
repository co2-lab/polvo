package agent_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/co2-lab/polvo/internal/agent"
)

func TestPatchSandbox_Record(t *testing.T) {
	s := &agent.PatchSandbox{}

	if err := s.Record("/tmp/test_record.txt", []byte("hello")); err != nil {
		t.Fatalf("Record failed: %v", err)
	}
	if s.Len() != 1 {
		t.Errorf("expected Len()=1, got %d", s.Len())
	}
}

func TestPatchSandbox_Apply(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.txt")

	s := &agent.PatchSandbox{}
	if err := s.Record(path, []byte("written by sandbox")); err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	if err := s.Apply(); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Verify file was written.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile after Apply failed: %v", err)
	}
	if string(data) != "written by sandbox" {
		t.Errorf("unexpected file content: %q", string(data))
	}

	// After Apply, pending list is cleared.
	if s.Len() != 0 {
		t.Errorf("expected Len()=0 after Apply, got %d", s.Len())
	}
}

func TestPatchSandbox_Discard(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "never_written.txt")

	s := &agent.PatchSandbox{}
	if err := s.Record(path, []byte("should not be written")); err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	s.Discard()

	if s.Len() != 0 {
		t.Errorf("expected Len()=0 after Discard, got %d", s.Len())
	}

	// File must not exist.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected file to not exist after Discard, but stat returned: %v", err)
	}
}

func TestPatchSandbox_Preview(t *testing.T) {
	dir := t.TempDir()

	// Create an existing file with known content.
	existing := filepath.Join(dir, "existing.txt")
	if err := os.WriteFile(existing, []byte("old content\n"), 0644); err != nil {
		t.Fatalf("writing existing file: %v", err)
	}

	// Also record a new file (no original).
	newFile := filepath.Join(dir, "new.txt")

	s := &agent.PatchSandbox{}
	_ = s.Record(existing, []byte("new content\n"))
	_ = s.Record(newFile, []byte("brand new\n"))

	preview := s.Preview()
	if preview == "" {
		t.Error("expected non-empty Preview()")
	}
	// Should contain file paths.
	if !strings.Contains(preview, "existing.txt") {
		t.Errorf("expected 'existing.txt' in preview, got:\n%s", preview)
	}
	if !strings.Contains(preview, "new.txt") {
		t.Errorf("expected 'new.txt' in preview, got:\n%s", preview)
	}
}

func TestPatchSandbox_ConcurrentSafe(t *testing.T) {
	dir := t.TempDir()
	s := &agent.PatchSandbox{}

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			path := filepath.Join(dir, "file.txt") // same path is fine for this test
			_ = s.Record(path, []byte("data"))
		}(i)
	}
	wg.Wait()

	// Len should reflect all goroutine writes without data races.
	if s.Len() != goroutines {
		t.Errorf("expected Len()=%d, got %d", goroutines, s.Len())
	}
}
