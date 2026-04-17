package tool

import (
	"strings"
	"testing"
)

func TestTruncateObservation(t *testing.T) {
	t.Run("conteúdo dentro do limite retorna inalterado", func(t *testing.T) {
		content := strings.Repeat("a", 100)
		got := TruncateObservation(content, 200)
		if got != content {
			t.Errorf("expected unchanged content, got different")
		}
	})

	t.Run("conteúdo exatamente no limite retorna inalterado", func(t *testing.T) {
		content := strings.Repeat("a", 200)
		got := TruncateObservation(content, 200)
		if got != content {
			t.Errorf("expected unchanged content at exact limit")
		}
	})

	t.Run("conteúdo acima do limite tem marcador head+tail", func(t *testing.T) {
		content := strings.Repeat("a", 100) + strings.Repeat("b", 100)
		got := TruncateObservation(content, 100)
		if !strings.Contains(got, "truncados") {
			t.Error("expected truncation marker in output")
		}
		// Head: first 50 chars should be 'a'
		if !strings.HasPrefix(got, strings.Repeat("a", 50)) {
			t.Error("expected head to start with 'a's")
		}
		// Tail: last 50 chars should be 'b'
		if !strings.HasSuffix(got, strings.Repeat("b", 50)) {
			t.Error("expected tail to end with 'b's")
		}
	})

	t.Run("marcador contém número correto de chars removidos", func(t *testing.T) {
		content := strings.Repeat("x", 1000)
		got := TruncateObservation(content, 100)
		// 1000 - 100 = 900 chars removed
		if !strings.Contains(got, "900 chars truncados") {
			t.Errorf("expected marker to contain '900 chars truncados', got: %q", got)
		}
	})

	t.Run("maxChars=0 usa DefaultMaxObservationChars", func(t *testing.T) {
		// Content smaller than default should not be truncated
		content := strings.Repeat("x", 100)
		got := TruncateObservation(content, 0)
		if got != content {
			t.Error("content under default limit should be unchanged")
		}
	})

	t.Run("maxChars negativo usa DefaultMaxObservationChars", func(t *testing.T) {
		content := strings.Repeat("x", 100)
		got := TruncateObservation(content, -1)
		if got != content {
			t.Error("content under default limit should be unchanged with negative maxChars")
		}
	})

	t.Run("tamanho final não excede maxChars + tamanho do marcador", func(t *testing.T) {
		content := strings.Repeat("z", 10_000)
		maxChars := 500
		got := TruncateObservation(content, maxChars)
		// Result should be head(250) + marker + tail(250)
		// Marker is small (~30 chars), so total should be well under 600
		if len(got) > maxChars+100 {
			t.Errorf("result too large: %d chars (max %d + marker overhead)", len(got), maxChars)
		}
	})

	t.Run("conteúdo vazio retorna vazio", func(t *testing.T) {
		got := TruncateObservation("", 100)
		if got != "" {
			t.Errorf("expected empty string, got: %q", got)
		}
	})
}
