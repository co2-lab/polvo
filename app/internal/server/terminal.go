//go:build !windows

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/creack/pty"
	"github.com/coder/websocket"
)

// session holds a running PTY process and its scrollback buffer.
type session struct {
	mu         sync.Mutex
	ptmx       *os.File
	scrollback []byte
}

// termState manages all active terminal sessions keyed by session ID.
type termState struct {
	mu       sync.Mutex
	sessions map[string]*session
}

func newTermState() *termState {
	return &termState{sessions: make(map[string]*session)}
}

func (ts *termState) get(id string) (*session, bool) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	s, ok := ts.sessions[id]
	return s, ok
}

func (ts *termState) set(id string, s *session) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.sessions[id] = s
}

func (ts *termState) delete(id string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	delete(ts.sessions, id)
}

func (s *Server) registerTerminalRoutes() {
	s.mux.HandleFunc("/terminal/ws", s.handleTerminalWS)
}

func (s *Server) handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		id = "default"
	}
	cmdArg := r.URL.Query().Get("cmd")

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // dev: accept all origins
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx := r.Context()

	send := func(kind byte, data []byte) error {
		msg := make([]byte, 1+len(data))
		msg[0] = kind
		copy(msg[1:], data)
		return conn.Write(ctx, websocket.MessageBinary, msg)
	}

	// Resume existing session if PTY is still alive.
	if sess, ok := s.term.get(id); ok {
		sess.mu.Lock()
		sb := make([]byte, len(sess.scrollback))
		copy(sb, sess.scrollback)
		sess.mu.Unlock()
		if len(sb) > 0 {
			_ = send(msgOutput, sb)
		}
		_ = send(msgResumed, nil)
		s.pumpSession(ctx, conn, sess, send)
		return
	}

	// Start a new PTY session
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	var cmd *exec.Cmd
	if cmdArg != "" {
		// Resolve the executable: prefer PATH lookup, fall back to the running
		// binary itself when the name matches (e.g. "polvo" inside the sidecar
		// where PATH may not include the install directory).
		resolved := cmdArg
		if _, err := exec.LookPath(cmdArg); err != nil {
			if self, serr := os.Executable(); serr == nil {
				base := filepath.Base(self)
				// Handle Tauri sidecars which often have target triple suffixes
				if base == cmdArg || base == cmdArg+".exe" || strings.HasPrefix(base, cmdArg+"-") {
					resolved = self
				}
			}
		}
		slog.Info("terminal: starting executable", "cmd", cmdArg, "resolved", resolved, "id", id)
		cmd = exec.CommandContext(ctx, resolved)
	} else {
		cmd = exec.CommandContext(ctx, shell)
	}
	// Filter out POLVO_SIDECAR so child processes (e.g. polvo TUI) don't
	// inherit sidecar mode and try to start another HTTP server.
	filteredEnv := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "POLVO_SIDECAR=") {
			filteredEnv = append(filteredEnv, e)
		}
	}
	termType := "xterm-256color"
	if cmdArg != "" {
		// Use screen prefix to skip termenv's background color query (which hangs for 5s in PTYs)
		termType = "screen-256color"
	}
	env := append(filteredEnv, "TERM="+termType)
	if cmdArg != "" {
		env = append(env, "POLVO_EMBEDDED=1")
	}
	cmd.Env = env

	ptmx, err := pty.Start(cmd)
	if err != nil {
		slog.Error("terminal: pty.Start failed", "cmd", cmd.Path, "err", err)
		return
	}

	sess := &session{ptmx: ptmx}
	s.term.set(id, sess)
	_ = send(msgNew, nil)

	go func() {
		_ = cmd.Wait()
		ptmx.Close()
		s.term.delete(id)
	}()

	s.pumpSession(ctx, conn, sess, send)
}

func (s *Server) pumpSession(ctx context.Context, conn *websocket.Conn, sess *session, send func(byte, []byte) error) {
	// PTY → WebSocket
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := sess.ptmx.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				sess.mu.Lock()
				if len(sess.scrollback) > 512*1024 {
					sess.scrollback = sess.scrollback[len(sess.scrollback)-256*1024:]
				}
				sess.scrollback = append(sess.scrollback, chunk...)
				sess.mu.Unlock()
				_ = send(msgOutput, chunk)
			}
			if err != nil {
				return
			}
		}
	}()

	// WebSocket → PTY
	for {
		_, msg, err := conn.Read(ctx)
		if err != nil || len(msg) == 0 {
			return
		}
		switch msg[0] {
		case msgInput:
			_, _ = sess.ptmx.Write(msg[1:])
		case msgResize:
			var size struct {
				Cols uint16 `json:"cols"`
				Rows uint16 `json:"rows"`
			}
			if json.Unmarshal(msg[1:], &size) == nil {
				_ = pty.Setsize(sess.ptmx, &pty.Winsize{Cols: size.Cols, Rows: size.Rows})
			}
		}
	}
}
