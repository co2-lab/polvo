package tool

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// TestSecurePath
// ---------------------------------------------------------------------------

func TestSecurePath(t *testing.T) {
	wd := t.TempDir()

	cases := []struct {
		name    string
		path    string
		wantErr bool
		note    string
	}{
		// Casos válidos — padrão cyphar/filepath-securejoin join_test.go
		{"relativo_valido", "foo/bar.go", false, ""},
		{"relativo_raiz_do_workdir", ".", false, "resolve para workdir"},
		{"absoluto_dentro_do_workdir", filepath.Join(wd, "foo.go"), false, ""},
		{"componente_unico_valido", "main.go", false, ""},
		{"subdir_valido", "internal/tool/bash.go", false, ""},
		// Casos de traversal — tabela canônica de path traversal
		{"traversal_relativo", "../../etc/passwd", true, "path traversal clássico"},
		{"absoluto_fora_do_workdir", "/etc/passwd", true, "path absoluto externo"},
		{"absoluto_raiz", "/", true, "raiz do sistema"},
		{"traversal_mascarado", "foo/../../etc/passwd", true, "traversal dentro de subpath"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := securePath(wd, tc.path)
			if (err != nil) != tc.wantErr {
				t.Errorf("securePath(%q, %q) error=%v, wantErr=%v — %s",
					wd, tc.path, err, tc.wantErr, tc.note)
			}
		})
	}
}

// TestSecurePath_WorkdirTrailingSlash verifica que workdir com trailing slash
// produz o mesmo resultado que sem trailing slash.
func TestSecurePath_WorkdirTrailingSlash(t *testing.T) {
	wd := t.TempDir()
	wdSlash := wd + "/"

	res1, err1 := securePath(wd, "foo/bar.go")
	res2, err2 := securePath(wdSlash, "foo/bar.go")

	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected error: err1=%v err2=%v", err1, err2)
	}
	if res1 != res2 {
		t.Errorf("trailing slash produces different result: %q vs %q", res1, res2)
	}
}

// TestSecurePath_SymlinkEscape verifies that a symlink inside the workdir
// pointing to a path outside the workdir is rejected by securePath.
func TestSecurePath_SymlinkEscape(t *testing.T) {
	wd := t.TempDir()

	// Create a symlink inside wd pointing to /etc/passwd (outside wd).
	symlinkPath := filepath.Join(wd, "escape_link")
	if err := os.Symlink("/etc/passwd", symlinkPath); err != nil {
		t.Skipf("cannot create symlink (may require privilege): %v", err)
	}

	_, err := securePath(wd, "escape_link")
	if err == nil {
		t.Errorf("securePath should have rejected symlink pointing outside workdir, but returned no error")
	}
}

// ---------------------------------------------------------------------------
// TestCheckIgnored
// ---------------------------------------------------------------------------

// ignorerFunc é um mock inline sem dependência de biblioteca de mock.
type ignorerFunc func(string) bool

func (f ignorerFunc) Ignored(p string) bool { return f(p) }

func TestCheckIgnored(t *testing.T) {
	t.Run("nil_ignorer_sempre_permite", func(t *testing.T) {
		err := checkIgnored(nil, "/qualquer/caminho")
		if err != nil {
			t.Errorf("expected nil error with nil ignorer, got %v", err)
		}
	})

	t.Run("ignorado_retorna_erro", func(t *testing.T) {
		ig := ignorerFunc(func(string) bool { return true })
		err := checkIgnored(ig, "/projeto/secrets.env")
		if err == nil {
			t.Error("expected error for ignored path")
		}
	})

	t.Run("nao_ignorado_retorna_nil", func(t *testing.T) {
		ig := ignorerFunc(func(string) bool { return false })
		err := checkIgnored(ig, "/projeto/main.go")
		if err != nil {
			t.Errorf("expected nil error for non-ignored path, got %v", err)
		}
	})
}

// TestPolvoIgnoreNotEnforcedInLsGlobGrep documenta o gap conhecido:
// ls, glob e grep não chamam checkIgnored, tornando .polvoignore ineficaz para listagens.
func TestPolvoIgnoreNotEnforcedInLsGlobGrep(t *testing.T) {
	// Documenta o gap: paths em .polvoignore são visíveis via ls, glob, grep.
	// checkIgnored existe em path.go mas não é chamado nesses tools.
	t.Log("GAP: ls, glob, grep não chamam checkIgnored — .polvoignore não protege contra listagem")
}
