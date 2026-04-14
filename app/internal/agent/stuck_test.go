package agent

import (
	"crypto/md5"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestInputSig(t *testing.T) {
	// String curta (≤32 chars) → retorna a própria string
	short := "hello"
	if got := inputSig(short); got != short {
		t.Errorf("inputSig(%q) = %q; want %q", short, got, short)
	}

	// String exatamente 32 chars → retorna a própria string (boundary)
	exact32 := strings.Repeat("a", 32)
	if got := inputSig(exact32); got != exact32 {
		t.Errorf("inputSig(32-char string) = %q; want %q", got, exact32)
	}

	// String com 33 chars → retorna MD5 hex (len==32)
	s33 := strings.Repeat("b", 33)
	h := md5.Sum([]byte(s33))
	want := fmt.Sprintf("%x", h)
	if got := inputSig(s33); got != want {
		t.Errorf("inputSig(33-char string) = %q; want MD5 %q", got, want)
	}
	if len(want) != 32 {
		t.Errorf("MD5 hex should be 32 chars, got %d", len(want))
	}
}

type historyEntry struct {
	tool  string
	input string
}

func TestStuckDetector_RecordAndIsStuck(t *testing.T) {
	t.Run("sequência variada não fica stuck", func(t *testing.T) {
		det := NewStuckDetector(6, 3)
		entries := []historyEntry{
			{"read", "file1.go"},
			{"write", "file2.go"},
			{"grep", "pattern"},
			{"read", "file3.go"},
		}
		for _, e := range entries {
			det.Record(e.tool, e.input)
		}
		if det.IsStuck() {
			t.Error("expected IsStuck=false for varied sequence")
		}
	})

	t.Run("mesma tool+input repetida threshold vezes → stuck", func(t *testing.T) {
		det := NewStuckDetector(6, 3)
		for i := 0; i < 3; i++ {
			det.Record("read", "file.go")
		}
		if !det.IsStuck() {
			t.Error("expected IsStuck=true after threshold repetitions")
		}
	})

	t.Run("mesma tool+input threshold-1 vezes → não stuck", func(t *testing.T) {
		det := NewStuckDetector(6, 3)
		for i := 0; i < 2; i++ {
			det.Record("read", "file.go")
		}
		if det.IsStuck() {
			t.Error("expected IsStuck=false for threshold-1 repetitions")
		}
	})

	t.Run("repetição fora da janela não conta", func(t *testing.T) {
		// windowSize=4, threshold=3
		// Preenche janela com 4 chamadas distintas, depois coloca 2 repetições
		det := NewStuckDetector(4, 3)
		// As 3 primeiras ficam fora da janela ao adicionar mais 4 distintas
		det.Record("read", "file.go")
		det.Record("read", "file.go")
		det.Record("read", "file.go")
		// Agora adicionamos 4 distintas para empurrar as 3 para fora da janela
		det.Record("write", "a.go")
		det.Record("grep", "pat")
		det.Record("ls", ".")
		det.Record("glob", "*.go")
		// Janela de 4 agora contém: write, grep, ls, glob — sem repetição
		if det.IsStuck() {
			t.Error("expected IsStuck=false when repetition is outside window")
		}
	})

	t.Run("windowSize=0 usa default 6", func(t *testing.T) {
		det := NewStuckDetector(0, 3)
		if det.WindowSize != 6 {
			t.Errorf("expected WindowSize=6, got %d", det.WindowSize)
		}
	})

	t.Run("threshold=0 usa default 3", func(t *testing.T) {
		det := NewStuckDetector(6, 0)
		if det.Threshold != 3 {
			t.Errorf("expected Threshold=3, got %d", det.Threshold)
		}
	})

	t.Run("reset implícito: após stuck, novas distintas → não stuck", func(t *testing.T) {
		det := NewStuckDetector(6, 3)
		// Primeiro, causa stuck
		for i := 0; i < 3; i++ {
			det.Record("read", "file.go")
		}
		if !det.IsStuck() {
			t.Error("should be stuck before reset")
		}
		// Adiciona novas chamadas distintas — a janela agora é composta por
		// dados mistos que não têm 3 repetições da MESMA chave
		// (deve sair do estado stuck via sliding window)
		det.Record("write", "a.go")
		det.Record("grep", "pat")
		det.Record("ls", ".")
		det.Record("glob", "*.go")
		// Agora a janela de 6 contém: read+file.go (2x), write, grep, ls, glob
		// — apenas 2 repetições de read:file.go → não stuck
		if det.IsStuck() {
			t.Error("expected IsStuck=false after distinct calls reset the window")
		}
	})

	t.Run("monologue: mesma tool think repetida → stuck", func(t *testing.T) {
		det := NewStuckDetector(6, 3)
		for i := 0; i < 3; i++ {
			det.Record("think", "same thought")
		}
		if !det.IsStuck() {
			t.Error("expected IsStuck=true for repeated think tool (monologue)")
		}
	})
}

func TestStuckDetector_KeyFormat(t *testing.T) {
	// tool="read" e tool="write" com mesmo input → chaves diferentes (não conflate)
	det := NewStuckDetector(6, 3)
	// Registra 3x "read" com input "file.go"
	for i := 0; i < 3; i++ {
		det.Record("read", "file.go")
	}
	if !det.IsStuck() {
		t.Error("expected IsStuck=true for read tool repeated 3 times")
	}

	// Novo detector: mistura read e write com mesmo input — as chaves são diferentes,
	// portanto nem read:file.go nem write:file.go atingem threshold=3 individualmente.
	// Verifica que tool+input são a chave composta (não apenas input).
	det2 := NewStuckDetector(6, 3)
	// 2x read + 2x write = 4 entradas, mas cada chave tem apenas 2 ocorrências → não stuck
	det2.Record("read", "file.go")
	det2.Record("write", "file.go")
	det2.Record("read", "file.go")
	det2.Record("write", "file.go")
	if det2.IsStuck() {
		t.Error("expected IsStuck=false: read:file.go e write:file.go são chaves distintas (2 cada, threshold=3)")
	}

	// Confirma que o mesmo input com a MESMA tool atinge o threshold
	det3 := NewStuckDetector(6, 3)
	det3.Record("read", "file.go")
	det3.Record("read", "file.go")
	det3.Record("read", "file.go")
	if !det3.IsStuck() {
		t.Error("expected IsStuck=true when same tool+input repeated threshold times")
	}
}

func TestStuckDetector_LargeInput(t *testing.T) {
	largeInput := strings.Repeat("x", 100) // >32 bytes → usa MD5

	t.Run("dois inputs idênticos grandes → stuck após threshold", func(t *testing.T) {
		det := NewStuckDetector(6, 3)
		for i := 0; i < 3; i++ {
			det.Record("read", largeInput)
		}
		if !det.IsStuck() {
			t.Error("expected IsStuck=true for identical large inputs")
		}
	})

	t.Run("dois inputs grandes diferentes → não stuck", func(t *testing.T) {
		largeInput2 := strings.Repeat("y", 100)
		det := NewStuckDetector(6, 3)
		det.Record("read", largeInput)
		det.Record("read", largeInput)
		det.Record("read", largeInput2) // diferente
		if det.IsStuck() {
			t.Error("expected IsStuck=false for different large inputs")
		}
	})
}

// ---- New gap-coverage tests -------------------------------------------------

// TestStuckDetector_ErrorObservationLoop checks behavior when the same tool+args
// are called repeatedly but each call produces an error result.
// GAP: error loop not distinguished from successful loop — the StuckDetector only
// looks at tool+input keys, not at result content. The detector DOES fire after
// threshold repetitions regardless of whether results are errors or successes.
func TestStuckDetector_ErrorObservationLoop(t *testing.T) {
	// Five entries with the same tool+args (the result is "error" in real usage,
	// but StuckDetector only sees the call signature, not the result).
	det := NewStuckDetector(6, 3)
	for i := 0; i < 5; i++ {
		det.Record("bash", `{"cmd":"failing command"}`)
	}
	// Current behavior: fires as stuck because tool+input repeated >= threshold.
	if !det.IsStuck() {
		t.Error("expected IsStuck=true: same tool+input repeated 5 times (error loop scenario)")
	}
}

// TestStuckDetector_ContextWindowErrorLoop checks behavior when calls carry a
// context-window-error payload in their input string.
// GAP: context window error patterns in content are not detected — StuckDetector
// only hashes tool+input keys. However, if the same tool is called with identical
// input threshold+ times it will still trigger.
func TestStuckDetector_ContextWindowErrorLoop(t *testing.T) {
	// Simulate 4 calls where the input resembles a context-length-exceeded scenario.
	det := NewStuckDetector(6, 4)
	for i := 0; i < 4; i++ {
		det.Record("chat", `{"error":"context length exceeded"}`)
	}
	// Current behavior: fires as stuck after threshold repetitions (4).
	if !det.IsStuck() {
		t.Error("expected IsStuck=true: same tool+input repeated 4 times (context window error loop)")
	}
}

// TestStuckDetector_CyclicPatternAB tests an alternating A,B,A,B,A pattern.
// GAP: cyclic action patterns not detected — StuckDetector only counts individual
// key occurrences within the window; it does not detect alternating or cyclic
// sequences. With windowSize=6 and threshold=3, "A" appears 3 times so it fires.
func TestStuckDetector_CyclicPatternAB(t *testing.T) {
	// Pattern: A, B, A, B, A — 5 entries, A appears 3 times, B appears 2 times.
	det := NewStuckDetector(6, 3)
	det.Record("read", "file-a.go") // A
	det.Record("grep", "pattern")   // B
	det.Record("read", "file-a.go") // A
	det.Record("grep", "pattern")   // B
	det.Record("read", "file-a.go") // A

	// Current behavior: "read:file-a.go" count=3 hits threshold=3 → stuck=true.
	// Note: a future cyclic-pattern detector might fire earlier on the AB pattern,
	// but the current implementation only fires when one key reaches threshold.
	if !det.IsStuck() {
		// Document current behavior: cyclic AB pattern fires stuck only because
		// A repeated threshold times, not because the cycle was detected.
		t.Log("GAP: cyclic action patterns not detected as a cycle; IsStuck=false here means A never hit threshold")
		t.Error("expected IsStuck=true: 'read:file-a.go' repeated 3 times within window of 6")
	}
}

// TestStuckDetector_NoObservationsBetweenRepeats ensures that when the same
// tool+args appear threshold-1 times but different calls are interspersed
// (pushing the older repeats outside the sliding window), stuck is NOT triggered.
func TestStuckDetector_NoObservationsBetweenRepeats(t *testing.T) {
	// windowSize=6, threshold=3
	// First add 2 identical calls, then fill the remaining 4 slots with distinct
	// calls so the 2 identical calls fall outside the window after more entries.
	det := NewStuckDetector(6, 3)
	det.Record("read", "file.go") // slot 1
	det.Record("read", "file.go") // slot 2 — only 2 so far, below threshold
	// Now add 6 distinct calls to push both "read:file.go" entries out of the window.
	det.Record("grep", "pat1")
	det.Record("grep", "pat2")
	det.Record("ls", "dir1")
	det.Record("glob", "*.go")
	det.Record("think", "step1")
	det.Record("write", "out.go")
	// The sliding window of the last 6 entries contains no "read:file.go" — not stuck.
	if det.IsStuck() {
		t.Error("expected IsStuck=false: 'read:file.go' was pushed outside the sliding window")
	}

	// Sanity check: if we now add a third "read:file.go" inside the window, it's
	// still only 1 occurrence within the current window → not stuck.
	det.Record("read", "file.go")
	if det.IsStuck() {
		t.Error("expected IsStuck=false: only 1 occurrence of 'read:file.go' in the current window")
	}
}

func TestStuckDetectorConcurrent(t *testing.T) {
	det := NewStuckDetector(6, 3)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			det.Record(fmt.Sprintf("tool-%d", n), "input")
		}(i)
	}
	wg.Wait()
	// StuckDetector is protected by sync.Mutex; concurrent Record and IsStuck
	// calls must not race under -race.
	_ = det.IsStuck()
}
