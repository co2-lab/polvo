package provider

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// retryBodyRe matches patterns like "retry after 10 seconds", "retry after 5s", "retry after 3sec".
var retryBodyRe = regexp.MustCompile(`(?i)retry.{1,20}?(\d+)\s*(s|sec|second)`)

// ParseRetryAfter extracts the server-suggested wait time from the Retry-After
// header value and/or response body text.
//
// Returns 0 if no hint is found — the caller should use exponential backoff.
func ParseRetryAfter(header string, body string) time.Duration {
	if s := strings.TrimSpace(header); s != "" {
		// Try integer seconds first.
		if secs, err := strconv.Atoi(s); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
		// Try RFC1123 / HTTP-date format.
		if t, err := http.ParseTime(s); err == nil {
			if d := time.Until(t); d > 0 {
				return d
			}
		}
	}

	// Scan body text for "retry after N seconds" patterns.
	if m := retryBodyRe.FindStringSubmatch(body); len(m) > 1 {
		if secs, err := strconv.Atoi(m[1]); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}

	return 0
}
