// Package telemetry wraps Sentry for error tracking.
// Enabled by default using the Polvo DSN embedded at build time.
// Users can opt out via polvo.yaml or ~/.polvo/config.yaml:
//
//	telemetry:
//	  disabled: true
package telemetry

import (
	"log/slog"
	"slices"
	"time"

	"github.com/getsentry/sentry-go"
)

// defaultDSN is the Polvo project DSN, injected via ldflags at build time:
//
//	-ldflags "-X github.com/co2-lab/polvo/internal/telemetry.defaultDSN=https://..."
//
// Empty in dev builds — telemetry is a no-op until a DSN is provided.
var defaultDSN = ""

// Config holds telemetry options derived from user/project config.
type Config struct {
	Disabled    bool   // true = user opted out
	Environment string // overrides auto-detected environment
	Release     string // version string set at build time
}

// Init initialises Sentry with the embedded DSN unless the user opted out.
// Safe to call when defaultDSN is empty — becomes a no-op.
func Init(cfg Config) {
	if cfg.Disabled || defaultDSN == "" {
		return
	}
	env := cfg.Environment
	if env == "" {
		env = "production"
	}
	err := sentry.Init(sentry.ClientOptions{
		Dsn:              defaultDSN,
		Environment:      env,
		Release:          cfg.Release,
		TracesSampleRate: 0, // errors only, no performance tracing
		BeforeSend:       scrubEvent,
	})
	if err != nil {
		slog.Warn("telemetry: sentry init failed", "err", err)
	}
}

// Flush waits up to 2 s for queued events to be delivered.
// Call defer telemetry.Flush() in main after Init.
func Flush() {
	sentry.Flush(2 * time.Second)
}

// CaptureError sends a non-fatal error to Sentry.
// No-op when Sentry is not initialised.
func CaptureError(err error, tags map[string]string) {
	if err == nil {
		return
	}
	sentry.WithScope(func(scope *sentry.Scope) {
		for k, v := range tags {
			scope.SetTag(k, v)
		}
		sentry.CaptureException(err)
	})
}

// scrubEvent strips sensitive data before the event leaves the process:
// – removes absolute file paths from stack frames
// – clears request bodies / HTTP payloads
// – removes extra keys that look like secrets
func scrubEvent(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	for i := range event.Exception {
		for j := range event.Exception[i].Stacktrace.Frames {
			f := &event.Exception[i].Stacktrace.Frames[j]
			f.AbsPath = "" // don't leak absolute paths
		}
	}
	if event.Request != nil {
		event.Request.Data = ""
		event.Request.Cookies = ""
		event.Request.Headers = nil
	}
	for k := range event.Extra {
		if isSensitiveKey(k) {
			delete(event.Extra, k)
		}
	}
	return event
}

func isSensitiveKey(k string) bool {
	return slices.Contains([]string{
		"api_key", "apikey", "token", "secret",
		"password", "prompt", "response", "content",
	}, k)
}
