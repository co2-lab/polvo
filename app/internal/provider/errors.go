package provider

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrorKind categorizes API errors to determine the recovery strategy.
type ErrorKind int

const (
	ErrKindUnknown        ErrorKind = iota
	ErrKindRateLimit                // HTTP 429 — wait and retry
	ErrKindOverloaded               // HTTP 529 / "overloaded" — wait and retry
	ErrKindContextTooLong           // HTTP 400 "context length" — do not retry; trigger condenser
	ErrKindInvalidRequest           // HTTP 400 generic — do not retry
	ErrKindAuth                     // HTTP 401/403 — fatal, do not retry
	ErrKindServer                   // HTTP 500/502/503/504 — retry with backoff
)

// ProviderError is a provider error annotated with kind, HTTP status, and
// an optional server-suggested retry delay.
type ProviderError struct {
	Kind       ErrorKind
	StatusCode int
	Message    string
	RetryAfter time.Duration // server hint; 0 if absent
}

func (e *ProviderError) Error() string {
	if e.StatusCode != 0 {
		return fmt.Sprintf("%s (HTTP %d)", e.Message, e.StatusCode)
	}
	return e.Message
}

// StatusCodeVal satisfies the interface checked by errors.As in registry.go.
func (e *ProviderError) StatusCodeVal() int { return e.StatusCode }

// IsRetriable reports whether the error kind warrants a retry.
func (k ErrorKind) IsRetriable() bool {
	switch k {
	case ErrKindRateLimit, ErrKindOverloaded, ErrKindServer:
		return true
	}
	return false
}

// ClassifyError maps an HTTP status code and message body to an ErrorKind.
func ClassifyError(status int, message string) ErrorKind {
	switch status {
	case 429:
		return ErrKindRateLimit
	case 529:
		return ErrKindOverloaded
	case 401, 403:
		return ErrKindAuth
	case 500, 502, 503, 504:
		return ErrKindServer
	case 400:
		lower := strings.ToLower(message)
		if strings.Contains(lower, "context") &&
			(strings.Contains(lower, "length") || strings.Contains(lower, "window") || strings.Contains(lower, "token")) {
			return ErrKindContextTooLong
		}
		return ErrKindInvalidRequest
	}
	lower := strings.ToLower(message)
	if contains(lower, "rate limit", "too many requests") {
		return ErrKindRateLimit
	}
	if contains(lower, "overloaded", "capacity") {
		return ErrKindOverloaded
	}
	if contains(lower, "context length", "context window", "maximum token", "context_length_exceeded") {
		return ErrKindContextTooLong
	}
	return ErrKindUnknown
}

// IsRetriableErr reports whether err is a transient error worth retrying and
// returns the server-suggested wait duration (0 if absent). It handles
// *ProviderError, errors with a StatusCode() method, and falls back to
// heuristic message matching so that SDK-wrapped errors (e.g. from the
// Anthropic or Gemini Go SDKs) are also detected.
func IsRetriableErr(err error) (bool, time.Duration) {
	if err == nil {
		return false, 0
	}
	var pe *ProviderError
	if errors.As(err, &pe) {
		return pe.Kind.IsRetriable(), pe.RetryAfter
	}
	var httpErr interface{ StatusCode() int }
	if errors.As(err, &httpErr) {
		kind := ClassifyError(httpErr.StatusCode(), err.Error())
		if kind.IsRetriable() {
			return true, ParseRetryAfter("", err.Error())
		}
		return false, 0
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	retriable := strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "overloaded") ||
		strings.Contains(lower, "too many requests") ||
		strings.Contains(lower, "429") ||
		strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "eof")
	if retriable {
		return true, ParseRetryAfter("", msg)
	}
	return false, 0
}

// NewProviderError builds a ProviderError from a status, message, and optional
// retry-after hint (pass 0 when absent).
func NewProviderError(status int, message string, retryAfter time.Duration) *ProviderError {
	return &ProviderError{
		Kind:       ClassifyError(status, message),
		StatusCode: status,
		Message:    message,
		RetryAfter: retryAfter,
	}
}
