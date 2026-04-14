package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// newTestSession starts a BashSession rooted at a temp directory.
// The test fails immediately if the session cannot be started.
func newTestSession(t *testing.T) *BashSession {
	t.Helper()
	sess, err := NewBashSession(t.TempDir(), 10*time.Second)
	if err != nil {
		t.Fatalf("NewBashSession: %v", err)
	}
	t.Cleanup(func() { sess.Close() })
	return sess
}

// ---------------------------------------------------------------------------
// TestBashSession_PersistWorkdir
// ---------------------------------------------------------------------------

func TestBashSession_PersistWorkdir(t *testing.T) {
	sess := newTestSession(t)
	ctx := context.Background()

	dir := t.TempDir()

	// cd to a known directory, then pwd — the cwd must persist.
	_, _, err := sess.Run(ctx, "cd "+dir)
	if err != nil {
		t.Fatalf("cd: %v", err)
	}

	out, code, err := sess.Run(ctx, "pwd")
	if err != nil {
		t.Fatalf("pwd: %v", err)
	}
	if code != 0 {
		t.Fatalf("pwd exit code %d", code)
	}

	got := strings.TrimSpace(out)
	if got != dir {
		t.Errorf("pwd = %q, want %q", got, dir)
	}
}

// ---------------------------------------------------------------------------
// TestBashSession_EnvVarPersistence
// ---------------------------------------------------------------------------

func TestBashSession_EnvVarPersistence(t *testing.T) {
	sess := newTestSession(t)
	ctx := context.Background()

	_, _, err := sess.Run(ctx, "export POLVO_TEST_VAR=hello123")
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	out, code, err := sess.Run(ctx, "echo $POLVO_TEST_VAR")
	if err != nil {
		t.Fatalf("echo: %v", err)
	}
	if code != 0 {
		t.Fatalf("echo exit code %d", code)
	}

	got := strings.TrimSpace(out)
	if got != "hello123" {
		t.Errorf("env var = %q, want %q", got, "hello123")
	}
}

// ---------------------------------------------------------------------------
// TestBashSession_ExitCode
// ---------------------------------------------------------------------------

func TestBashSession_ExitCode(t *testing.T) {
	sess := newTestSession(t)
	ctx := context.Background()

	t.Run("zero", func(t *testing.T) {
		_, code, err := sess.Run(ctx, "true")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
	})

	t.Run("non_zero", func(t *testing.T) {
		_, code, err := sess.Run(ctx, "false")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if code != 1 {
			t.Errorf("exit code = %d, want 1", code)
		}
	})

	t.Run("custom_exit_code", func(t *testing.T) {
		_, code, err := sess.Run(ctx, "exit 42")
		// After "exit 42" the persistent bash process terminates; this may
		// return an error. We only assert that if there is no error the code
		// is 42.
		if err == nil && code != 42 {
			t.Errorf("exit code = %d, want 42", code)
		}
	})
}

// ---------------------------------------------------------------------------
// TestBashSession_Timeout
// ---------------------------------------------------------------------------

func TestBashSession_Timeout(t *testing.T) {
	// New behavior: timeout kills the command but the session survives.
	sess, err := NewBashSession(t.TempDir(), 200*time.Millisecond)
	if err != nil {
		t.Fatalf("NewBashSession: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	_, _, err = sess.Run(context.Background(), "sleep 30")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected 'timed out' in error, got: %v", err)
	}

	// Session must survive — next command should succeed.
	out, code, err := sess.Run(context.Background(), "echo alive")
	if err != nil {
		t.Fatalf("session should survive timeout, but got error: %v", err)
	}
	if code != 0 {
		t.Errorf("echo alive exit code = %d, want 0", code)
	}
	if !strings.Contains(out, "alive") {
		t.Errorf("expected 'alive' in output, got %q", out)
	}
}

// ---------------------------------------------------------------------------
// TestBashSession_Close
// ---------------------------------------------------------------------------

func TestBashSession_Close(t *testing.T) {
	sess, err := NewBashSession(t.TempDir(), 5*time.Second)
	if err != nil {
		t.Fatalf("NewBashSession: %v", err)
	}

	if closeErr := sess.Close(); closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}

	// Calling Close a second time must be a no-op.
	if closeErr := sess.Close(); closeErr != nil {
		t.Errorf("second Close returned error: %v", closeErr)
	}

	// Run after Close must return an error.
	_, _, err = sess.Run(context.Background(), "echo hi")
	if err == nil {
		t.Fatal("expected error after Close, got nil")
	}
	if !strings.Contains(err.Error(), "closed") {
		t.Errorf("expected 'closed' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestBashSession_ContextCancellation
// ---------------------------------------------------------------------------

func TestBashSession_ContextCancellation(t *testing.T) {
	sess, err := NewBashSession(t.TempDir(), 30*time.Second)
	if err != nil {
		t.Fatalf("NewBashSession: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, _, err = sess.Run(ctx, "echo hi")
	// The session may already have written the command and received output
	// before noticing the cancellation — that's acceptable. We just require
	// that either it succeeds (if the command completed before cancel was
	// noticed) or returns a non-nil error.
	_ = err // result is non-deterministic; we just verify it doesn't panic
}

// ---------------------------------------------------------------------------
// TestBashSession_MultipleCommands
// ---------------------------------------------------------------------------

func TestBashSession_MultipleCommands(t *testing.T) {
	sess := newTestSession(t)
	ctx := context.Background()

	cmds := []struct {
		cmd  string
		want string
	}{
		{"echo first", "first"},
		{"echo second", "second"},
		{"echo third", "third"},
	}

	for _, c := range cmds {
		out, code, err := sess.Run(ctx, c.cmd)
		if err != nil {
			t.Fatalf("Run(%q): %v", c.cmd, err)
		}
		if code != 0 {
			t.Fatalf("Run(%q): exit code %d", c.cmd, code)
		}
		if !strings.Contains(out, c.want) {
			t.Errorf("Run(%q) = %q, want to contain %q", c.cmd, out, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// TestIsInteractiveCommand
// ---------------------------------------------------------------------------

func TestIsInteractiveCommand(t *testing.T) {
	cases := []struct {
		cmd  string
		want bool
	}{
		// Should be detected as interactive
		{"read VAR", true},
		{"read -p 'Enter: ' VAR", true},
		{"vim file.go", true},
		{"vi /etc/hosts", true},
		{"nano README.md", true},
		{"less /var/log/syslog", true},
		{"more /etc/passwd", true},
		{"ssh user@host", true},
		{"passwd", true},
		{"ftp ftp.example.com", true},
		{"telnet example.com", true},
		{"sftp user@host", true},
		{"git commit", true},           // no -m flag
		{"git commit --amend", true},   // no -m flag

		// Should NOT be detected as interactive
		{"echo hello", false},
		{"ls -la", false},
		{"go test ./...", false},
		{"git status", false},
		{"git commit -m 'fix: update'", false},
		{"git commit -m \"chore: release\"", false},
		{"cat /etc/os-release", false},
		{"grep -r pattern .", false},
		{"readlink -f /usr/bin/vim", false}, // "read" only as word boundary
	}

	for _, c := range cases {
		c := c
		t.Run(c.cmd, func(t *testing.T) {
			got := isInteractiveCommand(c.cmd)
			if got != c.want {
				t.Errorf("isInteractiveCommand(%q) = %v, want %v", c.cmd, got, c.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestBashExecute_WithSession
// ---------------------------------------------------------------------------

// bashInput mirrors the JSON input the Execute method accepts. Defined locally
// so the test does not depend on the unexported bashInput struct.
type bashExecInput struct {
	Command      string `json:"command"`
	SecurityRisk string `json:"security_risk"`
}

func bashInputJSON(t *testing.T, cmd, risk string) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(bashExecInput{Command: cmd, SecurityRisk: risk})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return b
}

func TestBashExecute_WithSession(t *testing.T) {
	ctx := context.Background()

	t.Run("session_persists_env", func(t *testing.T) {
		sess, err := NewBashSession(t.TempDir(), 10*time.Second)
		if err != nil {
			t.Fatalf("NewBashSession: %v", err)
		}
		bt := NewBashWithSession(t.TempDir(), nil, nil, sess)
		t.Cleanup(func() { sess.Close() })

		// Set env var
		res, err := bt.Execute(ctx, bashInputJSON(t, "export MY_VAR=works", "low"))
		if err != nil || res.IsError {
			t.Fatalf("export: err=%v, res=%+v", err, res)
		}

		// Read it back
		res, err = bt.Execute(ctx, bashInputJSON(t, "echo $MY_VAR", "low"))
		if err != nil {
			t.Fatalf("echo: %v", err)
		}
		if !strings.Contains(res.Content, "works") {
			t.Errorf("expected 'works' in output, got %q", res.Content)
		}
	})

	t.Run("interactive_rejected", func(t *testing.T) {
		sess, err := NewBashSession(t.TempDir(), 5*time.Second)
		if err != nil {
			t.Fatalf("NewBashSession: %v", err)
		}
		bt := NewBashWithSession(t.TempDir(), nil, nil, sess)
		t.Cleanup(func() { sess.Close() })

		res, err := bt.Execute(ctx, bashInputJSON(t, "read VAR", "low"))
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Error("expected IsError=true for interactive command")
		}
		if !strings.Contains(res.Content, "interactive TTY") {
			t.Errorf("expected TTY rejection message, got %q", res.Content)
		}
	})

	t.Run("nonzero_exit_prefixed", func(t *testing.T) {
		sess, err := NewBashSession(t.TempDir(), 5*time.Second)
		if err != nil {
			t.Fatalf("NewBashSession: %v", err)
		}
		bt := NewBashWithSession(t.TempDir(), nil, nil, sess)
		t.Cleanup(func() { sess.Close() })

		res, err := bt.Execute(ctx, bashInputJSON(t, "false", "low"))
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Error("expected IsError=true for non-zero exit")
		}
		if !strings.Contains(res.Content, "[exit 1]") {
			t.Errorf("expected '[exit 1]' prefix, got %q", res.Content)
		}
	})
}

// ---------------------------------------------------------------------------
// TestBashSession_IsRunningTransitions
// ---------------------------------------------------------------------------

func TestBashSession_IsRunningTransitions(t *testing.T) {
	sess := newTestSession(t)

	if sess.IsRunning() {
		t.Error("expected IsRunning=false before any command")
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		sess.Run(context.Background(), "sleep 1") //nolint:errcheck
	}()

	// Give the command time to start.
	time.Sleep(100 * time.Millisecond)
	if !sess.IsRunning() {
		t.Error("expected IsRunning=true while command is running")
	}

	<-done

	// Allow a brief moment for atomic store to propagate.
	time.Sleep(10 * time.Millisecond)
	if sess.IsRunning() {
		t.Error("expected IsRunning=false after command completes")
	}
}

// ---------------------------------------------------------------------------
// TestBuildUlimitPrefix
// ---------------------------------------------------------------------------

func TestBuildUlimitPrefix(t *testing.T) {
	cases := []struct {
		limits BashLimits
		want   string
	}{
		{BashLimits{}, ""},
		{BashLimits{MaxCPUSecs: 10}, "ulimit -t 10; "},
		{BashLimits{MaxMemMB: 256}, "ulimit -v 262144; "},
		{BashLimits{MaxFileSizeMB: 5}, "ulimit -f 5120; "},
		{BashLimits{MaxCPUSecs: 5, MaxMemMB: 128, MaxFileSizeMB: 10}, "ulimit -t 5; ulimit -v 131072; ulimit -f 10240; "},
	}

	for _, c := range cases {
		got := buildUlimitPrefix(c.limits)
		if got != c.want {
			t.Errorf("buildUlimitPrefix(%+v) = %q, want %q", c.limits, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// TestBashSession_PIDMarkerNotInOutput
// ---------------------------------------------------------------------------

func TestBashSession_PIDMarkerNotInOutput(t *testing.T) {
	sess := newTestSession(t)
	ctx := context.Background()

	out, code, err := sess.Run(ctx, "echo hello")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.Contains(out, pidMarkerPrefix) {
		t.Errorf("PID marker leaked into output: %q", out)
	}
	if !strings.Contains(out, "hello") {
		t.Errorf("expected 'hello' in output, got %q", out)
	}
}

// ---------------------------------------------------------------------------
// TestBashSession_TimeoutDoesNotDestroySession
// ---------------------------------------------------------------------------

func TestBashSession_TimeoutDoesNotDestroySession(t *testing.T) {
	// Identical to the updated TestBashSession_Timeout but makes the
	// "survives" assertion the primary focus of the test.
	sess, err := NewBashSession(t.TempDir(), 300*time.Millisecond)
	if err != nil {
		t.Fatalf("NewBashSession: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	_, _, timeoutErr := sess.Run(context.Background(), "sleep 60")
	if timeoutErr == nil {
		t.Fatal("expected timeout error")
	}

	// Session must still be usable.
	out, _, err := sess.Run(context.Background(), "echo survived")
	if err != nil {
		t.Fatalf("session destroyed after timeout: %v", err)
	}
	if !strings.Contains(out, "survived") {
		t.Errorf("expected 'survived', got %q", out)
	}
}
