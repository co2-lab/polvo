package tool

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// sentinelPrefix is a unique prefix for the end-of-command marker written to
// stdout by the persistent shell. It is deliberately long and unusual to
// minimise the chance of collision with normal command output.
const sentinelPrefix = "__POLVO_BASH_EOF_MARKER_"

// pidMarkerPrefix is the prefix for the background PID tracking marker.
const pidMarkerPrefix = "__POLVO_PID_MARKER_"

// interactivePatterns matches interactive command names that appear as a
// standalone command token (not embedded in a path like /usr/bin/vim).
var interactivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(^|\s)read(\s|$)`),
	regexp.MustCompile(`(^|\s)(vi|vim|nano|emacs|less|more)(\s|$)`),
	regexp.MustCompile(`(^|\s)ssh(\s|$)`),
	regexp.MustCompile(`(^|\s)passwd(\s|$)`),
	regexp.MustCompile(`(^|\s)(ftp|telnet|sftp)(\s|$)`),
	regexp.MustCompile(`\bpython[23]?\s+.*input\s*\(`),
}

var gitCommitRx = regexp.MustCompile(`\bgit\s+commit\b`)
var gitCommitWithMsgRx = regexp.MustCompile(`-m\b|--message\b`)

func isInteractiveCommand(cmd string) bool {
	for _, p := range interactivePatterns {
		if p.MatchString(cmd) {
			return true
		}
	}
	if gitCommitRx.MatchString(cmd) && !gitCommitWithMsgRx.MatchString(cmd) {
		return true
	}
	return false
}

// stateBuiltinRx matches shell builtins whose side-effects must happen in the
// outer persistent shell (so that subsequent commands see the changes).
// These are detected by a leading keyword at the start of a trimmed command.
var stateBuiltinRx = regexp.MustCompile(
	`^(cd|export|unset|alias|unalias|source|\.|set|umask|ulimit|eval|exec)\b`,
)

// isStateModifyingCommand returns true when the command must run directly in
// the outer shell to propagate state (cwd, env vars, aliases, etc.).
func isStateModifyingCommand(cmd string) bool {
	trimmed := strings.TrimSpace(cmd)
	return stateBuiltinRx.MatchString(trimmed)
}

type lineMsg struct {
	line string
	done bool
}

// BashLimits configures resource limits applied per command via ulimit.
// Zero value for each field means disabled (no limit).
type BashLimits struct {
	MaxCPUSecs    int // ulimit -t N  (CPU seconds)
	MaxMemMB      int // ulimit -v N*1024 (virtual memory)
	MaxFileSizeMB int // ulimit -f N*1024 (file size)
}

// BashSession manages a single, long-lived bash process.
type BashSession struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	lineCh  chan lineMsg
	workdir string
	timeout time.Duration
	closed  bool
	limits  BashLimits

	// Atomic fields — accessed without holding mu.
	fgPID   atomic.Int64 // PID of the foreground command wrapper; 0 when idle
	running atomic.Bool  // true while a command is executing
}

// NewBashSession starts a persistent bash process rooted at workdir.
func NewBashSession(workdir string, timeout time.Duration) (*BashSession, error) {
	return NewBashSessionWithLimits(workdir, timeout, BashLimits{})
}

// NewBashSessionWithLimits starts a persistent bash process with resource limits.
func NewBashSessionWithLimits(workdir string, timeout time.Duration, limits BashLimits) (*BashSession, error) {
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	cmd := exec.Command("bash", "--norc", "--noprofile")
	cmd.Dir = workdir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("bash session stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("bash session stdout pipe: %w", err)
	}

	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		return nil, fmt.Errorf("bash session start: %w", err)
	}

	lineCh := make(chan lineMsg, 512)

	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 512*1024), 512*1024)
		for scanner.Scan() {
			lineCh <- lineMsg{line: scanner.Text()}
		}
		lineCh <- lineMsg{done: true}
	}()

	return &BashSession{
		cmd:     cmd,
		stdin:   stdin,
		lineCh:  lineCh,
		workdir: workdir,
		timeout: timeout,
		limits:  limits,
	}, nil
}

// buildUlimitPrefix constructs the ulimit shell prefix for resource limits.
// Returns an empty string if no limits are configured.
func buildUlimitPrefix(l BashLimits) string {
	var parts []string
	if l.MaxCPUSecs > 0 {
		parts = append(parts, fmt.Sprintf("ulimit -t %d", l.MaxCPUSecs))
	}
	if l.MaxMemMB > 0 {
		parts = append(parts, fmt.Sprintf("ulimit -v %d", l.MaxMemMB*1024))
	}
	if l.MaxFileSizeMB > 0 {
		parts = append(parts, fmt.Sprintf("ulimit -f %d", l.MaxFileSizeMB*1024))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "; ") + "; "
}

// Cancel sends SIGTERM to the foreground command wrapper. The session remains alive.
func (s *BashSession) Cancel() {
	pid := int(s.fgPID.Load())
	if pid <= 0 {
		return
	}
	killPID(pid, sigTerm)
}

// ForceKill sends SIGKILL to the foreground command wrapper and its process group.
func (s *BashSession) ForceKill() {
	pid := int(s.fgPID.Load())
	if pid <= 0 {
		return
	}
	killPID(pid, sigKill)
	killPGID(pid, sigKill)
}

// IsRunning reports whether a command is currently executing.
func (s *BashSession) IsRunning() bool {
	return s.running.Load()
}

// Run executes cmd inside the persistent session and returns the combined
// stdout+stderr output together with the exit code.
//
// Two execution paths:
//
//  1. State-modifying commands (cd, export, unset, alias, source, …) are run
//     directly in the outer persistent shell so their side effects persist.
//     The shell's own PID is stored as fgPID; cancellation sends SIGTERM to
//     the outer shell (which may destroy the session — acceptable for force-
//     cancel scenarios).
//
//  2. All other commands are wrapped in a background subshell so that their
//     PID is trackable and they can be killed without destroying the session:
//
//     __POLVO_OUTFILE=$(mktemp)
//     ( <ulimit_prefix> cmd ) >"${__POLVO_OUTFILE}" 2>&1 &
//     __POLVO_FG_PID=$!
//     echo "__POLVO_PID_MARKER_${__POLVO_FG_PID}"
//     wait ${__POLVO_FG_PID}
//     __POLVO_EC=$?
//     cat "${__POLVO_OUTFILE}"
//     rm -f "${__POLVO_OUTFILE}"
//     echo "__POLVO_BASH_EOF_MARKER_${__POLVO_EC}"
func (s *BashSession) Run(ctx context.Context, cmd string) (output string, exitCode int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return "", -1, fmt.Errorf("bash session is closed")
	}

	// Mark running state.
	s.running.Store(true)
	s.fgPID.Store(0)
	defer func() {
		s.fgPID.Store(0)
		s.running.Store(false)
	}()

	var wrappedCmd string
	if isStateModifyingCommand(cmd) {
		// Run directly in the outer shell so cwd / env changes persist.
		// Store the outer shell's PID so Cancel can still reach it.
		wrappedCmd = fmt.Sprintf(
			"echo \"%s$$\"\n%s 2>&1\necho \"%s$?\"\n",
			pidMarkerPrefix, cmd, sentinelPrefix,
		)
	} else {
		ulimitPrefix := buildUlimitPrefix(s.limits)
		wrappedCmd = fmt.Sprintf(
			"__POLVO_OUTFILE=$(mktemp)\n"+
				"( %s%s ) >\"${__POLVO_OUTFILE}\" 2>&1 &\n"+
				"__POLVO_FG_PID=$!\n"+
				"echo \"%s${__POLVO_FG_PID}\"\n"+
				"wait ${__POLVO_FG_PID}\n"+
				"__POLVO_EC=$?\n"+
				"cat \"${__POLVO_OUTFILE}\"\n"+
				"rm -f \"${__POLVO_OUTFILE}\"\n"+
				"echo \"%s${__POLVO_EC}\"\n",
			ulimitPrefix, cmd, pidMarkerPrefix, sentinelPrefix,
		)
	}

	_, err = fmt.Fprint(s.stdin, wrappedCmd)
	if err != nil {
		return "", -1, fmt.Errorf("writing to bash stdin: %w", err)
	}

	// Per-command timeout: fires Cancel then ForceKill after 2s grace.
	// Uses time.AfterFunc so the session is NOT closed on timeout.
	var timedOut atomic.Bool
	timer := time.AfterFunc(s.timeout, func() {
		timedOut.Store(true)
		s.Cancel()
		time.Sleep(2 * time.Second)
		if s.running.Load() {
			s.ForceKill()
		}
	})
	defer timer.Stop()

	var sb strings.Builder
	for {
		select {
		case msg := <-s.lineCh:
			if msg.done {
				s.closeLocked()
				return sb.String(), -1, fmt.Errorf("bash session ended unexpectedly")
			}

			line := msg.line

			// PID marker: extract the foreground PID.
			if strings.HasPrefix(line, pidMarkerPrefix) {
				pidStr := line[len(pidMarkerPrefix):]
				if pid, parseErr := strconv.Atoi(strings.TrimSpace(pidStr)); parseErr == nil {
					s.fgPID.Store(int64(pid))
				}
				// PID marker never appears in output.
				continue
			}

			// Sentinel: end of command.
			if strings.HasPrefix(line, sentinelPrefix) {
				suffix := line[len(sentinelPrefix):]
				code, parseErr := strconv.Atoi(strings.TrimSpace(suffix))
				if parseErr == nil {
					exitCode = code
				}
				if timedOut.Load() {
					return sb.String(), exitCode, fmt.Errorf("command timed out after %v", s.timeout)
				}
				return sb.String(), exitCode, nil
			}

			sb.WriteString(line)
			sb.WriteByte('\n')

		case <-ctx.Done():
			// Context cancelled: kill command but keep session alive.
			s.Cancel()
			time.Sleep(200 * time.Millisecond)
			if s.running.Load() {
				s.ForceKill()
			}
			return sb.String(), -1, ctx.Err()
		}
	}
}

// Close terminates the bash process and releases all associated resources.
func (s *BashSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeLocked()
}

// closeLocked does the actual teardown. Caller must hold s.mu.
func (s *BashSession) closeLocked() error {
	if s.closed {
		return nil
	}
	s.closed = true

	// Kill any foreground command first.
	s.ForceKill()

	_ = s.stdin.Close()
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	_ = s.cmd.Wait()
	return nil
}
