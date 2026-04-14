// Package secrets provides secret detection and masking utilities.
package secrets

import (
	"math"
	"regexp"
	"strings"
)

// secretPattern is a compiled regex plus its replacement label.
type secretPattern struct {
	re    *regexp.Regexp
	label string
}

var secretPatterns = []secretPattern{
	// Specific token formats first, before the generic token= pattern.
	{regexp.MustCompile(`sk-[A-Za-z0-9]{48}`), "[OPENAI_KEY_REDACTED]"},
	{regexp.MustCompile(`ghp_[A-Za-z0-9]{36}`), "[GITHUB_PAT_REDACTED]"},
	{regexp.MustCompile(`gho_[A-Za-z0-9]{36}`), "[GITHUB_OAUTH_REDACTED]"},
	{regexp.MustCompile(`xoxb-[0-9]+-[A-Za-z0-9]+`), "[SLACK_TOKEN_REDACTED]"},
	{regexp.MustCompile(`AKIA[0-9A-Z]{16}`), "[AWS_ACCESS_KEY_REDACTED]"},
	// New patterns (Plan 45) — more specific first.
	{regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`), "[JWT_REDACTED]"},
	{regexp.MustCompile(`-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----`), "[PRIVATE_KEY_REDACTED]"},
	{regexp.MustCompile(`(?:mongodb|postgres|mysql|redis)://[^@\s]+:[^@\s]+@[^\s]+`), "[DB_CONN_REDACTED]"},
	{regexp.MustCompile(`[Bb]earer\s+[A-Za-z0-9\-._~+/]{20,}`), "[BEARER_TOKEN_REDACTED]"},
	{regexp.MustCompile(`sk_live_[A-Za-z0-9]{24,}`), "[STRIPE_KEY_REDACTED]"},
	{regexp.MustCompile(`SG\.[A-Za-z0-9_-]{22,}\.[A-Za-z0-9_-]{43,}`), "[SENDGRID_KEY_REDACTED]"},
	{regexp.MustCompile(`(?i)(?:secret|private_key)\s*[=:]\s*\S{8,}`), "[SECRET_REDACTED]"},
	// Key/value assignment patterns.
	{regexp.MustCompile(`(?i)(aws_access_key_id|aws_secret_access_key)\s*[=:]\s*\S+`), "[AWS_KEY_REDACTED]"},
	{regexp.MustCompile(`(?i)(api[_-]?key|apikey|api[_-]?secret)\s*[=:]\s*\S+`), "[API_KEY_REDACTED]"},
	{regexp.MustCompile(`(?i)(token|bearer)\s*[=:]\s*[A-Za-z0-9._\-]{20,}`), "[TOKEN_REDACTED]"},
	{regexp.MustCompile(`(?i)authorization\s*[=:]\s*\S+`), "[AUTH_REDACTED]"},
	{regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[=:]\s*\S+`), "[PASSWORD_REDACTED]"},
}

// MaskResult holds the masked content and details of what was redacted.
type MaskResult struct {
	Masked     string
	Redactions []Redaction
}

// Redaction records a single secret that was masked.
type Redaction struct {
	Pattern string // rule name or "entropy_hex"/"entropy_base64"/"entropy_alphanum"
	Offset  int    // byte offset in the masked string
	Length  int    // length of the replacement label
	Label   string // replacement label used
}

// MaskSecrets replaces detected secrets in content with redaction labels.
// Returns the masked string and a count of replacements made.
// This is the backward-compatible API.
func MaskSecrets(content string) (masked string, count int) {
	result := MaskSecretsDetailed(content)
	return result.Masked, len(result.Redactions)
}

// MaskSecretsDetailed runs the full masking pipeline (regex then entropy) and
// returns both the masked content and details of every redaction.
func MaskSecretsDetailed(content string) MaskResult {
	var redactions []Redaction
	masked := content

	// Pass 1: regex patterns.
	for _, p := range secretPatterns {
		locs := p.re.FindAllStringIndex(masked, -1)
		if len(locs) == 0 {
			continue
		}
		// Replace right-to-left to preserve offsets of earlier matches.
		for i := len(locs) - 1; i >= 0; i-- {
			loc := locs[i]
			redactions = append(redactions, Redaction{
				Pattern: p.label,
				Offset:  loc[0],
				Length:  len(p.label),
				Label:   p.label,
			})
			masked = masked[:loc[0]] + p.label + masked[loc[1]:]
		}
	}

	// Pass 2: entropy scan on the regex-masked result.
	entropyMasked, entropyRedactions := maskHighEntropy(masked)
	masked = entropyMasked
	redactions = append(redactions, entropyRedactions...)

	return MaskResult{Masked: masked, Redactions: redactions}
}

// ShannonEntropy computes the Shannon entropy of s in bits per character.
// Uses a fixed-size frequency array — no heap allocation.
func ShannonEntropy(s string) float64 {
	if len(s) == 0 {
		return 0
	}
	var freq [256]int
	for i := 0; i < len(s); i++ {
		freq[s[i]]++
	}
	n := float64(len(s))
	var h float64
	for _, f := range freq {
		if f == 0 {
			continue
		}
		p := float64(f) / n
		h -= p * math.Log2(p)
	}
	return h
}

// classifyCharset returns "hex", "base64", or "alphanum" based on the
// characters in s, or "" if s does not match any high-entropy charset.
func classifyCharset(s string) string {
	if len(s) == 0 {
		return ""
	}
	isHex := true
	isBase64 := true
	hasLower := false
	hasUpper := false
	hasDigit := false

	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			isHex = false
		}
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '+' || c == '/' || c == '=') {
			isBase64 = false
		}
		if c >= 'a' && c <= 'z' {
			hasLower = true
		}
		if c >= 'A' && c <= 'Z' {
			hasUpper = true
		}
		if c >= '0' && c <= '9' {
			hasDigit = true
		}
	}

	if isHex {
		return "hex"
	}
	if isBase64 {
		return "base64"
	}
	// Alphanumeric mixed-case: letters + digits, both upper and lower present.
	alphanumOnly := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			alphanumOnly = false
			break
		}
	}
	if alphanumOnly && hasLower && hasUpper && hasDigit {
		return "alphanum"
	}
	return ""
}

// isHighEntropySecret returns true if token appears to be a high-entropy secret
// based on its character set and Shannon entropy.
func isHighEntropySecret(token string) bool {
	charset := classifyCharset(token)
	if charset == "" {
		return false
	}
	h := ShannonEntropy(token)
	switch charset {
	case "hex":
		return len(token) >= 20 && h >= 3.5
	case "base64":
		return len(token) >= 20 && h >= 4.5
	case "alphanum":
		return len(token) >= 30 && h >= 4.0
	}
	return false
}

// separators used to split content into tokens for entropy scanning.
const separators = "\t\n |;=\"'{},:'`()"

// indexedToken is a token with its byte offset in the original string.
type indexedToken struct {
	value  string
	offset int
}

// tokenize splits s into tokens on separator characters, tracking offsets.
func tokenize(s string) []indexedToken {
	var tokens []indexedToken
	start := -1
	for i := 0; i < len(s); i++ {
		if strings.ContainsRune(separators, rune(s[i])) {
			if start >= 0 {
				tokens = append(tokens, indexedToken{value: s[start:i], offset: start})
				start = -1
			}
		} else {
			if start < 0 {
				start = i
			}
		}
	}
	if start >= 0 {
		tokens = append(tokens, indexedToken{value: s[start:], offset: start})
	}
	return tokens
}

// maskHighEntropy scans s for high-entropy tokens and replaces them.
// Replacement is done right-to-left to preserve earlier offsets.
func maskHighEntropy(s string) (string, []Redaction) {
	tokens := tokenize(s)
	type match struct {
		start int
		end   int
		label string
	}
	var matches []match

	for _, tok := range tokens {
		if !isHighEntropySecret(tok.value) {
			continue
		}
		var label string
		switch classifyCharset(tok.value) {
		case "hex":
			label = "[HEX_SECRET_REDACTED]"
		case "base64":
			label = "[B64_SECRET_REDACTED]"
		case "alphanum":
			label = "[SECRET_REDACTED]"
		default:
			label = "[SECRET_REDACTED]"
		}
		matches = append(matches, match{
			start: tok.offset,
			end:   tok.offset + len(tok.value),
			label: label,
		})
	}

	if len(matches) == 0 {
		return s, nil
	}

	var redactions []Redaction
	result := s
	// Replace right-to-left to preserve offsets.
	for i := len(matches) - 1; i >= 0; i-- {
		m := matches[i]
		redactions = append(redactions, Redaction{
			Pattern: "entropy_" + classifyCharset(s[m.start:m.end]),
			Offset:  m.start,
			Length:  len(m.label),
			Label:   m.label,
		})
		result = result[:m.start] + m.label + result[m.end:]
	}
	return result, redactions
}
