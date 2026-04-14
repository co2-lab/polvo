package tool

import (
	"strings"
	"testing"
)

func TestLsTool(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		wantErr     bool
		wantContain string
		errContains string
	}{
		{
			name:        "existing dir",
			path:        ".",
			wantErr:     false,
			wantContain: "main.go",
		},
		{
			name:        "subdir",
			path:        "sub",
			wantErr:     false,
			wantContain: "util.go",
		},
		{
			name:        "nonexistent dir",
			path:        "notexist",
			wantErr:     true,
			wantContain: "",
			errContains: "reading directory",
		},
		{
			name:        "path traversal",
			path:        "../../",
			wantErr:     true,
			wantContain: "",
			errContains: "escapes",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := testSetup(t)
			tool := NewLS(dir)

			res := execTool(t, tool, map[string]any{"path": tc.path})

			if tc.wantErr {
				if !res.IsError {
					t.Fatalf("expected error result, got success: %s", res.Content)
				}
				if tc.errContains != "" && !strings.Contains(res.Content, tc.errContains) {
					t.Errorf("error %q does not contain %q", res.Content, tc.errContains)
				}
			} else {
				assertSuccess(t, res)
				if tc.wantContain != "" && !strings.Contains(res.Content, tc.wantContain) {
					t.Errorf("expected %q in output, got: %q", tc.wantContain, res.Content)
				}
			}
		})
	}
}
