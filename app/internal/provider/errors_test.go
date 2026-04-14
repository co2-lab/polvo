package provider

import (
	"testing"
)

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		message    string
		wantKind   ErrorKind
		wantRetry  bool
	}{
		{"rate limit 429", 429, "", ErrKindRateLimit, true},
		{"overloaded 529", 529, "", ErrKindOverloaded, true},
		{"auth 401", 401, "", ErrKindAuth, false},
		{"auth 403", 403, "", ErrKindAuth, false},
		{"server 500", 500, "", ErrKindServer, true},
		{"server 502", 502, "", ErrKindServer, true},
		{"server 503", 503, "", ErrKindServer, true},
		{"server 504", 504, "", ErrKindServer, true},
		{"context length 400", 400, "context length exceeded", ErrKindContextTooLong, false},
		{"context window 400", 400, "maximum context window size", ErrKindContextTooLong, false},
		{"generic 400", 400, "invalid request body", ErrKindInvalidRequest, false},
		{"rate limit in message", 0, "you have hit the rate limit", ErrKindRateLimit, true},
		{"overloaded in message", 0, "model is overloaded", ErrKindOverloaded, true},
		{"context length in message", 0, "context length exceeded", ErrKindContextTooLong, false},
		{"unknown", 0, "something went wrong", ErrKindUnknown, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyError(tc.status, tc.message)
			if got != tc.wantKind {
				t.Errorf("ClassifyError(%d, %q) = %v; want %v", tc.status, tc.message, got, tc.wantKind)
			}
			if got.IsRetriable() != tc.wantRetry {
				t.Errorf("ClassifyError(%d, %q).IsRetriable() = %v; want %v", tc.status, tc.message, got.IsRetriable(), tc.wantRetry)
			}
		})
	}
}

func TestProviderError(t *testing.T) {
	pe := NewProviderError(429, "rate limited", 0)
	if pe.Kind != ErrKindRateLimit {
		t.Errorf("expected ErrKindRateLimit, got %v", pe.Kind)
	}
	if !pe.Kind.IsRetriable() {
		t.Error("expected retriable")
	}
	if pe.Error() == "" {
		t.Error("expected non-empty error message")
	}
}
