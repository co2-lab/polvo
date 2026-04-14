package provider

import (
	"testing"
	"time"
)

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name   string
		header string
		body   string
		want   time.Duration
	}{
		{
			name:   "integer header",
			header: "10",
			want:   10 * time.Second,
		},
		{
			name:   "integer header with whitespace",
			header: "  30  ",
			want:   30 * time.Second,
		},
		{
			name:   "body retry after N seconds",
			body:   "rate limit exceeded, retry after 5 seconds",
			want:   5 * time.Second,
		},
		{
			name:   "body retry after Ns abbreviated",
			body:   "please retry after 15s",
			want:   15 * time.Second,
		},
		{
			name:   "body retry after N sec",
			body:   "retry after 8 sec",
			want:   8 * time.Second,
		},
		{
			name:   "header takes precedence over body",
			header: "20",
			body:   "retry after 5 seconds",
			want:   20 * time.Second,
		},
		{
			name: "no hint",
			body: "internal server error",
			want: 0,
		},
		{
			name:   "zero header value",
			header: "0",
			want:   0,
		},
		{
			name:   "invalid header falls through to body",
			header: "not-a-number",
			body:   "retry after 3 seconds",
			want:   3 * time.Second,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseRetryAfter(tc.header, tc.body)
			if got != tc.want {
				t.Errorf("ParseRetryAfter(%q, %q) = %v; want %v", tc.header, tc.body, got, tc.want)
			}
		})
	}
}
