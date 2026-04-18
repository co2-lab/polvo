// Package sandbox provides Docker-based execution isolation for agent sessions.
// Each session runs inside a dedicated container with the project root bind-mounted
// read-only. A writable tmpfs overlay holds agent-generated files so the host is
// never mutated without explicit approval.
package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	defaultImage   = "ubuntu:24.04"
	defaultTimeout = 30 * time.Second
)

// Config configures a sandbox session.
type Config struct {
	// Image is the Docker image to use (default: ubuntu:24.04).
	Image string
	// ProjectRoot is the host path bind-mounted at /workspace (read-only).
	ProjectRoot string
	// WorkDir is the working directory inside the container (default: /workspace).
	WorkDir string
	// Env holds extra KEY=VALUE pairs injected into every exec.
	Env []string
	// Timeout is the per-command execution timeout (default: 30s).
	Timeout time.Duration
	// MemoryMB limits container RAM (0 = no limit).
	MemoryMB int
	// CPUs limits container CPU quota (0.0 = no limit).
	CPUs float64
	// Network disables network access when false (default: disabled).
	Network bool
}

// ExecResult is the output of a sandboxed command.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Session is a running Docker sandbox container scoped to one agent session.
// Create via New(), call Exec() to run commands, Close() to destroy the container.
type Session struct {
	cfg         Config
	containerID string
	mu          sync.Mutex
	closed      bool
}

// New starts a Docker container for sandboxed execution.
// Returns a Session that must be closed when done.
func New(ctx context.Context, cfg Config) (*Session, error) {
	if cfg.Image == "" {
		cfg.Image = defaultImage
	}
	if cfg.WorkDir == "" {
		cfg.WorkDir = "/workspace"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}

	args := []string{
		"docker", "run",
		"--detach",
		"--rm",
		"--init",
		"--workdir", cfg.WorkDir,
	}

	// Bind-mount project root read-only.
	if cfg.ProjectRoot != "" {
		args = append(args, "--volume", cfg.ProjectRoot+":/workspace:ro")
	}

	// Writable tmpfs for agent-generated files.
	args = append(args, "--tmpfs", "/tmp:exec,size=256m")

	// Resource limits.
	if cfg.MemoryMB > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", cfg.MemoryMB))
	}
	if cfg.CPUs > 0 {
		args = append(args, "--cpus", fmt.Sprintf("%.2f", cfg.CPUs))
	}

	// Network access (disabled by default for safety).
	if !cfg.Network {
		args = append(args, "--network", "none")
	}

	// Environment variables.
	for _, e := range cfg.Env {
		args = append(args, "--env", e)
	}

	// Keep container alive with an idle process.
	args = append(args, cfg.Image, "sleep", "infinity")

	out, err := runCmd(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("sandbox: docker run failed: %w\noutput: %s", err, out)
	}

	containerID := strings.TrimSpace(out)
	if containerID == "" {
		return nil, fmt.Errorf("sandbox: docker run returned empty container ID")
	}

	slog.Info("sandbox: container started",
		"container", containerID[:12],
		"image", cfg.Image,
	)

	return &Session{cfg: cfg, containerID: containerID}, nil
}

// Exec runs a shell command inside the sandbox container.
// The command is executed via `sh -c` with the configured timeout.
func (s *Session) Exec(ctx context.Context, command string) (*ExecResult, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, fmt.Errorf("sandbox: session is closed")
	}
	s.mu.Unlock()

	timeout := s.cfg.Timeout
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{
		"docker", "exec",
		"--workdir", s.cfg.WorkDir,
	}
	for _, e := range s.cfg.Env {
		args = append(args, "--env", e)
	}
	args = append(args, s.containerID, "sh", "-c", command)

	cmd := exec.CommandContext(execCtx, args[0], args[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			err = nil // non-zero exit is not a Go error — it's in ExitCode
		} else {
			return nil, fmt.Errorf("sandbox: exec failed: %w", err)
		}
	}

	return &ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}, nil
}

// CopyOut copies a file from the container to the host path dst.
func (s *Session) CopyOut(ctx context.Context, containerPath, hostPath string) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("sandbox: session is closed")
	}
	s.mu.Unlock()

	_, err := runCmd(ctx, "docker", "cp",
		s.containerID+":"+containerPath,
		hostPath,
	)
	return err
}

// ContainerID returns the short container ID (first 12 chars).
func (s *Session) ContainerID() string {
	if len(s.containerID) >= 12 {
		return s.containerID[:12]
	}
	return s.containerID
}

// Close stops and removes the sandbox container. Idempotent.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := runCmd(ctx, "docker", "rm", "-f", s.containerID)
	if err != nil {
		slog.Warn("sandbox: failed to remove container",
			"container", s.ContainerID(),
			"error", err,
			"output", out,
		)
		return err
	}
	slog.Info("sandbox: container removed", "container", s.ContainerID())
	return nil
}

// runCmd runs a command and returns combined stdout+stderr output and error.
func runCmd(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}
