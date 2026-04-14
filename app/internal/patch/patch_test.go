package patch

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/co2-lab/polvo/internal/provider"
)

// ---------------------------------------------------------------------------
// sanitizeName
// ---------------------------------------------------------------------------

func TestSanitizeName(t *testing.T) {
	// Real separator is "-" and truncation at 64 chars (not "_" or 40)
	cases := []struct {
		input string
		want  string
	}{
		{"myagent", "myagent"},
		{"my/agent", "my-agent"},
		{"my\\agent", "my-agent"},
		{"my agent", "my-agent"},
		{"my:agent", "my-agent"},
		// Special chars not in replacer — pass through unchanged
		{"*", "*"},
		{"?", "?"},
		{"<", "<"},
		{">", ">"},
		// Exactly 64 chars — no truncation
		{strings.Repeat("a", 64), strings.Repeat("a", 64)},
		// 65 chars — truncated to 64
		{strings.Repeat("a", 65), strings.Repeat("a", 64)},
		// Empty string — no panic
		{"", ""},
	}
	for _, tc := range cases {
		got := sanitizeName(tc.input)
		if got != tc.want {
			t.Errorf("sanitizeName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// isGitRepo
// ---------------------------------------------------------------------------

func TestIsGitRepo(t *testing.T) {
	t.Run("with .git dir", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".git"), 0755); err != nil {
			t.Fatal(err)
		}
		if !isGitRepo(dir) {
			t.Error("expected isGitRepo=true for dir with .git/")
		}
	})

	t.Run("without .git dir", func(t *testing.T) {
		dir := t.TempDir()
		if isGitRepo(dir) {
			t.Error("expected isGitRepo=false for dir without .git/")
		}
	})

	t.Run("non-existent dir", func(t *testing.T) {
		if isGitRepo("/tmp/polvo-test-nonexistent-xyzzy-12345") {
			t.Error("expected isGitRepo=false for non-existent dir")
		}
	})
}

// ---------------------------------------------------------------------------
// git helpers
// ---------------------------------------------------------------------------

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}
}

func setupGitRepo(t *testing.T) string {
	t.Helper()
	requireGit(t)
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "ci@test.com"},
		{"config", "user.name", "CI"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

// ---------------------------------------------------------------------------
// Writer.Write integration
// ---------------------------------------------------------------------------

func TestPatchWriter_NoGit(t *testing.T) {
	dir := t.TempDir() // no .git
	w := New(dir)

	// Create a dummy file to reference
	dummy := filepath.Join(dir, "a.go")
	os.WriteFile(dummy, []byte("package main\n"), 0644)

	patchPath, err := w.Write(context.Background(), "myagent", []string{"a.go"})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if patchPath == "" {
		t.Fatal("expected non-empty patch path")
	}

	// Patch should be inside .polvo/patches/
	expectedDir := filepath.Join(dir, ".polvo", "patches")
	if !strings.HasPrefix(patchPath, expectedDir) {
		t.Errorf("patch path %q should be under %q", patchPath, expectedDir)
	}

	// Filename should contain agent name (sanitized)
	base := filepath.Base(patchPath)
	if !strings.Contains(base, "myagent") {
		t.Errorf("patch filename %q should contain 'myagent'", base)
	}

	// Patch content should have no-git header
	content, err := os.ReadFile(patchPath)
	if err != nil {
		t.Fatalf("reading patch: %v", err)
	}
	if !strings.HasPrefix(string(content), "# polvo patch — no git repo") {
		t.Errorf("expected no-git header, got: %q", string(content)[:min(80, len(string(content)))])
	}

	// Patch should list the files
	if !strings.Contains(string(content), "# modified: a.go") {
		t.Errorf("patch should mention 'a.go', got:\n%s", string(content))
	}
}

func TestPatchWriter_NoFiles(t *testing.T) {
	dir := t.TempDir()
	w := New(dir)

	patchPath, err := w.Write(context.Background(), "myagent", []string{})
	if err != nil {
		t.Fatalf("Write with no files: %v", err)
	}
	if patchPath != "" {
		t.Errorf("expected empty path for no files, got %q", patchPath)
	}
}

func TestPatchWriter_WithGit(t *testing.T) {
	dir := setupGitRepo(t)
	w := New(dir)

	// Write a file
	dummy := filepath.Join(dir, "a.go")
	os.WriteFile(dummy, []byte("package main\n"), 0644)

	patchPath, err := w.Write(context.Background(), "myagent", []string{"a.go"})
	if err != nil {
		t.Fatalf("Write with git: %v", err)
	}
	// Should not panic; patchPath is valid (may have empty diff content)
	if patchPath == "" {
		t.Fatal("expected non-empty patch path even with git")
	}
	if _, err := os.Stat(patchPath); err != nil {
		t.Errorf("patch file should exist: %v", err)
	}
}

func TestPatchWriter_SpecialCharsInName(t *testing.T) {
	dir := t.TempDir()
	w := New(dir)

	dummy := filepath.Join(dir, "a.go")
	os.WriteFile(dummy, []byte("package main\n"), 0644)

	patchPath, err := w.Write(context.Background(), "my/agent", []string{"a.go"})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	base := filepath.Base(patchPath)
	// "/" should be sanitized to "-" in filename
	if strings.Contains(base, "/") {
		t.Errorf("filename should not contain '/', got: %q", base)
	}
	if !strings.Contains(base, "my-agent") {
		t.Errorf("filename should contain 'my-agent', got: %q", base)
	}
}

// ---------------------------------------------------------------------------
// mock provider for CommitMessageGenerator tests
// ---------------------------------------------------------------------------

type mockChatProvider struct {
	response string
	err      error
}

func (m *mockChatProvider) Chat(_ context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &provider.ChatResponse{
		Message: provider.Message{Content: m.response},
	}, nil
}

func (m *mockChatProvider) Name() string                                    { return "mock" }
func (m *mockChatProvider) Complete(_ context.Context, _ provider.Request) (*provider.Response, error) {
	return nil, nil
}
func (m *mockChatProvider) Available(_ context.Context) error { return nil }

// ---------------------------------------------------------------------------
// CommitMessageGenerator.Generate
// ---------------------------------------------------------------------------

func TestCommitMessage_NilProvider(t *testing.T) {
	g := &CommitMessageGenerator{Provider: nil, AgentName: ""}
	msg := g.Generate(context.Background(), "some diff")
	// Fallback with no AgentName — subject line should be the fallback.
	wantSubject := "chore: apply polvo agent changes"
	if !strings.HasPrefix(msg, wantSubject) {
		t.Errorf("subject: got %q, want prefix %q", msg, wantSubject)
	}
}

func TestCommitMessage_NilProvider_WithAgentName(t *testing.T) {
	g := &CommitMessageGenerator{Provider: nil, AgentName: "myagent"}
	msg := g.Generate(context.Background(), "some diff")
	wantSubject := "chore(myagent): apply agent changes"
	if !strings.HasPrefix(msg, wantSubject) {
		t.Errorf("subject: got %q, want prefix %q", msg, wantSubject)
	}
}

func TestCommitMessage_EmptyDiff(t *testing.T) {
	mock := &mockChatProvider{response: "feat: add thing"}
	g := &CommitMessageGenerator{Provider: mock, AgentName: "ag"}
	// empty diff → fallback (provider not called due to early return)
	msg := g.Generate(context.Background(), "")
	wantSubject := "chore(ag): apply agent changes"
	if !strings.HasPrefix(msg, wantSubject) {
		t.Errorf("subject: got %q, want prefix %q", msg, wantSubject)
	}
}

func TestCommitMessage_ValidResponse(t *testing.T) {
	validMsg := "feat: add authentication"
	mock := &mockChatProvider{response: validMsg}
	g := &CommitMessageGenerator{Provider: mock, AgentName: "ag"}
	msg := g.Generate(context.Background(), "some diff content")
	if !strings.HasPrefix(msg, validMsg) {
		t.Errorf("subject: got %q, want prefix %q", msg, validMsg)
	}
}

func TestCommitMessage_TooLong(t *testing.T) {
	tooLong := strings.Repeat("x", 73)
	mock := &mockChatProvider{response: tooLong}
	g := &CommitMessageGenerator{Provider: mock, AgentName: "myagent"}
	msg := g.Generate(context.Background(), "some diff")
	wantSubject := "chore(myagent): apply agent changes"
	if !strings.HasPrefix(msg, wantSubject) {
		t.Errorf("subject: got %q, want prefix %q", msg, wantSubject)
	}
}

func TestCommitMessage_ProviderError(t *testing.T) {
	mock := &mockChatProvider{err: fmt.Errorf("LLM unavailable")}
	g := &CommitMessageGenerator{Provider: mock, AgentName: "myagent"}
	msg := g.Generate(context.Background(), "some diff")
	wantSubject := "chore(myagent): apply agent changes"
	if !strings.HasPrefix(msg, wantSubject) {
		t.Errorf("subject: got %q, want prefix %q", msg, wantSubject)
	}
}

func TestCommitMessage_QuoteStripping(t *testing.T) {
	// Provider returns message wrapped in double quotes → stripped
	mock := &mockChatProvider{response: `"feat: add auth"`}
	g := &CommitMessageGenerator{Provider: mock, AgentName: "ag"}
	msg := g.Generate(context.Background(), "some diff")
	wantSubject := "feat: add auth"
	if !strings.HasPrefix(msg, wantSubject) {
		t.Errorf("subject: got %q, want prefix %q", msg, wantSubject)
	}
}

func TestCommitMessage_QuoteStripping_SingleQuote(t *testing.T) {
	mock := &mockChatProvider{response: `'feat: add auth'`}
	g := &CommitMessageGenerator{Provider: mock, AgentName: "ag"}
	msg := g.Generate(context.Background(), "some diff")
	wantSubject := "feat: add auth"
	if !strings.HasPrefix(msg, wantSubject) {
		t.Errorf("subject: got %q, want prefix %q", msg, wantSubject)
	}
}

func TestCommitMessage_UnmatchedQuotesPreserved(t *testing.T) {
	// Internal quotes are not stripped (strings.Trim only removes from edges)
	inner := `feat: add "user" auth`
	mock := &mockChatProvider{response: inner}
	g := &CommitMessageGenerator{Provider: mock, AgentName: "ag"}
	msg := g.Generate(context.Background(), "some diff")
	if !strings.HasPrefix(msg, inner) {
		t.Errorf("subject: got %q, want prefix %q", msg, inner)
	}
}

// TestCommitMessage_WatcherName verifies that WatcherName and Timestamp are
// included as trailers in the generated commit message body.
func TestCommitMessage_WatcherName(t *testing.T) {
	fixedTime := time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC)
	g := &CommitMessageGenerator{
		Provider:    nil,
		AgentName:   "myagent",
		WatcherName: "file-watcher",
		Timestamp:   fixedTime,
	}

	msg := g.Generate(context.Background(), "some diff")

	// Subject line must be present.
	wantSubject := "chore(myagent): apply agent changes"
	if !strings.HasPrefix(msg, wantSubject) {
		t.Errorf("subject: got %q, want prefix %q", msg, wantSubject)
	}

	// Triggered-by trailer must contain the watcher name.
	if !strings.Contains(msg, "Triggered-by: file-watcher") {
		t.Errorf("commit message missing 'Triggered-by: file-watcher':\n%s", msg)
	}

	// Generated-at trailer must contain the RFC3339 timestamp.
	wantTS := fixedTime.UTC().Format(time.RFC3339)
	if !strings.Contains(msg, "Generated-at: "+wantTS) {
		t.Errorf("commit message missing 'Generated-at: %s':\n%s", wantTS, msg)
	}
}

// TestCommitMessage_TimestampDefaultsToNow verifies that when Timestamp is zero,
// the Generated-at trailer is still present (using time.Now()).
func TestCommitMessage_TimestampDefaultsToNow(t *testing.T) {
	g := &CommitMessageGenerator{
		Provider:  nil,
		AgentName: "ag",
	}
	before := time.Now().UTC().Truncate(time.Second)
	msg := g.Generate(context.Background(), "diff")
	after := time.Now().UTC().Add(time.Second)

	if !strings.Contains(msg, "Generated-at: ") {
		t.Fatalf("commit message missing Generated-at trailer:\n%s", msg)
	}

	// Extract the timestamp value from the trailer.
	idx := strings.Index(msg, "Generated-at: ")
	tsStr := strings.TrimSpace(msg[idx+len("Generated-at: "):])
	// tsStr may have more lines after; take only the first token.
	if nl := strings.IndexByte(tsStr, '\n'); nl >= 0 {
		tsStr = tsStr[:nl]
	}
	ts, err := time.Parse(time.RFC3339, tsStr)
	if err != nil {
		t.Fatalf("Generated-at is not valid RFC3339 %q: %v", tsStr, err)
	}
	if ts.Before(before) || ts.After(after) {
		t.Errorf("Generated-at %v is outside expected range [%v, %v]", ts, before, after)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
