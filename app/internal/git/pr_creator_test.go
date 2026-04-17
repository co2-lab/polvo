package git

import (
	"context"
	"strings"
	"testing"
)

// mockRunner records the last Run call.
type mockRunner struct {
	name   string
	args   []string
	output string
	err    error
}

func (m *mockRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	m.name = name
	m.args = args
	return m.output, m.err
}

func TestGHCLIPRCreator_CreatePR(t *testing.T) {
	mock := &mockRunner{output: "https://github.com/owner/repo/pull/1"}
	c := &GHCLIPRCreator{Runner: mock}

	result, err := c.CreatePR(context.Background(), PROptions{
		Owner: "owner",
		Repo:  "repo",
		Title: "test title",
		Body:  "test body",
		Head:  "feature-branch",
		Base:  "main",
		Draft: true,
	})
	if err != nil {
		t.Fatalf("CreatePR: %v", err)
	}
	if result.URL != "https://github.com/owner/repo/pull/1" {
		t.Errorf("URL = %q", result.URL)
	}

	// Check --draft was included.
	args := strings.Join(mock.args, " ")
	if !strings.Contains(args, "--draft") {
		t.Error("expected --draft in args")
	}
	if !strings.Contains(args, "--title") {
		t.Error("expected --title in args")
	}
	if !strings.Contains(args, "--base") {
		t.Error("expected --base in args")
	}
}

func TestGHCLIPRCreator_NoDraftWhenDisabled(t *testing.T) {
	mock := &mockRunner{output: "https://github.com/owner/repo/pull/2"}
	c := &GHCLIPRCreator{Runner: mock}

	_, err := c.CreatePR(context.Background(), PROptions{
		Title: "t",
		Body:  "b",
		Head:  "branch",
		Base:  "main",
		Draft: false,
	})
	if err != nil {
		t.Fatal(err)
	}

	args := strings.Join(mock.args, " ")
	if strings.Contains(args, "--draft") {
		t.Error("--draft should not be in args when Draft=false")
	}
}

func TestGHCLIPRCreator_ExistingPR_Found(t *testing.T) {
	mock := &mockRunner{output: "https://github.com/owner/repo/pull/99"}
	c := &GHCLIPRCreator{Runner: mock}

	url, err := c.ExistingPRURL(context.Background(), "feature-branch")
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://github.com/owner/repo/pull/99" {
		t.Errorf("ExistingPRURL = %q", url)
	}
}

func TestGHCLIPRCreator_NotFound(t *testing.T) {
	mock := &mockRunner{output: "", err: &execError{}}
	c := &GHCLIPRCreator{Runner: mock}

	url, err := c.ExistingPRURL(context.Background(), "feature-branch")
	if err != nil {
		t.Fatalf("ExistingPRURL should not return error when PR not found: %v", err)
	}
	if url != "" {
		t.Errorf("expected empty URL, got %q", url)
	}
}

// execError is a minimal error to simulate exec failure.
type execError struct{}

func (e *execError) Error() string { return "exit status 1" }

func TestBuildPRBody(t *testing.T) {
	body := BuildPRBody("my-agent", "fixed the bug", []string{"main.go", "handler.go"})
	if !strings.Contains(body, "## Summary") {
		t.Error("missing ## Summary")
	}
	if !strings.Contains(body, "fixed the bug") {
		t.Error("missing summary text")
	}
	if !strings.Contains(body, "## Changes") {
		t.Error("missing ## Changes")
	}
	if !strings.Contains(body, "main.go") {
		t.Error("missing file main.go")
	}
	if !strings.Contains(body, "my-agent") {
		t.Error("missing agent name in footer")
	}
}

func TestParseOwnerRepo_SSH(t *testing.T) {
	owner, repo := ParseOwnerRepo("git@github.com:co2-lab/polvo.git")
	if owner != "co2-lab" {
		t.Errorf("owner = %q, want co2-lab", owner)
	}
	if repo != "polvo" {
		t.Errorf("repo = %q, want polvo", repo)
	}
}

func TestParseOwnerRepo_HTTPS(t *testing.T) {
	owner, repo := ParseOwnerRepo("https://github.com/co2-lab/polvo.git")
	if owner != "co2-lab" {
		t.Errorf("owner = %q, want co2-lab", owner)
	}
	if repo != "polvo" {
		t.Errorf("repo = %q, want polvo", repo)
	}
}

func TestParseOwnerRepo_Invalid(t *testing.T) {
	owner, repo := ParseOwnerRepo("not-a-url")
	if owner != "" || repo != "" {
		t.Errorf("expected empty, got owner=%q repo=%q", owner, repo)
	}
}
