package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"strings"
	"sync"
	"time"
)

// SeqHandler is an slog.Handler that ships log records to a Seq server
// using the CLEF (Compact Log Event Format) over HTTP.
//
// Records are buffered and flushed in batches every 500 ms or when the
// buffer reaches 50 events, whichever comes first. A final flush is
// triggered by Close.
//
// Wire it in main:
//
//	if url := os.Getenv("SEQ_URL"); url != "" {
//	    h := telemetry.NewSeqHandler(url, "polvo-tui", slog.LevelDebug)
//	    slog.SetDefault(slog.New(h))
//	}
type SeqHandler struct {
	url     string // e.g. "http://localhost:5341"
	service string
	level   slog.Level
	attrs   []slog.Attr
	groups  []string

	mu     sync.Mutex
	buf    []clefEvent
	done   chan struct{}
	client *http.Client
}

type clefEvent struct {
	Timestamp string         `json:"@t"`
	Template  string         `json:"@mt"`
	Level     string         `json:"@l,omitempty"`
	Exception string         `json:"@x,omitempty"`
	Service   string         `json:"service,omitempty"`
	Extra     map[string]any `json:"-"`
}

// NewSeqHandler creates a SeqHandler that sends to url/api/events/raw.
// service is attached to every event as the "service" field.
func NewSeqHandler(url, service string, level slog.Level) *SeqHandler {
	h := &SeqHandler{
		url:     url,
		service: service,
		level:   level,
		done:    make(chan struct{}),
		client:  &http.Client{Timeout: 5 * time.Second},
	}
	go h.flusher()
	return h
}

// Close flushes remaining events and stops the background goroutine.
func (h *SeqHandler) Close() {
	close(h.done)
	h.flush()
}

// — slog.Handler interface ————————————————————————————————————————————————

func (h *SeqHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *SeqHandler) Handle(_ context.Context, r slog.Record) error {
	ev := clefEvent{
		Timestamp: r.Time.UTC().Format(time.RFC3339Nano),
		Template:  r.Message,
		Level:     seqLevel(r.Level),
		Service:   h.service,
		Extra:     make(map[string]any, r.NumAttrs()+len(h.attrs)),
	}

	// Parent attrs (from WithAttrs).
	for _, a := range h.attrs {
		ev.Extra[attrKey(h.groups, a.Key)] = a.Value.Any()
	}

	// Record attrs.
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "err" || a.Key == "error" {
			if err, ok := a.Value.Any().(error); ok {
				ev.Exception = err.Error()
				return true
			}
		}
		ev.Extra[attrKey(h.groups, a.Key)] = a.Value.Any()
		return true
	})

	h.mu.Lock()
	h.buf = append(h.buf, ev)
	flush := len(h.buf) >= 50
	h.mu.Unlock()

	if flush {
		h.flush()
	}
	return nil
}

func (h *SeqHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h2 := h.clone()
	h2.attrs = append(h2.attrs, attrs...)
	return h2
}

func (h *SeqHandler) WithGroup(name string) slog.Handler {
	h2 := h.clone()
	h2.groups = append(h2.groups, name)
	return h2
}

// — internals ——————————————————————————————————————————————————————————————

func (h *SeqHandler) clone() *SeqHandler {
	return &SeqHandler{
		url:     h.url,
		service: h.service,
		level:   h.level,
		attrs:   append([]slog.Attr{}, h.attrs...),
		groups:  append([]string{}, h.groups...),
		done:    h.done,
		client:  h.client,
	}
}

func (h *SeqHandler) flusher() {
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			h.flush()
		case <-h.done:
			return
		}
	}
}

func (h *SeqHandler) flush() {
	h.mu.Lock()
	if len(h.buf) == 0 {
		h.mu.Unlock()
		return
	}
	events := h.buf
	h.buf = nil
	h.mu.Unlock()

	var body bytes.Buffer
	for _, ev := range events {
		// Merge Extra into the top-level CLEF object.
		m := map[string]any{
			"@t":  ev.Timestamp,
			"@mt": ev.Template,
		}
		if ev.Level != "" {
			m["@l"] = ev.Level
		}
		if ev.Exception != "" {
			m["@x"] = ev.Exception
		}
		if ev.Service != "" {
			m["service"] = ev.Service
		}
		maps.Copy(m, ev.Extra)
		line, err := json.Marshal(m)
		if err != nil {
			continue
		}
		body.Write(line)
		body.WriteByte('\n')
	}

	req, err := http.NewRequest(http.MethodPost,
		h.url+"/api/events/raw",
		&body,
	)
	if err != nil {
		fmt.Fprintf(h.fallback(), "seq: build request error: %v\n", err)
		return
	}
	req.Header.Set("Content-Type", "application/vnd.serilog.clef")

	resp, err := h.client.Do(req)
	if err != nil {
		fmt.Fprintf(h.fallback(), "seq: send error: %v\n", err)
		return
	}
	resp.Body.Close()
}

// fallback returns stderr as a last-resort writer (avoids import cycle with os).
func (h *SeqHandler) fallback() *seqStderr { return &seqStderr{} }

type seqStderr struct{}

func (s *seqStderr) Write(p []byte) (int, error) {
	return fmt.Print(string(p))
}

func seqLevel(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "Error"
	case l >= slog.LevelWarn:
		return "Warning"
	case l >= slog.LevelInfo:
		return "Information"
	default:
		return "Debug"
	}
}

func attrKey(groups []string, key string) string {
	if len(groups) == 0 {
		return key
	}
	return fmt.Sprintf("%s.%s", joinDot(groups), key)
}

func joinDot(parts []string) string {
	return strings.Join(parts, ".")
}
