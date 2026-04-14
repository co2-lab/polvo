package provider

import (
	"errors"
	"testing"
)

func TestIsContextWindowError(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		want    bool
	}{
		{"nil error", nil, false},
		{"unrelated error", errors.New("connection refused"), false},
		{"anthropic contextwindowexceedederror", errors.New("ContextWindowExceededError: too many tokens"), true},
		{"openai context_length_exceeded", errors.New("This model's maximum context length is 4097 tokens. context_length_exceeded"), true},
		{"openai maximum context length", errors.New("maximum context length is 8192 tokens"), true},
		{"openai reduce the length", errors.New("Please reduce the length of the messages"), true},
		{"anthropic prompt is too long", errors.New("prompt is too long"), true},
		{"generic token limit", errors.New("token limit exceeded for this request"), true},
		{"input length exceed", errors.New("input length and `max_tokens` exceed context limit"), true},
		{"rate limit (not context)", errors.New("rate limit exceeded"), false},
		{"case insensitive check", errors.New("MAXIMUM CONTEXT LENGTH exceeded"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isContextWindowError(tc.err)
			if got != tc.want {
				t.Errorf("isContextWindowError(%v) = %v; want %v", tc.err, got, tc.want)
			}
		})
	}
}
