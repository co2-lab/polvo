package skill

import (
	"strings"
	"testing"
)

func TestParseSkills(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "NONE response yields no skills",
			input:    "NONE",
			expected: nil,
		},
		{
			name:     "empty response yields no skills",
			input:    "",
			expected: nil,
		},
		{
			name:  "single skill",
			input: "SKILL: To run tests: cd app && go test ./...",
			expected: []string{
				"To run tests: cd app && go test ./...",
			},
		},
		{
			name: "multiple skills with surrounding text",
			input: `Here are the extracted skills:
SKILL: Deploy requires AWS_PROFILE=prod
SKILL: Migration files go in app/internal/db/migrations/
Nothing else worth extracting.`,
			expected: []string{
				"Deploy requires AWS_PROFILE=prod",
				"Migration files go in app/internal/db/migrations/",
			},
		},
		{
			name:     "SKILL prefix with empty body is ignored",
			input:    "SKILL: ",
			expected: nil,
		},
		{
			name: "lines with leading/trailing whitespace are trimmed",
			input: "  SKILL: Use make build to compile  \n  SKILL: Config lives in polvo.yaml  ",
			expected: []string{
				"Use make build to compile",
				"Config lives in polvo.yaml",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseSkills(tc.input)
			if len(got) != len(tc.expected) {
				t.Fatalf("parseSkills(%q): got %d skills, want %d; skills=%v",
					tc.input, len(got), len(tc.expected), got)
			}
			for i, s := range got {
				if s != tc.expected[i] {
					t.Errorf("skill[%d]: got %q, want %q", i, s, tc.expected[i])
				}
			}
		})
	}
}

func TestLast3000(t *testing.T) {
	short := "hello world"
	if got := last3000(short); got != short {
		t.Errorf("last3000(short): got %q, want %q", got, short)
	}

	exact := strings.Repeat("a", 3000)
	if got := last3000(exact); got != exact {
		t.Errorf("last3000(exact 3000): expected unchanged")
	}

	long := strings.Repeat("b", 6000)
	got := last3000(long)
	const prefix = "...(truncated)...\n"
	if !strings.HasPrefix(got, prefix) {
		t.Errorf("last3000(6000): missing truncation prefix")
	}
	if got[len(prefix):] != long[3000:] {
		t.Errorf("last3000(6000): suffix mismatch")
	}
}

func TestBuildExtractionPrompt(t *testing.T) {
	prompt := buildExtractionPrompt("history text", "summary text", "/my/project")
	for _, want := range []string{"/my/project", "summary text", "history text", "SKILL: "} {
		if !strings.Contains(prompt, want) {
			t.Errorf("buildExtractionPrompt: expected %q in prompt", want)
		}
	}
}
