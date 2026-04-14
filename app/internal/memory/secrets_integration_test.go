package memory

import (
	"strings"
	"testing"
)

func TestStore_MasksSecretsOnWrite(t *testing.T) {
	store := openStore(t)

	t.Run("API key is masked before persisting", func(t *testing.T) {
		raw := "api_key=supersecretvalue123"
		if err := store.Write(Entry{Agent: "ag", Type: "observation", Content: raw}); err != nil {
			t.Fatalf("Write: %v", err)
		}
		entries, err := store.Read(Filter{Agent: "ag"})
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if len(entries) == 0 {
			t.Fatal("expected 1 entry")
		}
		stored := entries[0].Content
		if strings.Contains(stored, "supersecretvalue123") {
			t.Errorf("API key not masked in stored content: %q", stored)
		}
		if !strings.Contains(stored, "[API_KEY_REDACTED]") {
			t.Errorf("expected [API_KEY_REDACTED] in stored content: %q", stored)
		}
	})

	t.Run("password is masked before persisting", func(t *testing.T) {
		store2 := openStore(t)
		raw := "password=hunter2"
		if err := store2.Write(Entry{Agent: "ag", Type: "observation", Content: raw}); err != nil {
			t.Fatalf("Write: %v", err)
		}
		entries, err := store2.Read(Filter{Agent: "ag"})
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if len(entries) == 0 {
			t.Fatal("expected 1 entry")
		}
		stored := entries[0].Content
		if strings.Contains(stored, "hunter2") {
			t.Errorf("password not masked in stored content: %q", stored)
		}
		if !strings.Contains(stored, "[PASSWORD_REDACTED]") {
			t.Errorf("expected [PASSWORD_REDACTED] in stored content: %q", stored)
		}
	})

	t.Run("normal content is stored unchanged", func(t *testing.T) {
		store3 := openStore(t)
		raw := "This is a normal observation with no secrets."
		if err := store3.Write(Entry{Agent: "ag", Type: "observation", Content: raw}); err != nil {
			t.Fatalf("Write: %v", err)
		}
		entries, err := store3.Read(Filter{Agent: "ag"})
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if len(entries) == 0 {
			t.Fatal("expected 1 entry")
		}
		if entries[0].Content != raw {
			t.Errorf("normal content changed: got %q, want %q", entries[0].Content, raw)
		}
	})

	t.Run("GitHub PAT is masked before persisting", func(t *testing.T) {
		store4 := openStore(t)
		pat := "ghp_" + strings.Repeat("Z", 36)
		raw := "cloning with token " + pat
		if err := store4.Write(Entry{Agent: "ag", Type: "observation", Content: raw}); err != nil {
			t.Fatalf("Write: %v", err)
		}
		entries, err := store4.Read(Filter{Agent: "ag"})
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if len(entries) == 0 {
			t.Fatal("expected 1 entry")
		}
		stored := entries[0].Content
		if strings.Contains(stored, pat) {
			t.Errorf("GitHub PAT not masked in stored content: %q", stored)
		}
		if !strings.Contains(stored, "[GITHUB_PAT_REDACTED]") {
			t.Errorf("expected [GITHUB_PAT_REDACTED] in stored content: %q", stored)
		}
	})
}
