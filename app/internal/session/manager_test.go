package session

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestManager_StartAndGet(t *testing.T) {
	db := openTestDB(t)
	mgr, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	wi, err := mgr.Start(ctx, KindTask, "implement feature X")
	if err != nil {
		t.Fatal(err)
	}

	if wi.ID != "task#01" {
		t.Errorf("want ID task#01, got %q", wi.ID)
	}
	if wi.Kind != KindTask {
		t.Errorf("want kind task, got %q", wi.Kind)
	}
	if wi.Prompt != "implement feature X" {
		t.Errorf("unexpected prompt %q", wi.Prompt)
	}
	if wi.StartedAt.IsZero() {
		t.Error("StartedAt should not be zero")
	}

	got, err := mgr.Get(ctx, "task#01")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if got.ID != wi.ID || got.Prompt != wi.Prompt {
		t.Errorf("Get mismatch: %+v", got)
	}
}

func TestManager_SequentialIDs(t *testing.T) {
	db := openTestDB(t)
	mgr, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	wi1, _ := mgr.Start(ctx, KindTask, "first")
	wi2, _ := mgr.Start(ctx, KindQuestion, "second")
	wi3, _ := mgr.Start(ctx, KindTask, "third")

	if wi1.ID != "task#01" {
		t.Errorf("want task#01, got %q", wi1.ID)
	}
	if wi2.ID != "question#02" {
		t.Errorf("want question#02, got %q", wi2.ID)
	}
	if wi3.ID != "task#03" {
		t.Errorf("want task#03, got %q", wi3.ID)
	}
}

func TestManager_Finish(t *testing.T) {
	db := openTestDB(t)
	mgr, _ := Open(db)
	ctx := context.Background()

	wi, _ := mgr.Start(ctx, KindTask, "x")
	if err := mgr.Finish(ctx, wi.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := mgr.Get(ctx, wi.ID)
	if got.EndedAt.IsZero() {
		t.Error("EndedAt should be set after Finish")
	}
}

func TestManager_SetSummary(t *testing.T) {
	db := openTestDB(t)
	mgr, _ := Open(db)
	ctx := context.Background()

	wi, _ := mgr.Start(ctx, KindQuestion, "what is X?")
	if err := mgr.SetSummary(ctx, wi.ID, "question about X"); err != nil {
		t.Fatal(err)
	}
	got, _ := mgr.Get(ctx, wi.ID)
	if got.Summary != "question about X" {
		t.Errorf("unexpected summary %q", got.Summary)
	}
}

func TestManager_ListRecent(t *testing.T) {
	db := openTestDB(t)
	mgr, _ := Open(db)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		mgr.Start(ctx, KindTask, "item")
	}
	items, err := mgr.ListRecent(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("want 3 items, got %d", len(items))
	}
	// newest first
	if items[0].Seq < items[1].Seq {
		t.Error("expected newest first")
	}
}

func TestManager_GetNotFound(t *testing.T) {
	db := openTestDB(t)
	mgr, _ := Open(db)
	ctx := context.Background()

	got, err := mgr.Get(ctx, "task#99")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil for unknown ID, got %+v", got)
	}
}

func TestManager_SeqPersistsAcrossOpen(t *testing.T) {
	db := openTestDB(t)
	mgr1, _ := Open(db)
	ctx := context.Background()

	mgr1.Start(ctx, KindTask, "first")
	mgr1.Start(ctx, KindTask, "second")

	// Re-open with the same DB — nextSeq must resume from 3.
	mgr2, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	wi, _ := mgr2.Start(ctx, KindTask, "third")
	if !strings.HasSuffix(wi.ID, "#03") {
		t.Errorf("want seq 03 after re-open, got %q", wi.ID)
	}
}
