package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
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
	"github.com/co2-lab/polvo/internal/session"
	"github.com/co2-lab/polvo/internal/skill"
	"github.com/co2-lab/polvo/internal/tool"
)

// FileIndexer handles incremental file indexing for the chunk search index.
type FileIndexer interface {
	IndexFile(path string) error
	RemoveFile(path string) error
}

// Config configures the TUI session.
type Config struct {
	WorkDir         string
	Provider        provider.ChatProvider
	Model           string
	ToolReg         *tool.Registry
	System          string
	MaxTurns        int
	ProviderOptions []ProviderOption // optional: enables /model picker
	// AddProviderFn is called when the user selects "Add new provider…" in the
	// /model picker. It should run a setup wizard and return an updated list of
	// ProviderOptions. If nil, the "Add" entry is not shown.
	AddProviderFn func() ([]ProviderOption, error)
	// Indexer is optional. When set, file change events update the chunk index.
	Indexer FileIndexer
	// SessionManager is optional. When set, /task and /question commands are enabled.
	SessionManager *session.Manager
	// SummaryRunner is optional. When set, async summaries are generated for work items.
	SummaryRunner *session.SummaryRunner
	// SummaryModel is optional. When set, a dedicated cheap model is used for turn
	// summaries and session work item summaries. When empty, InlineSummary mode is used.
	SummaryModel string
	// SummaryProvider is the provider to use for dedicated summary calls.
	// Only used when SummaryModel is non-empty.
	SummaryProvider provider.ChatProvider
	// SkillExtractor is optional. When set, skills are extracted at the end
	// of each work item and stored in the memory store for future sessions.
	SkillExtractor *skill.Extractor
}

// ProviderOption is a selectable provider+model pair for the /model picker.
type ProviderOption struct {
	Label    string               // display name, e.g. "claude · claude-sonnet-4-5"
	Provider provider.ChatProvider
	Model    string
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
	if fm, ok := final.(model); ok {
		printExitSummary(fm)
	}
	return nil
}

func printExitSummary(m model) {
	var parts []string
	if m.turn > 0 {
		parts = append(parts, fmt.Sprintf("%d turns", m.turn))
	}
	if m.liveTokens > 0 {
		parts = append(parts, fmtTokens(m.liveTokens))
	}
	if m.liveCost > 0 {
		parts = append(parts, fmtCost(m.liveCost))
	}
	if !m.startedAt.IsZero() {
		parts = append(parts, fmtDuration(time.Since(m.startedAt)))
	}

	goodbye := "\033[2m✦ goodbye"
	if len(parts) > 0 {
		goodbye += "  " + strings.Join(parts, " · ")
	}
	goodbye += "\033[0m"
	fmt.Println(goodbye)
}

// ── tea messages ─────────────────────────────────────────────────────────────

type agentDeltaMsg struct{ delta string }
type agentDoneMsg struct {
	finalText  string
	err        error
	tokensUsed provider.TokenUsage
	costUSD    float64
}
type toolCallMsg struct {
	name, preview string
	output        string
	dur           time.Duration
	done, isError bool
}
type approvalRequestMsg struct{ req agent.ApprovalRequest }
type approvalSessionMsg struct{ toolName string } // allow this tool for the rest of the session
type agentSuspendedMsg struct{ signal agent.SuspendSignal }
type turnSummaryDoneMsg struct {
	turnIdx int
	summary string
	err     error
}
type addProviderDoneMsg struct {
	opts []ProviderOption
	err  error
}

// addProviderExec implements tea.ExecCommand so the TUI hands the terminal to
// AddProviderFn (the setup wizard) and then resumes.
type addProviderExec struct {
	fn     func() ([]ProviderOption, error)
	result *addProviderDoneMsg

	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func (e *addProviderExec) Run() error {
	opts, err := e.fn()
	e.result.opts = opts
	e.result.err = err
	return nil // errors are surfaced via addProviderDoneMsg.err
}
func (e *addProviderExec) SetStdin(r io.Reader)  { e.stdin = r }
func (e *addProviderExec) SetStdout(w io.Writer) { e.stdout = w }
func (e *addProviderExec) SetStderr(w io.Writer) { e.stderr = w }

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

	// message blocks: vibrant left border, margin top/bottom for spacing between blocks
	styleUserBlock      = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, false, true).BorderForeground(lipgloss.Color("147")).PaddingLeft(1).MarginBottom(1)
	styleAssistantBlock = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, false, true).BorderForeground(lipgloss.Color("43")).PaddingLeft(1).MarginBottom(1)
	styleErrorBlock     = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, false, true).BorderForeground(lipgloss.Color("204")).PaddingLeft(1).MarginBottom(1)
	styleToolBlock      = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, false, true).BorderForeground(lipgloss.Color("239")).PaddingLeft(1).MarginBottom(1)

	// ui chrome
	styleDim      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleMuted    = lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
	styleApproval = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	styleSuccess  = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	styleWarning  = lipgloss.NewStyle().Foreground(lipgloss.Color("221"))

	// turn marks
	styleUsefulBlock    = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, false, true).BorderForeground(lipgloss.Color("220")).PaddingLeft(1).MarginBottom(1)
	styleSelectedBlock  = lipgloss.NewStyle().Border(lipgloss.ThickBorder(), false, false, false, true).BorderForeground(lipgloss.Color("220")).PaddingLeft(1).MarginBottom(1)
	styleDismissedBlock = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, false, true).BorderForeground(lipgloss.Color("238")).PaddingLeft(1).MarginBottom(1)

	// solid backgrounds for header/footer
	styleStatusBg = lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("250")).Padding(0, 1)

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
		"think":      "💭",
		"diff":       "±",
		"patch":      "🔩",
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
	preview string        // formatted input (human-readable)
	output  string        // first meaningful line of tool output
	done    bool
	isError bool
	start   time.Time
	dur     time.Duration // set when done=true
}

// historyEntry is a single rendered entry in the conversation history.
// Entries with turnIdx >= 0 belong to a completed turn and can be marked.
type historyEntry struct {
	role    string // "user", "assistant", "error", "system", "tools"
	text    string // raw text content (unstyled)
	turnIdx int    // -1 for non-turn entries (error, system, tools)
	// token/cost metadata (only set for "assistant" role)
	tokens provider.TokenUsage
	cost   float64
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
	history      string          // cached rendered output, rebuilt by refreshViewport
	historyEntries []historyEntry // structured entries backing history

	toolCalls []toolEntry

	awaitingApproval bool
	pendingReq       agent.ApprovalRequest
	approvalResCh    chan agent.ApprovalDecision
	perm             *tuiPermission

	currentDelta string
	status       string
	startedAt    time.Time
	// live token accumulation (updated per turn via agentDoneMsg)
	liveTokens int
	liveCost   float64

	confirmingExit bool

	// /model picker state
	pickingModel  bool
	pickerCursor  int
	addingProvider bool // true while AddProviderFn is running

	// autocomplete state
	acItems  []string // current completion candidates
	acCursor int      // selected index
	acActive bool     // dropdown visible

	// tool detail visibility (Ctrl+T toggle)
	showTools bool

	// active work item (set by /task or /question commands)
	activeWorkItemID    string
	lastWorkItemSummary string

	// suspend/resume state
	suspended      bool
	suspendCh      chan agent.SuspendSignal
	resumeCh       chan string
	pendingSuspend agent.SuspendSignal

	// turn mark state
	turnSelectMode bool
	selectedTurn   int // -1 = none
	conv           *agent.Conversation
	ckptStore      checkpoint.Saver
	ckptSessionID  string
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
		showTools:     true,
		selectedTurn:  -1,
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

		// /model picker navigation
		if m.pickingModel {
			opts := m.cfg.ProviderOptions
			hasAdd := m.cfg.AddProviderFn != nil
			total := len(opts)
			if hasAdd {
				total++ // extra "Add new provider…" row
			}
			switch msg.Type {
			case tea.KeyEsc, tea.KeyCtrlC:
				m.pickingModel = false
				return m, nil
			case tea.KeyUp:
				if m.pickerCursor > 0 {
					m.pickerCursor--
				}
				return m, nil
			case tea.KeyDown:
				if m.pickerCursor < total-1 {
					m.pickerCursor++
				}
				return m, nil
			case tea.KeyEnter:
				if hasAdd && m.pickerCursor == len(opts) {
					// "Add new provider…" selected — hand terminal to wizard.
					m.pickingModel = false
					m.addingProvider = true
					fn := m.cfg.AddProviderFn
					var result addProviderDoneMsg
					return m, tea.Exec(&addProviderExec{fn: fn, result: &result}, func(err error) tea.Msg {
						if err != nil {
							result.err = err
						}
						return result
					})
				}
				if m.pickerCursor < len(opts) {
					sel := opts[m.pickerCursor]
					m.cfg.Provider = sel.Provider
					m.cfg.Model = sel.Model
				}
				m.pickingModel = false
				return m, nil
			case tea.KeyRunes:
				switch string(msg.Runes) {
				case "k":
					if m.pickerCursor > 0 {
						m.pickerCursor--
					}
				case "j":
					if m.pickerCursor < total-1 {
						m.pickerCursor++
					}
				}
				return m, nil
			}
			return m, nil
		}

		// Turn selection mode navigation.
		if m.turnSelectMode {
			totalTurns := m.turn
			switch msg.Type {
			case tea.KeyEsc:
				m.turnSelectMode = false
				m.selectedTurn = -1
				m.rebuildHistory()
				m.refreshViewport()
				return m, nil
			case tea.KeyUp:
				if m.selectedTurn > 0 {
					m.selectedTurn--
					m.refreshViewport()
				}
				return m, nil
			case tea.KeyDown:
				if m.selectedTurn < totalTurns-1 {
					m.selectedTurn++
					m.refreshViewport()
				}
				return m, nil
			case tea.KeyRunes:
				switch string(msg.Runes) {
				case "k":
					if m.selectedTurn > 0 {
						m.selectedTurn--
						m.refreshViewport()
					}
					return m, nil
				case "j":
					if m.selectedTurn < totalTurns-1 {
						m.selectedTurn++
						m.refreshViewport()
					}
					return m, nil
				case "d":
					// Dismiss: set mark and trigger async summary generation.
					if m.selectedTurn >= 0 && m.conv != nil {
						turnIdx := m.selectedTurn
						m.conv.SetMark(turnIdx, agent.TurnMarkDismissed, "")
						m.turnSelectMode = false
						m.selectedTurn = -1
						m.rebuildHistory()
						m.refreshViewport()
						return m, m.generateTurnSummary(m.ctx, turnIdx)
					}
					return m, nil
				case "u":
					// Mark as useful.
					if m.selectedTurn >= 0 && m.conv != nil {
						m.conv.SetMark(m.selectedTurn, agent.TurnMarkUseful, "")
						m.persistTurnMarks()
					}
					m.turnSelectMode = false
					m.selectedTurn = -1
					m.rebuildHistory()
					m.refreshViewport()
					return m, nil
				case "c":
					m.turnSelectMode = false
					m.selectedTurn = -1
					m.rebuildHistory()
					m.refreshViewport()
					return m, nil
				}
			}
			return m, nil
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
			if m.suspended {
				input := strings.TrimSpace(m.textarea.Value())
				if input != "" {
					m.textarea.Reset()
					m.textarea.Placeholder = "Ask anything… (Enter ↵ send  Ctrl+D exit)"
					m.suspended = false
					m.agentRunning = true
					m.status = "resuming…"
					m.resumeCh <- input
				}
				return m, nil
			}
			if !m.agentRunning {
				prompt := strings.TrimSpace(m.textarea.Value())
				if prompt != "" {
					m.textarea.Reset()
					// intercept /model command
					if prompt == "/model" {
						if len(m.cfg.ProviderOptions) > 0 {
							// pre-select current
							for i, o := range m.cfg.ProviderOptions {
								if o.Provider.Name() == m.cfg.Provider.Name() && o.Model == m.cfg.Model {
									m.pickerCursor = i
									break
								}
							}
							m.pickingModel = true
						}
						return m, nil
					}
					// intercept /pause command when agent is running
					if prompt == "/pause" && m.agentRunning && m.cancelRun != nil {
						m.cancelRun()
						return m, nil
					}
					// intercept /task and /question commands
					if m.cfg.SessionManager != nil {
						if strings.HasPrefix(prompt, "/task ") || strings.HasPrefix(prompt, "/question ") {
							return m, m.startWorkItem(prompt)
						}
					}
					return m, m.startAgent(prompt)
				}
			}

		case tea.KeyCtrlT:
			m.showTools = !m.showTools
			m.refreshViewport()
			return m, nil

		case tea.KeyCtrlX:
			// Enter/exit turn selection mode (when there are completed turns).
			if !m.agentRunning && m.turn > 0 {
				if m.turnSelectMode {
					m.turnSelectMode = false
					m.selectedTurn = -1
					m.rebuildHistory()
				} else {
					m.turnSelectMode = true
					m.selectedTurn = m.turn - 1 // start at most recent turn
					m.rebuildHistory()
				}
				m.refreshViewport()
				return m, nil
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
			if m.acActive {
				m.acActive = false
				m.acItems = nil
				return m, nil
			}
			if m.awaitingApproval {
				m.awaitingApproval = false
				m.approvalResCh <- agent.ApprovalDeny
				m.status = "ready"
				return m, nil
			}

		case tea.KeyTab:
			if !m.awaitingApproval && !m.agentRunning {
				if m.acActive && len(m.acItems) > 0 {
					m.acApply(m.acItems[m.acCursor])
					m.acActive = false
					m.acItems = nil
					return m, nil
				}
				// trigger autocomplete
				items := m.acComputeItems(m.textarea.Value())
				if len(items) > 0 {
					m.acItems = items
					m.acCursor = 0
					m.acActive = true
					return m, nil
				}
			}

		case tea.KeyUp:
			if m.acActive {
				if m.acCursor > 0 {
					m.acCursor--
				}
				return m, nil
			}

		case tea.KeyDown:
			if m.acActive {
				if m.acCursor < len(m.acItems)-1 {
					m.acCursor++
				}
				return m, nil
			}
		}

		if !m.awaitingApproval && !m.agentRunning {
			var taCmd tea.Cmd
			m.textarea, taCmd = m.textarea.Update(msg)
			cmds = append(cmds, taCmd)
			// update autocomplete after every keystroke
			if !m.acActive {
				items := m.acComputeItems(m.textarea.Value())
				if len(items) > 0 {
					m.acItems = items
					m.acCursor = 0
					m.acActive = true
				}
			} else {
				// recompute and close if no matches
				items := m.acComputeItems(m.textarea.Value())
				if len(items) == 0 {
					m.acActive = false
					m.acItems = nil
				} else {
					m.acItems = items
					if m.acCursor >= len(items) {
						m.acCursor = 0
					}
				}
			}
		}

	case spinner.TickMsg:
		var spCmd tea.Cmd
		m.spinner, spCmd = m.spinner.Update(msg)
		cmds = append(cmds, spCmd)

	case agentDeltaMsg:
		m.currentDelta += msg.delta
		m.refreshViewport()
		m.viewport.GotoBottom()
		cmds = append(cmds, drainChan(msg))

	case toolCallMsg:
		if msg.done {
			for i := len(m.toolCalls) - 1; i >= 0; i-- {
				if m.toolCalls[i].name == msg.name && !m.toolCalls[i].done {
					m.toolCalls[i].done = true
					m.toolCalls[i].isError = msg.isError
					m.toolCalls[i].output = msg.output
					m.toolCalls[i].dur = msg.dur
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

	case addProviderDoneMsg:
		m.addingProvider = false
		if msg.err != nil {
			m.appendHistory("error", "add provider: "+msg.err.Error(), provider.TokenUsage{}, 0)
		} else if len(msg.opts) > 0 {
			m.cfg.ProviderOptions = msg.opts
			// Reopen the picker so the user can choose among all providers.
			m.pickerCursor = len(msg.opts) - 1 // pre-select the newly added one
			m.pickingModel = true
		}
		m.refreshViewport()

	case turnSummaryDoneMsg:
		if msg.err == nil && msg.summary != "" && m.conv != nil {
			m.conv.SetMark(msg.turnIdx, agent.TurnMarkDismissed, msg.summary)
			// Update the historyEntry summary for rendering.
			for i := range m.historyEntries {
				if m.historyEntries[i].turnIdx == msg.turnIdx {
					m.historyEntries[i].text = msg.summary // store summary as text for dismissed entry
				}
			}
			m.persistTurnMarks()
			// Track latest summary for skill extraction at work-item end.
			m.lastWorkItemSummary = msg.summary
		} else if msg.err != nil && m.conv != nil {
			// Summary failed — keep the mark but clear it so content is shown.
			m.conv.SetMark(msg.turnIdx, agent.TurnMarkNone, "")
		}
		m.refreshViewport()

	case agentSuspendedMsg:
		m.suspended = true
		m.agentRunning = false
		m.pendingSuspend = msg.signal
		m.status = "waiting for guidance…"
		banner := "⏸  agent suspended: " + msg.signal.Preview
		m.appendHistory("system", banner, provider.TokenUsage{}, 0)
		m.textarea.Placeholder = "Type guidance to resume the agent…"
		m.refreshViewport()
		m.viewport.GotoBottom()
		return m, textarea.Blink

	case agentDoneMsg:
		m.agentRunning = false
		m.cancelRun = nil
		m.liveTokens += msg.tokensUsed.TotalTokens
		m.liveCost += msg.costUSD
		if len(m.toolCalls) > 0 {
			m.appendToolSummary()
			m.toolCalls = nil
		}
		if m.currentDelta != "" {
			m.appendHistory("assistant", m.currentDelta, msg.tokensUsed, msg.costUSD)
			m.currentDelta = ""
		}
		if msg.err != nil {
			m.appendHistory("error", friendlyError(msg.err.Error()), provider.TokenUsage{}, 0)
		}
		// Finish active work item (non-blocking).
		if m.activeWorkItemID != "" && m.cfg.SessionManager != nil {
			id := m.activeWorkItemID
			go func() { _ = m.cfg.SessionManager.Finish(m.ctx, id) }()
			// Extract skills learned during this work item.
			if m.cfg.SkillExtractor != nil {
				history := m.buildTurnHistoryText()
				summary := m.lastWorkItemSummary
				workDir := m.cfg.WorkDir
				extractor := m.cfg.SkillExtractor
				go func() {
					ctx2, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					n, err := extractor.Extract(ctx2, history, summary, workDir)
					if err != nil {
						slog.Warn("skill extraction failed", "err", err)
					} else if n > 0 {
						slog.Info("skills extracted", "count", n)
					}
				}()
			}
			m.activeWorkItemID = ""
			m.lastWorkItemSummary = ""
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

	toolsIndicator := ""
	if !m.showTools {
		toolsIndicator = styleDim.Render("  [tools hidden]")
	}
	left := wordmark + "  " + styleMuted.Render(dir) + styleDim.Render(" • ") + styleMuted.Render(model) + toolsIndicator

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
		statPlain := " " + truncate(m.status, maxW-40)

		// right side: elapsed + live tokens
		var rightParts []string
		if !m.startedAt.IsZero() {
			rightParts = append(rightParts, fmtDuration(time.Since(m.startedAt)))
		}
		if m.liveTokens > 0 {
			rightParts = append(rightParts, fmtTokens(m.liveTokens))
		}
		rightPlain := ""
		if len(rightParts) > 0 {
			rightPlain = " " + strings.Join(rightParts, " · ") + " "
		}

		left := styleStatusBg.Render(sp) + styleStatusBg.Render("Running") + styleMuted.Render(statPlain)
		right := styleDim.Render(rightPlain)
		pad := m.width - lipgloss.Width(left) - lipgloss.Width(right)
		if pad < 0 {
			pad = 0
		}
		return left + strings.Repeat(" ", pad) + right

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
		if m.turnSelectMode {
			sel := styleWarning.Render(fmt.Sprintf("● Turn %d/%d", m.selectedTurn+1, m.turn))
			keys := styleDim.Render(" ↑↓=navigate  d=dismiss  u=useful  c/Esc=cancel ")
			pad := m.width - lipgloss.Width(sel) - lipgloss.Width(keys)
			if pad < 0 {
				pad = 0
			}
			return sel + strings.Repeat(" ", pad) + keys
		}
		ready := styleStatusBg.Render("● Ready")
		keys := styleDim.Render(" ↑↓ scroll  Ctrl+T tools  Ctrl+X marks  Ctrl+D exit ")
		pad := m.width - lipgloss.Width(ready) - lipgloss.Width(keys)
		if pad < 0 {
			pad = 0
		}
		return ready + strings.Repeat(" ", pad) + keys
	}
}

func (m model) renderModelPicker() string {
	opts := m.cfg.ProviderOptions
	hasAdd := m.cfg.AddProviderFn != nil
	var sb strings.Builder
	sb.WriteString(styleSystem.Render("/model") + styleDim.Render("  ↑↓ j/k · Enter · Esc=cancel") + "\n")
	for i, o := range opts {
		if i == m.pickerCursor {
			sb.WriteString(styleAssistant.Render("▶ " + o.Label))
		} else {
			sb.WriteString(styleDim.Render("  " + o.Label))
		}
		sb.WriteString("\n")
	}
	if hasAdd {
		addLabel := "＋ Add new provider…"
		if m.pickerCursor == len(opts) {
			sb.WriteString(styleAssistant.Render("▶ " + addLabel))
		} else {
			sb.WriteString(styleDim.Render("  " + addLabel))
		}
	}
	return styleApprovalBorder.Width(m.width - 2).Render(sb.String())
}

func (m model) renderAutocompleteDropdown() string {
	var sb strings.Builder
	for i, item := range m.acItems {
		label := item
		desc := ""
		if idx := strings.Index(item, "\t"); idx >= 0 {
			label = item[:idx]
			desc = item[idx+1:]
		}
		if i > 0 {
			sb.WriteString("\n")
		}
		if i == m.acCursor {
			sb.WriteString(styleAssistant.Render("▶ " + label))
			if desc != "" {
				sb.WriteString(styleDim.Render("  " + desc))
			}
		} else {
			sb.WriteString(styleDim.Render("  " + label))
			if desc != "" {
				sb.WriteString(styleDim.Render("  " + desc))
			}
		}
	}
	hint := styleDim.Render("Tab=confirm  ↑↓=navigate  Esc=close")
	return styleBorder.Width(m.width - 2).Render(sb.String() + "\n" + hint)
}

func (m model) renderInput() string {
	if m.pickingModel {
		return m.renderModelPicker()
	}
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
	if m.suspended {
		return styleBorder.Width(m.width - 2).Render(m.textarea.View())
	}
	inputBox := styleBorder.Width(m.width - 2).Render(m.textarea.View())
	if m.acActive && len(m.acItems) > 0 {
		return lipgloss.JoinVertical(lipgloss.Left, m.renderAutocompleteDropdown(), inputBox)
	}
	return inputBox
}

// ── agent start ───────────────────────────────────────────────────────────────

func (m *model) startAgent(prompt string) tea.Cmd {
	m.agentRunning = true
	m.currentDelta = ""
	m.toolCalls = nil
	m.startedAt = time.Now()
	m.liveTokens = 0
	m.liveCost = 0
	m.appendHistory("user", prompt, provider.TokenUsage{}, 0)
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
	suspendCh := make(chan agent.SuspendSignal, 1)
	resumeCh := make(chan string, 1)
	m.suspendCh = suspendCh
	m.resumeCh = resumeCh

	// Initialise a checkpoint recorder for this run (non-fatal if it fails).
	var ckptRecorder *checkpoint.Recorder
	ckptDir := filepath.Join(m.cfg.WorkDir, ".polvo", "checkpoints")
	ckptStore := checkpoint.NewFSStore(ckptDir)
	ckptSessionID := fmt.Sprintf("tui-%d", time.Now().UnixNano())
	if rec, err := checkpoint.NewRecorder(ckptStore, ckptSessionID, "tui"); err == nil {
		ckptRecorder = rec
	}

	// Load microagents for context injection.
	homeDir, _ := os.UserHomeDir()
	maLoader := microagent.NewLoader(
		filepath.Join(m.cfg.WorkDir, ".polvo", "microagents"),
		filepath.Join(homeDir, ".polvo", "microagents"),
	)

	model := m.cfg.Model

	system := m.cfg.System
	if m.cfg.SummaryModel == "" {
		// No dedicated summary model — ask the main model to include inline summaries.
		system += agent.InlineSummarySystemSuffix
	}

	loopCfg := agent.LoopConfig{
		Provider:           m.cfg.Provider,
		Tools:              m.cfg.ToolReg,
		System:             system,
		Model:              model,
		MaxTurns:           maxTurns,
		PermissionCallback: m.perm,
		Checkpoint:         ckptRecorder,
		MicroagentLoader:   maLoader,
		SuspendCh:          suspendCh,
		ResumeCh:           resumeCh,
		InlineSummary:      m.cfg.SummaryModel == "",
		OnText: func(text string) {
			// Called when the provider does NOT support streaming (full response at once).
			// Split into small chunks so the delta path is used for display.
			select {
			case deltaCh <- text:
			case <-ctx.Done():
			}
		},
		OnTextDelta: func(d string) {
			select {
			case deltaCh <- d:
			case <-ctx.Done():
			}
		},
		OnToolCall: func(c provider.ToolCall) {
			toolCallCh <- toolCallMsg{
				name:    c.Name,
				preview: formatToolInput(c.Name, c.Input),
			}
		},
		OnToolResult: func(_, name, result string, isError bool) {
			// find start time from pending entry to compute duration
			toolCallCh <- toolCallMsg{
				name:    name,
				done:    true,
				isError: isError,
				output:  firstMeaningfulLine(result, 120),
			}
		},
		OnTurnSummary: func(turnIdx int, summary string) {
			prog.Send(turnSummaryDoneMsg{turnIdx: turnIdx, summary: summary})
		},
	}

	l := agent.NewLoop(loopCfg)

	// Expose conversation and checkpoint store for turn mark operations.
	m.conv = l.Conv()
	m.ckptStore = ckptStore
	m.ckptSessionID = ckptSessionID

	// Resolve @@task[...] / @@question[...] references before sending to the model.
	var resolver *session.Resolver
	if m.cfg.SessionManager != nil {
		resolver = session.NewResolver(m.cfg.SessionManager, m.cfg.SummaryRunner)
	}

	go func() {
		resolvedPrompt := prompt
		if resolver != nil && session.HasRefs(prompt) {
			if rp, rerr := resolver.Resolve(ctx, prompt); rerr == nil {
				resolvedPrompt = rp
			}
		}
		res, err := l.Run(ctx, resolvedPrompt)
		if ckptRecorder != nil {
			status := "completed"
			if err != nil {
				status = "failed"
			}
			_ = ckptRecorder.Finish(status)
		}
		done := agentDoneMsg{err: err}
		if res != nil {
			done.tokensUsed = res.TokensUsed
			done.costUSD = provider.ComputeCostUSD(res.TokensUsed, model)
		}
		doneCh <- done
	}()

	go func() {
		defer cancel()
		// track start times per tool name to compute duration
		startTimes := map[string]time.Time{}
		// batch deltas every 50ms to avoid saturating the bubbletea event loop
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		var pending strings.Builder
		flush := func() {
			if pending.Len() > 0 {
				prog.Send(agentDeltaMsg{delta: pending.String()})
				pending.Reset()
			}
		}
		for {
			select {
			case <-ctx.Done():
				// Context cancelled — drain remaining deltas and exit.
				flush()
				return
			case d, ok := <-deltaCh:
				if ok {
					pending.WriteString(d)
				}
			case <-ticker.C:
				flush()
			case sig, ok := <-suspendCh:
				if ok {
					flush()
					prog.Send(agentSuspendedMsg{signal: sig})
				}
			case tc, ok := <-toolCallCh:
				if ok {
					flush() // send any buffered delta before tool msg
					if !tc.done {
						startTimes[tc.name] = time.Now()
					} else {
						if st, found := startTimes[tc.name]; found {
							tc.dur = time.Since(st)
							delete(startTimes, tc.name)
						}
					}
					prog.Send(tc)
				}
			case done := <-doneCh:
				// Drain remaining deltas before signalling done.
			drainLoop:
				for {
					select {
					case d := <-deltaCh:
						pending.WriteString(d)
					default:
						break drainLoop
					}
				}
				flush()
				prog.Send(done)
				return
			}
		}
	}()

	return nil
}

// ── work item commands ────────────────────────────────────────────────────────

// startWorkItem handles /task <prompt> and /question <prompt> commands.
// It finishes the active work item, creates a new one, resets conversation
// context, and starts the agent with the given prompt.
func (m *model) startWorkItem(cmd string) tea.Cmd {
	var kind session.Kind
	var body string
	if after, ok := strings.CutPrefix(cmd, "/task "); ok {
		kind = session.KindTask
		body = strings.TrimSpace(after)
	} else if after, ok := strings.CutPrefix(cmd, "/question "); ok {
		kind = session.KindQuestion
		body = strings.TrimSpace(after)
	} else {
		return nil
	}
	if body == "" {
		return nil
	}

	ctx := m.ctx

	// Finish any active work item.
	if m.activeWorkItemID != "" {
		_ = m.cfg.SessionManager.Finish(ctx, m.activeWorkItemID)
	}

	// Create new work item.
	wi, err := m.cfg.SessionManager.Start(ctx, kind, body)
	if err != nil {
		m.appendHistory("error", "work item: "+err.Error(), provider.TokenUsage{}, 0)
		m.refreshViewport()
		return nil
	}
	m.activeWorkItemID = wi.ID

	// Start async summary.
	if m.cfg.SummaryRunner != nil {
		m.cfg.SummaryRunner.StartAsync(ctx, wi)
	}

	// Reset conversation context.
	m.history = ""
	m.turn = 0
	m.liveTokens = 0
	m.liveCost = 0
	m.toolCalls = nil
	m.currentDelta = ""
	m.historyEntries = nil
	m.conv = nil
	m.turnSelectMode = false
	m.selectedTurn = -1

	// Banner in history.
	bannerText := string(kind) + " " + wi.ID + ": " + body
	m.appendHistory("system", bannerText, provider.TokenUsage{}, 0)
	m.refreshViewport()

	return m.startAgent(body)
}


// ── turn marks ────────────────────────────────────────────────────────────────

// generateTurnSummary returns a tea.Cmd that generates a summary for the given
// turn asynchronously, using the dedicated summary model when configured or the
// main model otherwise.
func (m *model) generateTurnSummary(ctx context.Context, turnIdx int) tea.Cmd {
	if m.conv == nil {
		return nil
	}
	userText := m.conv.TurnUserContent(turnIdx)
	assistantText := m.conv.TurnAssistantContent(turnIdx)
	if userText == "" && assistantText == "" {
		return nil
	}

	p := m.cfg.Provider
	mdl := m.cfg.Model
	if m.cfg.SummaryProvider != nil && m.cfg.SummaryModel != "" {
		p = m.cfg.SummaryProvider
		mdl = m.cfg.SummaryModel
	}

	return func() tea.Msg {
		summary, err := agent.SummarizeTurn(ctx, p, mdl, userText, assistantText)
		return turnSummaryDoneMsg{turnIdx: turnIdx, summary: summary, err: err}
	}
}

// hasAnyMarks returns true if any completed turn has a non-default mark.

// buildTurnHistoryText returns a plain-text concatenation of user and assistant
// messages from historyEntries, suitable as input for skill extraction.
func (m model) buildTurnHistoryText() string {
	var sb strings.Builder
	for _, e := range m.historyEntries {
		switch e.role {
		case "user":
			sb.WriteString("User: " + e.text + "\n")
		case "assistant":
			sb.WriteString("Assistant: " + e.text + "\n")
		}
	}
	return sb.String()
}

// persistTurnMarks saves all current turn marks to the checkpoint store.
func (m *model) persistTurnMarks() {
	if m.ckptStore == nil || m.ckptSessionID == "" || m.conv == nil {
		return
	}
	var records []checkpoint.TurnMarkRecord
	for i := 0; i < m.conv.TurnCount(); i++ {
		meta := m.conv.GetMark(i)
		if meta.Mark != agent.TurnMarkNone {
			records = append(records, checkpoint.TurnMarkRecord{
				TurnIndex: i,
				Mark:      int8(meta.Mark),
				Summary:   meta.Summary,
			})
		}
	}
	_ = m.ckptStore.SaveTurnMarks(m.ckptSessionID, records)
}

// ── history rendering ─────────────────────────────────────────────────────────

func (m *model) appendHistory(role, text string, tokens provider.TokenUsage, costUSD float64) {
	// Record structured entry before rendering.
	// user and assistant entries get a turnIdx; others get -1.
	turnIdx := -1
	switch role {
	case "user":
		// This user message will start turn m.turn (0-based).
		turnIdx = m.turn
	case "assistant":
		// This closes the turn that started with turn m.turn.
		turnIdx = m.turn
	}
	m.historyEntries = append(m.historyEntries, historyEntry{
		role:    role,
		text:    text,
		turnIdx: turnIdx,
		tokens:  tokens,
		cost:    costUSD,
	})

	w := m.width - 2 // -2 for left border + padding
	if w < 20 {
		w = 20
	}
	var line string
	switch role {
	case "user":
		label := styleUser.Render("You")
		body := styleMuted.Render(text)
		line = styleUserBlock.Width(w).Render(label + "\n" + body)
	case "assistant":
		label := styleAssistant.Render("Polvo")
		body := label + "\n" + text
		if tokens.TotalTokens > 0 {
			footer := fmtTokens(tokens.TotalTokens)
			if costUSD > 0 {
				footer += " · " + fmtCost(costUSD)
			}
			body += "\n" + styleDim.Render("✦ "+footer)
		}
		line = styleAssistantBlock.Width(w).Render(body)
		m.turn++
	case "error":
		label := styleError.Render("Error")
		line = styleErrorBlock.Width(w).Render(label + "\n" + styleError.Render(text))
	case "system":
		line = styleErrorBlock.Width(w).Render(styleDim.Render("· " + text))
	case "tools":
		label := styleSystem.Render("Tools Used")
		line = styleToolBlock.Width(w).Render(label + "\n" + text)
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
	text := m.renderToolList(m.toolCalls, false)
	m.appendHistory("tools", text, provider.TokenUsage{}, 0)
}

// renderToolList renders the tool call list for display.
// When collapsed (showTools=false), shows only a compact summary line.
func (m *model) renderToolList(calls []toolEntry, hasPending bool) string {
	if !m.showTools {
		// collapsed: show count + names inline
		names := make([]string, 0, len(calls))
		for _, tc := range calls {
			names = append(names, tc.name)
		}
		toggle := styleDim.Render(" Ctrl+T to expand")
		return styleSystem.Render(fmt.Sprintf("%d tools", len(calls))) +
			styleDim.Render("  "+strings.Join(names, " › ")) + toggle
	}

	var sb strings.Builder
	for i, tc := range calls {
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

		if tc.dur > 0 {
			sb.WriteString(styleDim.Render("  " + fmtDuration(tc.dur)))
		}

		if tc.done && tc.output != "" {
			sb.WriteString("\n")
			if tc.isError {
				sb.WriteString(styleError.Render("  " + truncate(tc.output, 100)))
			} else {
				sb.WriteString(styleDim.Render("  " + truncate(tc.output, 100)))
			}
		}
	}
	return sb.String()
}

// rebuildHistory reconstructs the history string from historyEntries, applying
// turn mark visuals (selected, useful, dismissed). Called when marks or selection
// may have changed.
func (m *model) rebuildHistory() {
	w := m.width - 2
	if w < 20 {
		w = 20
	}
	var sb strings.Builder
	// Track whether we already emitted a dismissed stub for this turn.
	dismissedSeen := make(map[int]bool)

	for _, e := range m.historyEntries {
		var line string

		if e.turnIdx >= 0 && m.conv != nil {
			meta := m.conv.GetMark(e.turnIdx)
			dismissed := meta.Mark == agent.TurnMarkDismissed
			useful := meta.Mark == agent.TurnMarkUseful
			selected := m.turnSelectMode && m.selectedTurn == e.turnIdx

			if dismissed {
				if dismissedSeen[e.turnIdx] {
					continue // only emit stub once per turn
				}
				dismissedSeen[e.turnIdx] = true
				summary := meta.Summary
				if summary == "" {
					summary = "generating summary…"
				}
				stub := styleDim.Render(fmt.Sprintf("[turn %d dismissed · %s]", e.turnIdx+1, truncate(summary, 60)))
				line = styleDismissedBlock.Width(w).Render(stub)
			} else {
				// Render normally, with mark-aware style.
				line = m.renderEntry(e, w)
				if selected {
					line = styleSelectedBlock.Width(w).Render(
						styleWarning.Render("▶ ") + m.renderEntryInner(e),
					)
				} else if useful {
					line = styleUsefulBlock.Width(w).Render(
						styleWarning.Render("★ ") + m.renderEntryInner(e),
					)
				}
			}
		} else {
			line = m.renderEntry(e, w)
		}

		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(line)
	}
	m.history = sb.String()
}

// renderEntryInner renders the inner content of a history entry (no block border).
func (m *model) renderEntryInner(e historyEntry) string {
	switch e.role {
	case "user":
		return styleUser.Render("You") + "\n" + styleMuted.Render(e.text)
	case "assistant":
		body := styleAssistant.Render("Polvo") + "\n" + e.text
		if e.tokens.TotalTokens > 0 {
			footer := fmtTokens(e.tokens.TotalTokens)
			if e.cost > 0 {
				footer += " · " + fmtCost(e.cost)
			}
			body += "\n" + styleDim.Render("✦ "+footer)
		}
		return body
	default:
		return e.text
	}
}

// renderEntry renders a history entry into a styled block string.
func (m *model) renderEntry(e historyEntry, w int) string {
	switch e.role {
	case "user":
		label := styleUser.Render("You")
		body := styleMuted.Render(e.text)
		return styleUserBlock.Width(w).Render(label + "\n" + body)
	case "assistant":
		label := styleAssistant.Render("Polvo")
		body := label + "\n" + e.text
		if e.tokens.TotalTokens > 0 {
			footer := fmtTokens(e.tokens.TotalTokens)
			if e.cost > 0 {
				footer += " · " + fmtCost(e.cost)
			}
			body += "\n" + styleDim.Render("✦ "+footer)
		}
		return styleAssistantBlock.Width(w).Render(body)
	case "error":
		label := styleError.Render("Error")
		return styleErrorBlock.Width(w).Render(label + "\n" + styleError.Render(e.text))
	case "system":
		return styleErrorBlock.Width(w).Render(styleDim.Render("· " + e.text))
	case "tools":
		label := styleSystem.Render("Tools Used")
		return styleToolBlock.Width(w).Render(label + "\n" + e.text)
	}
	return e.text
}

func (m *model) refreshViewport() {
	content := m.history

	// live tool calls: show Running block while any tool is still pending
	if len(m.toolCalls) > 0 {
		hasPending := false
		for _, tc := range m.toolCalls {
			if !tc.done {
				hasPending = true
				break
			}
		}

		if content != "" {
			content += "\n"
		}
		label := styleSystem.Render("Running")
		if !hasPending {
			label = styleSystem.Render("Tools Used")
		}
		w := m.width - 2
		if w < 20 {
			w = 20
		}
		content += styleToolBlock.Width(w).Render(label + "\n" + m.renderToolList(m.toolCalls, hasPending))
	}

	// streaming assistant response — only show after all tools are done
	if m.currentDelta != "" {
		allDone := true
		for _, tc := range m.toolCalls {
			if !tc.done {
				allDone = false
				break
			}
		}
		if allDone {
			if content != "" {
				content += "\n"
			}
			label := styleAssistant.Render("Polvo")
			w := m.width - 2
			if w < 20 {
				w = 20
			}
			content += styleAssistantBlock.Width(w).Render(label + "\n" + m.currentDelta)
		}
	}

	m.viewport.SetContent(content)
}

// ── autocomplete ─────────────────────────────────────────────────────────────

// slashCommands lists all available slash commands with their descriptions.
var slashCommands = []struct{ cmd, desc string }{
	{"/model", "switch provider or model"},
	{"/task", "start a new task (resets context)"},
	{"/question", "start a new question (resets context)"},
	{"/pause", "suspend the agent and wait for guidance"},
	{"/clear", "clear conversation history"},
	{"/help", "show keyboard shortcuts"},
}

// acComputeItems returns completion candidates for the current input value.
// Returns nil (no autocomplete) when the input doesn't match a trigger.
func (m *model) acComputeItems(val string) []string {
	// slash command completion: triggered when input starts with "/"
	if strings.HasPrefix(val, "/") && !strings.Contains(val, " ") {
		prefix := strings.ToLower(val)
		var out []string
		for _, sc := range slashCommands {
			if strings.HasPrefix(sc.cmd, prefix) {
				out = append(out, sc.cmd+"\t"+sc.desc)
			}
		}
		return out
	}

	// @@ work item completion: triggered after "@@"
	if m.cfg.SessionManager != nil {
		if aaIdx := strings.LastIndex(val, "@@"); aaIdx >= 0 {
			partial := val[aaIdx+2:]
			if !strings.Contains(partial, "]") {
				// strip leading @@task[ or @@question[ prefix
				kindPrefix := ""
				if strings.HasPrefix(partial, "task[") {
					kindPrefix = "task["
					partial = partial[5:]
				} else if strings.HasPrefix(partial, "question[") {
					kindPrefix = "question["
					partial = partial[9:]
				}
				items, _ := m.cfg.SessionManager.ListRecent(context.Background(), 20)
				var out []string
				for _, wi := range items {
					if kindPrefix != "" && !strings.HasPrefix(wi.ID, strings.TrimSuffix(kindPrefix, "[")) {
						continue
					}
					if strings.HasPrefix(wi.ID, string(wi.Kind)+"#"+partial) || partial == "" {
						desc := wi.Summary
						if desc == "" {
							desc = wi.Prompt
						}
						if len(desc) > 60 {
							desc = desc[:57] + "…"
						}
						out = append(out, "@@"+string(wi.Kind)+"["+wi.ID+"]\t"+desc)
					}
				}
				if len(out) > 0 {
					return out
				}
			}
		}
	}

	// @ file path completion: triggered after the last "@" (but not "@@")
	atIdx := strings.LastIndex(val, "@")
	if atIdx < 0 {
		return nil
	}
	// skip if this "@" is actually part of "@@"
	if atIdx > 0 && val[atIdx-1] == '@' {
		return nil
	}
	if atIdx+1 < len(val) && val[atIdx+1] == '@' {
		return nil
	}
	partial := val[atIdx+1:]
	if strings.Contains(partial, " ") {
		return nil // already completed (space after path)
	}

	pattern := filepath.Join(m.cfg.WorkDir, partial+"*")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return nil
	}
	sort.Strings(matches)
	if len(matches) > 12 {
		matches = matches[:12]
	}
	out := make([]string, len(matches))
	for i, p := range matches {
		rel, err := filepath.Rel(m.cfg.WorkDir, p)
		if err != nil {
			rel = p
		}
		// append "/" to directories for clarity
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			rel += "/"
		}
		out[i] = rel
	}
	return out
}

// acApply replaces the relevant portion of the input with the selected item.
func (m *model) acApply(item string) {
	// strip description from slash commands and @@ refs (tab-separated)
	if idx := strings.Index(item, "\t"); idx >= 0 {
		item = item[:idx]
	}
	val := m.textarea.Value()

	if strings.HasPrefix(val, "/") {
		m.textarea.SetValue(item + " ")
		return
	}

	// @@ work item ref: item looks like "@@task[task#01]"
	if strings.HasPrefix(item, "@@") {
		aaIdx := strings.LastIndex(val, "@@")
		if aaIdx >= 0 {
			m.textarea.SetValue(val[:aaIdx] + item)
		}
		return
	}

	atIdx := strings.LastIndex(val, "@")
	if atIdx >= 0 {
		m.textarea.SetValue(val[:atIdx+1] + item)
	}
}

// ── tool input formatting ─────────────────────────────────────────────────────

// formatToolInput extracts the most meaningful field from a tool's JSON input
// to display a clean, human-readable summary instead of raw JSON.
func formatToolInput(name string, input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(input, &fields); err != nil {
		return truncate(string(input), 80)
	}

	// primary field by tool name
	primary := map[string]string{
		"bash":       "command",
		"read":       "file_path",
		"write":      "file_path",
		"edit":       "file_path",
		"glob":       "pattern",
		"grep":       "pattern",
		"ls":         "path",
		"web_fetch":  "url",
		"web_search": "query",
		"think":      "thought",
		"diff":       "file_path",
		"patch":      "file_path",
	}

	if key, ok := primary[name]; ok {
		if raw, found := fields[key]; found {
			var s string
			if err := json.Unmarshal(raw, &s); err == nil && s != "" {
				return truncate(s, 80)
			}
		}
		// field exists in schema but was empty/absent — show workdir placeholder
		return "./"
	}

	// fallback: first non-empty string value
	for _, raw := range fields {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil && s != "" {
			return truncate(s, 80)
		}
	}

	// no displayable fields
	return ""
}

// firstMeaningfulLine returns the first non-empty, non-whitespace line of s,
// truncated to maxLen chars. Returns empty string if s is empty or error-like boilerplate.
func firstMeaningfulLine(s string, maxLen int) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		return truncate(line, maxLen)
	}
	return ""
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
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}

func fmtTokens(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk tokens", float64(n)/1000)
	}
	return fmt.Sprintf("%d tokens", n)
}

func fmtCost(usd float64) string {
	if usd < 0.001 {
		return fmt.Sprintf("$%.4f", usd)
	}
	return fmt.Sprintf("$%.3f", usd)
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
	// network / TCP timeouts
	if strings.Contains(msg, "operation timed out") || strings.Contains(msg, "i/o timeout") || strings.Contains(msg, "read tcp") {
		return "connection timed out — the provider did not respond in time, try again"
	}
	// generic connection errors
	if strings.Contains(msg, "connection refused") || strings.Contains(msg, "no such host") || strings.Contains(msg, "dial tcp") {
		return "could not reach provider — check your internet connection"
	}
	// EOF during streaming
	if strings.Contains(msg, "unexpected EOF") || strings.Contains(msg, "EOF") {
		return "connection dropped mid-stream — try again"
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
