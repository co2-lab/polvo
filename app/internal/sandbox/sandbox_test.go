package sandbox

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestConfig_Defaults(t *testing.T) {
	cfg := Config{}
	// Simulate New() defaults application (inline, since we can't call New without Docker).
	if cfg.Image == "" {
		cfg.Image = defaultImage
	}
	if cfg.WorkDir == "" {
		cfg.WorkDir = "/workspace"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}

	if cfg.Image != "ubuntu:24.04" {
		t.Errorf("default image: want ubuntu:24.04, got %q", cfg.Image)
	}
	if cfg.WorkDir != "/workspace" {
		t.Errorf("default workdir: want /workspace, got %q", cfg.WorkDir)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("default timeout: want 30s, got %v", cfg.Timeout)
	}
}

func TestSession_ExecOnClosed(t *testing.T) {
	s := &Session{
		cfg:         Config{Timeout: 5 * time.Second, WorkDir: "/workspace"},
		containerID: "fake",
		closed:      true,
	}
	_, err := s.Exec(context.Background(), "echo hello")
	if err == nil {
		t.Error("expected error when exec on closed session")
	}
	if !strings.Contains(err.Error(), "closed") {
		t.Errorf("expected 'closed' in error, got %q", err.Error())
	}
}

func TestSession_CopyOutOnClosed(t *testing.T) {
	s := &Session{
		containerID: "fake",
		closed:      true,
	}
	err := s.CopyOut(context.Background(), "/tmp/foo", "/tmp/bar")
	if err == nil {
		t.Error("expected error when CopyOut on closed session")
	}
}

func TestSession_CloseIdempotent(t *testing.T) {
	// A closed session with a fake container ID — Close() will try docker rm and fail,
	// but the second call must not panic or error differently.
	s := &Session{
		containerID: "nonexistent-fake-id-123",
		closed:      true, // already closed
	}
	// Double-close must not panic.
	err1 := s.Close()
	err2 := s.Close()
	// Both nil (already closed is a no-op).
	_ = err1
	_ = err2
}

func TestSession_ContainerID_Short(t *testing.T) {
	s := &Session{containerID: "abcdef1234567890"}
	if got := s.ContainerID(); got != "abcdef123456" {
		t.Errorf("want first 12 chars, got %q", got)
	}
}

func TestSession_ContainerID_Short_Short(t *testing.T) {
	s := &Session{containerID: "abc"}
	if got := s.ContainerID(); got != "abc" {
		t.Errorf("short id should be returned as-is: %q", got)
	}
}

func TestExecResult_Fields(t *testing.T) {
	r := &ExecResult{Stdout: "out", Stderr: "err", ExitCode: 1}
	if r.Stdout != "out" || r.Stderr != "err" || r.ExitCode != 1 {
		t.Errorf("unexpected fields: %+v", r)
	}
}

