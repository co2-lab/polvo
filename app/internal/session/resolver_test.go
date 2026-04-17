package session

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestResolver_NoRefs(t *testing.T) {
	db := openTestDB(t)
	mgr, _ := Open(db)
	r := NewResolver(mgr, nil)

	out, err := r.Resolve(context.Background(), "hello world")
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello world" {
		t.Errorf("expected unchanged, got %q", out)
	}
}

func TestResolver_WithSummary(t *testing.T) {
	db := openTestDB(t)
	mgr, _ := Open(db)
	ctx := context.Background()

	wi, _ := mgr.Start(ctx, KindTask, "implement auth")
	mgr.SetSummary(ctx, wi.ID, "Auth implementation task")

	r := NewResolver(mgr, nil)
	out, err := r.Resolve(ctx, "continue from @@task[task#01]")
	if err != nil {
		t.Fatal(err)
	}
	if out != "continue from Auth implementation task" {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestResolver_FallbackToPrompt(t *testing.T) {
	db := openTestDB(t)
	mgr, _ := Open(db)
	ctx := context.Background()

	// No summary set — should fall back to the prompt text.
	mgr.Start(ctx, KindQuestion, "what is X?")

	r := NewResolver(mgr, nil)
	out, err := r.Resolve(ctx, "ref: @@question[question#01]")
	if err != nil {
		t.Fatal(err)
	}
	if out != "ref: what is X?" {
		t.Errorf("expected prompt fallback, got %q", out)
	}
}

func TestResolver_UnknownID(t *testing.T) {
	db := openTestDB(t)
	mgr, _ := Open(db)
	ctx := context.Background()

	r := NewResolver(mgr, nil)
	// Unknown ID falls back to the ID string itself.
	out, err := r.Resolve(ctx, "see @@task[task#99]")
	if err != nil {
		t.Fatal(err)
	}
	if out != "see task#99" {
		t.Errorf("expected id fallback, got %q", out)
	}
}

func TestResolver_MultipleRefs(t *testing.T) {
	db := openTestDB(t)
	mgr, _ := Open(db)
	ctx := context.Background()

	wi1, _ := mgr.Start(ctx, KindTask, "feature A")
	wi2, _ := mgr.Start(ctx, KindQuestion, "what about B?")
	mgr.SetSummary(ctx, wi1.ID, "Summary A")
	mgr.SetSummary(ctx, wi2.ID, "Summary B")

	r := NewResolver(mgr, nil)
	out, err := r.Resolve(ctx, "ref @@task[task#01] and @@question[question#02]")
	if err != nil {
		t.Fatal(err)
	}
	if out != "ref Summary A and Summary B" {
		t.Errorf("unexpected: %q", out)
	}
}

func TestHasRefs(t *testing.T) {
	if HasRefs("no refs here") {
		t.Error("should be false")
	}
	if !HasRefs("see @@task[task#01]") {
		t.Error("should be true")
	}
}

func TestListRefs(t *testing.T) {
	refs := ListRefs("do @@task[task#01] and @@question[question#02] and @@task[task#01] again")
	if len(refs) != 2 {
		t.Errorf("want 2 unique refs, got %d: %v", len(refs), refs)
	}
}

// Satisfy the openTestDB dependency used in this file too.
var _ *sql.DB
