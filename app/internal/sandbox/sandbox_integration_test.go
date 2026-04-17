//go:build integration

package sandbox

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSandbox_Integration(t *testing.T) {
	ctx := context.Background()
	sess, err := New(ctx, Config{
		Image:   "alpine:latest",
		Network: false,
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sess.Close()

	res, err := sess.Exec(ctx, "echo hello-from-sandbox")
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("exit code: %d, stderr: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "hello-from-sandbox") {
		t.Errorf("expected 'hello-from-sandbox' in stdout, got %q", res.Stdout)
	}
}

func TestSandbox_Integration_ExitCode(t *testing.T) {
	ctx := context.Background()
	sess, err := New(ctx, Config{Image: "alpine:latest"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sess.Close()

	res, err := sess.Exec(ctx, "exit 42")
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.ExitCode != 42 {
		t.Errorf("expected exit code 42, got %d", res.ExitCode)
	}
}
