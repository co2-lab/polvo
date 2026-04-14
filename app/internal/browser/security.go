package browser

import (
	"html"
	"net/url"
	"regexp"
	"strings"
	"sync"
)

// urlRegex matches http/https URLs in text.
var urlRegex = regexp.MustCompile(`https?://[^\s"'<>]+`)

// URLAllowlist tracks domains that have appeared in the conversation.
// The browser tool may only navigate to allowed domains.
type URLAllowlist struct {
	mu      sync.RWMutex
	allowed map[string]bool // normalized hostname → true
}

// NewURLAllowlist creates a new allowlist pre-seeded with extraDomains.
func NewURLAllowlist(extraDomains []string) *URLAllowlist {
	a := &URLAllowlist{allowed: make(map[string]bool)}
	for _, d := range extraDomains {
		d = strings.TrimSpace(strings.ToLower(d))
		if d != "" {
			a.allowed[d] = true
		}
	}
	return a
}

// AddFromText extracts URLs from text and adds their hostnames to the allowlist.
func (a *URLAllowlist) AddFromText(text string) {
	matches := urlRegex.FindAllString(text, -1)
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, raw := range matches {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			continue
		}
		host := normalizeHost(u.Host)
		a.allowed[host] = true
	}
}

// IsAllowed reports whether the given rawURL may be navigated to.
// localhost (and 127.0.0.1) are always allowed.
func (a *URLAllowlist) IsAllowed(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return false
	}
	host := normalizeHost(u.Host)
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.allowed[host]
}

// normalizeHost strips the port and lowercases the hostname.
func normalizeHost(host string) string {
	// Strip port if present.
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		// IPv6 addresses contain colons; only strip if not inside brackets.
		if !strings.Contains(host, "[") {
			host = host[:idx]
		}
	}
	return strings.ToLower(host)
}

// injectionPatterns lists strings that indicate prompt injection attempts.
var injectionPatterns = []string{
	"ignore previous instructions",
	"ignore all previous",
	"new instructions:",
	"system prompt:",
	"you are now",
	"forget everything",
	"disregard all",
}

// DetectPromptInjection checks whether content contains known injection patterns.
// Returns (detected, matchedPattern). The check is case-insensitive and also
// detects HTML-encoded payloads by decoding entities before matching.
func DetectPromptInjection(content string) (bool, string) {
	decoded := html.UnescapeString(content)
	for _, candidate := range []string{content, decoded} {
		lower := strings.ToLower(candidate)
		for _, pattern := range injectionPatterns {
			if strings.Contains(lower, pattern) {
				return true, pattern
			}
		}
	}
	return false, ""
}
