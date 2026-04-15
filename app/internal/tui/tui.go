package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/co2-lab/polvo/internal/agent"
	"github.com/co2-lab/polvo/internal/agent/checkpoint"
	"github.com/co2-lab/polvo/internal/agent/microagent"
	"github.com/co2-lab/polvo/internal/provider"
	"github.com/co2-lab/polvo/internal/tool"
)

// Config configures the TUI session.
type Config struct {
	WorkDir  string
	Provider provider.ChatProvider
	Model    string
	ToolReg  *tool.Registry
	System   string
	MaxTurns int
}

// Run starts the bubbletea TUI and blocks until the user exits.
func Run(ctx context.Context, cfg Config) error {
	m := newModel(ctx, cfg)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithInput(os.Stdin))
	m.perm.prog = p
	final, err := p.Run()
	if err != nil {
		return err
	}
	if fm, ok := final.(model); ok && fm.history != "" {
		fmt.Println(fm.history)
	}
	return nil
}

// ── tea messages ─────────────────────────────────────────────────────────────

type agentDeltaMsg struct{ delta string }
type agentDoneMsg struct {
	finalText string
	err       error
}
type toolCallMsg struct {
	name, preview string
	done, isError bool
}
type approvalRequestMsg struct{ req agent.ApprovalRequest }
type approvalSessionMsg struct{ toolName string } // allow this tool for the rest of the session

// ── styles ────────────────────────────────────────────────────────────────────

var (
	// borders
	styleBorder         = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("237")).Padding(0, 1)
	styleApprovalBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("214")).Padding(0, 1)

	// roles
	styleUser      = lipgloss.NewStyle().Foreground(lipgloss.Color("147")).Bold(true) // Pastel purple
	styleAssistant = lipgloss.NewStyle().Foreground(lipgloss.Color("43")).Bold(true)  // Pastel cyan
	styleError     = lipgloss.NewStyle().Foreground(lipgloss.Color("204")).Bold(true) // Pastel red
	styleSystem    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	// message blocks (left border)
	styleUserBlock      = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, false, true).BorderForeground(lipgloss.Color("147")).PaddingLeft(1).MarginBottom(1)
	styleAssistantBlock = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, false, true).BorderForeground(lipgloss.Color("43")).PaddingLeft(1).MarginBottom(1)
	styleErrorBlock     = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, false, true).BorderForeground(lipgloss.Color("204")).PaddingLeft(1).MarginBottom(1)
	styleToolBlock      = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, false, true).BorderForeground(lipgloss.Color("239")).PaddingLeft(1).MarginBottom(1)

	// ui chrome
	styleDim       = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleMuted     = lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
	styleApproval  = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	styleSuccess   = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	styleWarning   = lipgloss.NewStyle().Foreground(lipgloss.Color("221"))
	
	// solid backgrounds for header/footer
	styleStatusBg  = lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("250")).Padding(0, 1)

	// tool icons by name
	toolIcons = map[string]string{
		"bash":       "❯",
		"read":       "📄",
		"write":      "✏️",
		"edit":       "✂️",
		"glob":       "🔍",
		"grep":       "🎯",
		"ls":         "📂",
		"web_fetch":  "🌐",
		"web_search": "🔎",
	}
)

const (
	inputHeight = 2
	// header(1) + margin(1) + statusbar(1) + margin(1) + input + borders(2)
	verticalMargin = 1 + 1 + 1 + 1 + inputHeight + 2
)

// ── permission callback ───────────────────────────────────────────────────────

type tuiPermission struct {
	prog         *tea.Program
	resCh        chan agent.ApprovalDecision
	sessionAllow map[string]bool // tools approved "for this session"
}

func (t *tuiPermission) RequestApproval(ctx context.Context, req agent.ApprovalRequest) (agent.ApprovalDecision, error) {
	if t.sessionAllow[req.ToolName] {
		return agent.ApprovalAllow, nil
	}
	t.prog.Send(approvalRequestMsg{req: req})
	select {
	case <-ctx.Done():
		return agent.ApprovalDeny, ctx.Err()
	case d := <-t.resCh:
		return d, nil
	}
}

// ── model ─────────────────────────────────────────────────────────────────────

type toolEntry struct {
	name    string
	preview string
	done    bool
	isError bool
	start   time.Time
}

type model struct {
	ctx context.Context
	cfg Config

	width, height int

	viewport viewport.Model
	textarea textarea.Model
	spinner  spinner.Model

	agentRunning bool
	cancelRun    context.CancelFunc
	turn         int
	history      string

	toolCalls []toolEntry

	awaitingApproval bool
	pendingReq       agent.ApprovalRequest
	approvalResCh    chan agent.ApprovalDecision
	perm             *tuiPermission

	currentDelta  strings.Builder
	status        string
	startedAt     time.Time
	confirmingExit bool
}

func newModel(ctx context.Context, cfg Config) model {
	ta := textarea.New()
	ta.Placeholder = "Ask anything… (Enter ↵ send  Ctrl+D exit)"
	ta.Focus()
	ta.SetHeight(inputHeight)
	ta.ShowLineNumbers = false
	ta.CharLimit = 0

	// Remove default backgrounds from the textarea to match the terminal
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Text = lipgloss.NewStyle()
	ta.FocusedStyle.Prompt = lipgloss.NewStyle()
	ta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	
	// Ensure cursor is visible
	ta.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	ta.Cursor.Blink = true

	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("43"))

	vp := viewport.New(80, 20)
	vp.SetContent("")

	resCh := make(chan agent.ApprovalDecision, 1)
	perm := &tuiPermission{resCh: resCh, sessionAllow: map[string]bool{}}

	return model{
		ctx:           ctx,
		cfg:           cfg,
		textarea:      ta,
		spinner:       sp,
		viewport:      vp,
		perm:          perm,
		approvalResCh: resCh,
		status:        "ready",
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.spinner.Tick)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		vpH := m.height - verticalMargin
		if vpH < 2 {
			vpH = 2
		}
		m.viewport.Width = m.width
		m.viewport.Height = vpH
		m.textarea.SetWidth(m.width - 4) // adjust for Padding(0, 1) and Borders

	case tea.KeyMsg:
		// Any key cancels the exit-confirmation state (unless it's the confirm key itself).
		if m.confirmingExit && msg.Type != tea.KeyCtrlC && msg.Type != tea.KeyCtrlD {
			m.confirmingExit = false
		}

		switch msg.Type {
		case tea.KeyCtrlD:
			if m.agentRunning && m.cancelRun != nil {
				m.cancelRun()
				return m, nil
			}
			if m.confirmingExit {
				return m, tea.Quit
			}
			m.confirmingExit = true
			return m, nil

		case tea.KeyCtrlC:
			if m.agentRunning && m.cancelRun != nil {
				m.cancelRun()
				return m, nil
			}
			if m.confirmingExit {
				return m, tea.Quit
			}
			m.confirmingExit = true
			return m, nil

		case tea.KeyEnter:
			if m.awaitingApproval {
				m.awaitingApproval = false
				m.approvalResCh <- agent.ApprovalAllow
				m.status = "running…"
				return m, nil
			}
			if !m.agentRunning {
				prompt := strings.TrimSpace(m.textarea.Value())
				if prompt != "" {
					m.textarea.Reset()
					return m, m.startAgent(prompt)
				}
			}

		case tea.KeyRunes:
			if m.awaitingApproval && string(msg.Runes) == "a" {
				// Allow this tool for the rest of the session.
				toolName := m.pendingReq.ToolName
				m.perm.sessionAllow[toolName] = true
				m.awaitingApproval = false
				m.approvalResCh <- agent.ApprovalAllow
				m.status = "running…"
				return m, tea.Cmd(func() tea.Msg { return approvalSessionMsg{toolName: toolName} })
			}

		case tea.KeyEsc:
			if m.awaitingApproval {
				m.awaitingApproval = false
				m.approvalResCh <- agent.ApprovalDeny
				m.status = "ready"
				return m, nil
			}
		}

		if !m.awaitingApproval && !m.agentRunning {
			var taCmd tea.Cmd
			m.textarea, taCmd = m.textarea.Update(msg)
			cmds = append(cmds, taCmd)
		}

	case spinner.TickMsg:
		var spCmd tea.Cmd
		m.spinner, spCmd = m.spinner.Update(msg)
		cmds = append(cmds, spCmd)

	case agentDeltaMsg:
		m.currentDelta.WriteString(msg.delta)
		m.refreshViewport()
		m.viewport.GotoBottom()
		cmds = append(cmds, drainChan(msg))

	case toolCallMsg:
		if msg.done {
			for i := len(m.toolCalls) - 1; i >= 0; i-- {
				if m.toolCalls[i].name == msg.name && !m.toolCalls[i].done {
					m.toolCalls[i].done = true
					m.toolCalls[i].isError = msg.isError
					break
				}
			}
			m.status = fmt.Sprintf("%s done", msg.name)
		} else {
			m.toolCalls = append(m.toolCalls, toolEntry{
				name:    msg.name,
				preview: msg.preview,
				start:   time.Now(),
			})
			m.status = fmt.Sprintf("%s %s", toolIcon(msg.name), truncate(msg.preview, 55))
		}
		m.refreshViewport()

	case approvalRequestMsg:
		m.awaitingApproval = true
		m.pendingReq = msg.req
		m.status = "awaiting approval"
		m.refreshViewport()

	case approvalSessionMsg:
		// no-op: already handled inline; just triggers a re-render

	case agentDoneMsg:
		m.agentRunning = false
		m.cancelRun = nil
		if m.currentDelta.Len() > 0 {
			m.appendHistory("assistant", m.currentDelta.String())
			m.currentDelta.Reset()
		}
		if len(m.toolCalls) > 0 {
			m.appendToolSummary()
			m.toolCalls = nil
		}
		if msg.err != nil {
			m.appendHistory("error", friendlyError(msg.err.Error()))
		}
		m.refreshViewport()
		m.viewport.GotoBottom()
		m.status = "ready"
		cmds = append(cmds, textarea.Blink)

	default:
		// Pass other messages (like blink ticks) to textarea
		var taCmd tea.Cmd
		m.textarea, taCmd = m.textarea.Update(msg)
		cmds = append(cmds, taCmd)
	}

	if m.agentRunning {
		cmds = append(cmds, m.spinner.Tick)
	}

	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	return m, tea.Batch(cmds...)
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m model) View() string {
	if m.width == 0 {
		return "loading…"
	}

	header := m.renderHeader()
	vpView := m.viewport.View()
	statusBar := m.renderStatusBar()
	inputView := m.renderInput()

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		"",
		vpView,
		"",
		statusBar,
		inputView,
	)
}

func (m model) renderHeader() string {
	wordmark :=
		lipgloss.NewStyle().Foreground(lipgloss.Color("48")).Render("P") +
			lipgloss.NewStyle().Foreground(lipgloss.Color("45")).Render("o") +
			lipgloss.NewStyle().Foreground(lipgloss.Color("45")).Render("l") +
			lipgloss.NewStyle().Foreground(lipgloss.Color("63")).Render("v") +
			lipgloss.NewStyle().Foreground(lipgloss.Color("63")).Render("o")

	dir := compactPath(m.cfg.WorkDir)
	model := m.cfg.Model

	left := wordmark + "  " + styleMuted.Render(dir) + styleDim.Render(" • ") + styleMuted.Render(model)

	right := ""
	if m.agentRunning && m.cfg.MaxTurns > 0 {
		right = styleDim.Render(fmt.Sprintf("Turn %d/%d", m.turn+1, m.cfg.MaxTurns))
	} else if !m.agentRunning && m.turn > 0 {
		right = styleDim.Render(fmt.Sprintf("%d turns", m.turn))
	}

	leftLen := lipgloss.Width(left)
	rightLen := lipgloss.Width(right)
	pad := m.width - leftLen - rightLen
	if pad < 0 {
		pad = 0
	}
	return left + strings.Repeat(" ", pad) + right
}

func (m model) renderStatusBar() string {
	maxW := m.width

	switch {
	case m.awaitingApproval:
		req := m.pendingReq
		icon := toolIcon(req.ToolName)
		toolLabel := styleApproval.Render(icon + " " + req.ToolName)
		preview := styleMuted.Render("  " + truncate(req.Preview, maxW-50))
		keys := styleDim.Render("  Enter=once  a=session  Esc=deny ")

		left := styleStatusBg.Render("⚠ Approval") + "  " + toolLabel + preview
		pad := m.width - lipgloss.Width(left) - lipgloss.Width(keys)
		if pad < 0 {
			pad = 0
		}
		return left + strings.Repeat(" ", pad) + keys

	case m.agentRunning:
		sp := m.spinner.View()
		stat := styleMuted.Render(" " + truncate(m.status, maxW-30))
		elapsed := ""
		if !m.startedAt.IsZero() {
			elapsed = styleDim.Render(fmt.Sprintf(" %s ", fmtDuration(time.Since(m.startedAt))))
		}
		
		left := styleStatusBg.Render(sp+" Running") + stat
		leftLen := lipgloss.Width(left)
		elapsedLen := lipgloss.Width(elapsed)
		pad := m.width - leftLen - elapsedLen
		if pad < 0 { pad = 0 }
		return left + strings.Repeat(" ", pad) + elapsed

	default:
		if m.confirmingExit {
			msg := styleWarning.Render("● Press Ctrl+C or Ctrl+D again to exit")
			keys := styleDim.Render(" any other key to cancel ")
			pad := m.width - lipgloss.Width(msg) - lipgloss.Width(keys)
			if pad < 0 {
				pad = 0
			}
			return msg + strings.Repeat(" ", pad) + keys
		}
		ready := styleStatusBg.Render("● Ready")
		keys := styleDim.Render(" ↑↓ scroll  Ctrl+D exit  Ctrl+C cancel ")
		pad := m.width - lipgloss.Width(ready) - lipgloss.Width(keys)
		if pad < 0 {
			pad = 0
		}
		return ready + strings.Repeat(" ", pad) + keys
	}
}

func (m model) renderInput() string {
	if m.awaitingApproval {
		req := m.pendingReq
		icon := toolIcon(req.ToolName)
		tool := styleApproval.Render(icon + " " + req.ToolName)

		risk := ""
		switch req.RiskLevel {
		case "critical":
			risk = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render(" ● critical")
		case "high":
			risk = lipgloss.NewStyle().Foreground(lipgloss.Color("202")).Render(" ● high")
		case "medium":
			risk = lipgloss.NewStyle().Foreground(lipgloss.Color("221")).Render(" ● medium")
		case "low":
			risk = styleDim.Render(" ● low")
		}

		header := tool + risk
		body := styleMuted.Render("  " + req.Preview)
		keys := styleDim.Render("  Enter=allow once  a=allow for session  Esc=deny")
		inner := lipgloss.JoinVertical(lipgloss.Left, header, body, keys)
		return styleApprovalBorder.Width(m.width - 2).Render(inner)
	}
	if m.agentRunning {
		return styleBorder.Width(m.width - 2).Render(styleDim.Render("(agent running… Ctrl+D to interrupt)"))
	}
	return styleBorder.Width(m.width - 2).Render(m.textarea.View())
}

// ── agent start ───────────────────────────────────────────────────────────────

func (m *model) startAgent(prompt string) tea.Cmd {
	m.agentRunning = true
	m.currentDelta.Reset()
	m.toolCalls = nil
	m.startedAt = time.Now()
	m.appendHistory("user", prompt)
	m.refreshViewport()
	m.viewport.GotoBottom()
	m.status = "thinking…"

	ctx, cancel := context.WithCancel(m.ctx)
	m.cancelRun = cancel

	maxTurns := m.cfg.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 50
	}

	prog := m.perm.prog
	deltaCh := make(chan string, 128)
	toolCallCh := make(chan toolCallMsg, 16)
	doneCh := make(chan agentDoneMsg, 1)

	// Initialise a checkpoint recorder for this run (non-fatal if it fails).
	var ckptRecorder *checkpoint.Recorder
	ckptDir := filepath.Join(m.cfg.WorkDir, ".polvo", "checkpoints")
	if store := checkpoint.NewFSStore(ckptDir); store != nil {
		sessionID := fmt.Sprintf("tui-%d", time.Now().UnixNano())
		if rec, err := checkpoint.NewRecorder(store, sessionID, "tui"); err == nil {
			ckptRecorder = rec
		}
	}

	// Load microagents for context injection.
	homeDir, _ := os.UserHomeDir()
	maLoader := microagent.NewLoader(
		filepath.Join(m.cfg.WorkDir, ".polvo", "microagents"),
		filepath.Join(homeDir, ".polvo", "microagents"),
	)

	loopCfg := agent.LoopConfig{
		Provider:           m.cfg.Provider,
		Tools:              m.cfg.ToolReg,
		System:             m.cfg.System,
		Model:              m.cfg.Model,
		MaxTurns:           maxTurns,
		PermissionCallback: m.perm,
		Checkpoint:         ckptRecorder,
		MicroagentLoader:   maLoader,
		OnTextDelta: func(d string) {
			deltaCh <- d
		},
		OnToolCall: func(c provider.ToolCall) {
			toolCallCh <- toolCallMsg{
				name:    c.Name,
				preview: truncate(string(c.Input), 120),
			}
		},
		OnToolResult: func(_, name, _ string, isError bool) {
			toolCallCh <- toolCallMsg{name: name, done: true, isError: isError}
		},
	}

	l := agent.NewLoop(loopCfg)

	go func() {
		_, err := l.Run(ctx, prompt)
		if ckptRecorder != nil {
			status := "completed"
			if err != nil {
				status = "failed"
			}
			_ = ckptRecorder.Finish(status)
		}
		doneCh <- agentDoneMsg{err: err}
	}()

	go func() {
		defer cancel()
		for {
			select {
			case d, ok := <-deltaCh:
				if ok {
					prog.Send(agentDeltaMsg{delta: d})
				}
			case tc, ok := <-toolCallCh:
				if ok {
					prog.Send(tc)
				}
			case done := <-doneCh:
				for {
					select {
					case d := <-deltaCh:
						prog.Send(agentDeltaMsg{delta: d})
					default:
						goto sendDone
					}
				}
			sendDone:
				prog.Send(done)
				return
			}
		}
	}()

	return nil
}

// ── history rendering ─────────────────────────────────────────────────────────

func (m *model) appendHistory(role, text string) {
	var line string
	switch role {
	case "user":
		label := styleUser.Render("You")
		body := styleMuted.Render(text)
		line = styleUserBlock.Render(label + "\n" + body)
	case "assistant":
		label := styleAssistant.Render("Polvo")
		line = styleAssistantBlock.Render(label + "\n" + text)
		m.turn++
	case "error":
		label := styleError.Render("Error")
		line = styleErrorBlock.Render(label + "\n" + styleError.Render(text))
	case "tools":
		label := styleSystem.Render("Tools Used")
		line = styleToolBlock.Render(label + "\n" + text)
	}
	
	if m.history != "" {
		m.history += "\n"
	}
	m.history += line
}

func (m *model) appendToolSummary() {
	if len(m.toolCalls) == 0 {
		return
	}
	var sb strings.Builder
	for i, tc := range m.toolCalls {
		if i > 0 {
			sb.WriteString("\n")
		}
		icon := toolIcon(tc.name)
		if tc.done {
			if tc.isError {
				sb.WriteString(styleError.Render("✗ " + icon + " " + tc.name))
			} else {
				sb.WriteString(styleSuccess.Render("✓ " + icon + " " + tc.name))
			}
		} else {
			sb.WriteString(styleSystem.Render("· " + icon + " " + tc.name))
		}
		if tc.preview != "" {
			sb.WriteString(styleDim.Render("  " + truncate(tc.preview, 60)))
		}
	}
	m.appendHistory("tools", sb.String())
}

func (m *model) refreshViewport() {
	content := m.history

	// live tool calls being executed
	if len(m.toolCalls) > 0 {
		var sb strings.Builder
		for i, tc := range m.toolCalls {
			if i > 0 {
				sb.WriteString("\n")
			}
			icon := toolIcon(tc.name)
			if tc.done {
				if tc.isError {
					sb.WriteString(styleError.Render("✗ " + icon + " " + tc.name))
				} else {
					sb.WriteString(styleSuccess.Render("✓ " + icon + " " + tc.name))
				}
			} else {
				sb.WriteString(styleSystem.Render("· " + icon + " " + tc.name))
				if tc.preview != "" {
					sb.WriteString(styleDim.Render("  " + truncate(tc.preview, 60)))
				}
			}
		}
		if content != "" {
			content += "\n"
		}
		content += styleToolBlock.Render(styleSystem.Render("Running") + "\n" + sb.String())
	}

	// streaming assistant response
	if m.currentDelta.Len() > 0 {
		if content != "" {
			content += "\n"
		}
		label := styleAssistant.Render("Polvo")
		content += styleAssistantBlock.Render(label + "\n" + m.currentDelta.String())
	}

	m.viewport.SetContent(content)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func toolIcon(name string) string {
	if ic, ok := toolIcons[name]; ok {
		return ic
	}
	return "·"
}

func compactPath(p string) string {
	parts := strings.Split(filepath.ToSlash(p), "/")
	if len(parts) <= 2 {
		return p
	}
	return "…/" + strings.Join(parts[len(parts)-2:], "/")
}

func fmtDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}

func drainChan(_ agentDeltaMsg) tea.Cmd { return nil }

// friendlyError converts raw API error JSON/messages into readable one-liners.
func friendlyError(msg string) string {
	// HTTP status codes
	for _, code := range []string{"429", "401", "403", "500", "503"} {
		if strings.Contains(msg, "HTTP "+code) {
			switch code {
			case "429":
				// extract retryDelay if present
				if i := strings.Index(msg, `"retryDelay": "`); i >= 0 {
					s := msg[i+15:]
					if j := strings.Index(s, `"`); j >= 0 {
						return fmt.Sprintf("rate limit exceeded — retry in %s", s[:j])
					}
				}
				return "rate limit exceeded (HTTP 429) — wait a moment and try again"
			case "401":
				return "authentication failed (HTTP 401) — check your API key"
			case "403":
				return "access denied (HTTP 403) — check your API key permissions"
			case "500":
				return "provider internal error (HTTP 500) — try again"
			case "503":
				return "provider unavailable (HTTP 503) — try again later"
			}
		}
	}
	// context cancellation
	if strings.Contains(msg, "context canceled") || strings.Contains(msg, "context cancelled") {
		return "cancelled"
	}
	// quota messages
	if strings.Contains(msg, "RESOURCE_EXHAUSTED") || strings.Contains(msg, "quota") {
		return "quota exhausted — check your plan or wait before retrying"
	}
	return msg
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}
