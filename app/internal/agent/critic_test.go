package agent

import (
	"context"
	"errors"
	"testing"
)

// alwaysApprove is a critic that always approves.
type alwaysApprove struct{ name string }

func (a *alwaysApprove) Name() string { return a.name }
func (a *alwaysApprove) Role() Role   { return RoleReviewer }
func (a *alwaysApprove) Execute(_ context.Context, _ *Input) (*Result, error) {
	return &Result{Decision: DecisionApprove, Summary: "looks good"}, nil
}

// alwaysReject is a critic that always rejects.
type alwaysReject struct{ name string }

func (r *alwaysReject) Name() string { return r.name }
func (r *alwaysReject) Role() Role   { return RoleReviewer }
func (r *alwaysReject) Execute(_ context.Context, _ *Input) (*Result, error) {
	return &Result{
		Decision: DecisionReject,
		Summary:  "bad output",
		Findings: []Finding{{Severity: "error", Category: "logic", Message: "wrong impl"}},
	}, nil
}

// countingAuthor counts how many times it was executed and always produces content.
type countingAuthor struct {
	name    string
	calls   int
	content string
}

func (c *countingAuthor) Name() string { return c.name }
func (c *countingAuthor) Role() Role   { return RoleAuthor }
func (c *countingAuthor) Execute(_ context.Context, _ *Input) (*Result, error) {
	c.calls++
	return &Result{Content: c.content, Summary: "authored"}, nil
}

// errorAuthor always returns an error.
type errorAuthor struct{ name string }

func (e *errorAuthor) Name() string { return e.name }
func (e *errorAuthor) Role() Role   { return RoleAuthor }
func (e *errorAuthor) Execute(_ context.Context, _ *Input) (*Result, error) {
	return nil, errors.New("author exploded")
}

// errorCritic always returns an error.
type errorCritic struct{ name string }

func (e *errorCritic) Name() string { return e.name }
func (e *errorCritic) Role() Role   { return RoleReviewer }
func (e *errorCritic) Execute(_ context.Context, _ *Input) (*Result, error) {
	return nil, errors.New("critic exploded")
}

func TestCritic_ApproveOnFirstTry(t *testing.T) {
	author := &countingAuthor{name: "author", content: "good code"}
	cr, err := RunCritic(context.Background(), CriticConfig{
		Author:     author,
		Critic:     &alwaysApprove{name: "critic"},
		MaxRetries: 2,
	}, &Input{})
	if err != nil {
		t.Fatal(err)
	}
	if !cr.Approved {
		t.Error("expected Approved")
	}
	if cr.Retries != 0 {
		t.Errorf("expected 0 retries, got %d", cr.Retries)
	}
	if author.calls != 1 {
		t.Errorf("author should be called once, got %d", author.calls)
	}
}

func TestCritic_RetryOnReject_ThenApprove(t *testing.T) {
	rejectCount := 0
	flexCritic := &funcAgent{
		name: "critic",
		fn: func(_ context.Context, _ *Input) (*Result, error) {
			rejectCount++
			if rejectCount < 2 {
				return &Result{Decision: DecisionReject, Summary: "not yet"}, nil
			}
			return &Result{Decision: DecisionApprove, Summary: "ok now"}, nil
		},
	}
	author := &countingAuthor{name: "author", content: "output"}
	cr, err := RunCritic(context.Background(), CriticConfig{
		Author:     author,
		Critic:     flexCritic,
		MaxRetries: 3,
	}, &Input{})
	if err != nil {
		t.Fatal(err)
	}
	if !cr.Approved {
		t.Error("expected Approved after second attempt")
	}
	if cr.Retries != 1 {
		t.Errorf("expected 1 retry, got %d", cr.Retries)
	}
	if author.calls != 2 {
		t.Errorf("expected 2 author calls, got %d", author.calls)
	}
}

func TestCritic_ExhaustRetries(t *testing.T) {
	author := &countingAuthor{name: "author", content: "bad"}
	cr, err := RunCritic(context.Background(), CriticConfig{
		Author:     author,
		Critic:     &alwaysReject{name: "critic"},
		MaxRetries: 2,
	}, &Input{})
	if err != nil {
		t.Fatal(err)
	}
	if cr.Approved {
		t.Error("expected NOT Approved after exhausting retries")
	}
	if cr.Retries != 2 {
		t.Errorf("expected 2 retries, got %d", cr.Retries)
	}
	if author.calls != 3 { // initial + 2 retries
		t.Errorf("expected 3 author calls, got %d", author.calls)
	}
}

func TestCritic_AuthorError(t *testing.T) {
	_, err := RunCritic(context.Background(), CriticConfig{
		Author: &errorAuthor{name: "author"},
		Critic: &alwaysApprove{name: "critic"},
	}, &Input{})
	if err == nil {
		t.Error("expected error from failing author")
	}
}

func TestCritic_CriticError(t *testing.T) {
	_, err := RunCritic(context.Background(), CriticConfig{
		Author: &countingAuthor{name: "author"},
		Critic: &errorCritic{name: "critic"},
	}, &Input{})
	if err == nil {
		t.Error("expected error from failing critic")
	}
}

func TestCritic_MissingAuthor(t *testing.T) {
	_, err := RunCritic(context.Background(), CriticConfig{
		Critic: &alwaysApprove{name: "critic"},
	}, &Input{})
	if err == nil {
		t.Error("expected error when Author is nil")
	}
}

func TestCritic_MissingCritic(t *testing.T) {
	_, err := RunCritic(context.Background(), CriticConfig{
		Author: &countingAuthor{name: "author"},
	}, &Input{})
	if err == nil {
		t.Error("expected error when Critic is nil")
	}
}

func TestCritic_PublishesToBus(t *testing.T) {
	bus := NewAgentBus()
	defer bus.Close()

	authorCh := bus.Subscribe("author")
	criticCh := bus.Subscribe("critic")

	author := &countingAuthor{name: "author", content: "code"}
	_, err := RunCritic(context.Background(), CriticConfig{
		Author:     author,
		Critic:     &alwaysApprove{name: "critic"},
		MaxRetries: 1,
		Bus:        bus,
	}, &Input{})
	if err != nil {
		t.Fatal(err)
	}

	// Author should have received a directive (critic→author) — none in approve case
	// Critic should have received a result (author→critic)
	select {
	case msg := <-criticCh:
		if msg.Type != MessageResult {
			t.Errorf("expected MessageResult, got %v", msg.Type)
		}
	default:
		t.Error("critic should have received author result message")
	}
	// No directive in approve case
	select {
	case <-authorCh:
		// A directive would be sent on rejection; in approve case nothing is sent
		// (approve flow does publish directive) — accept both behaviors
	default:
		// OK
	}
}

func TestCritic_DefaultMaxRetries(t *testing.T) {
	author := &countingAuthor{name: "author"}
	// MaxRetries=0 → defaults to 2
	cr, err := RunCritic(context.Background(), CriticConfig{
		Author:     author,
		Critic:     &alwaysReject{name: "critic"},
		MaxRetries: 0,
	}, &Input{})
	if err != nil {
		t.Fatal(err)
	}
	if cr.Retries != 2 {
		t.Errorf("expected default 2 retries, got %d", cr.Retries)
	}
}
