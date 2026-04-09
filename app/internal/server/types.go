package server

import (
	"encoding/json"
	"time"

	"github.com/co2-lab/polvo/internal/config"
	"github.com/co2-lab/polvo/internal/guide"
	"github.com/co2-lab/polvo/internal/provider"
)

// EventKind identifies the type of event streamed over SSE.
type EventKind string

const (
	EventAgentStarted EventKind = "agent_started"
	EventAgentDone    EventKind = "agent_done"
	EventWatchStarted EventKind = "watch_started"
	EventWatchStopped EventKind = "watch_stopped"
	EventFileChanged  EventKind = "file_changed"
	EventPRCreated    EventKind = "pr_created"
	EventReportSaved  EventKind = "report_saved"
	EventLogLine      EventKind = "log"
	EventSnapshot     EventKind = "snapshot"
	EventChatToken    EventKind = "chat_token"
	EventChatDone     EventKind = "chat_done"
	EventChatError    EventKind = "chat_error"
)

// Event is a single dashboard event sent over SSE.
type Event struct {
	Kind    EventKind       `json:"kind"`
	Payload json.RawMessage `json:"payload"`
}

// AgentStatus holds the current state of a running or recently finished agent.
type AgentStatus struct {
	Name      string    `json:"name"`
	File      string    `json:"file"`
	StartedAt time.Time `json:"started_at"`
	Done      bool      `json:"done"`
	Error     string    `json:"error,omitempty"`
}

// ProviderStatus holds the health status of a single provider.
type ProviderStatus struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// StatusResponse is the payload for GET /api/status.
type StatusResponse struct {
	Project      string `json:"project"`
	ProjectColor string `json:"project_color,omitempty"`
	ProjectIcon  string `json:"project_icon,omitempty"`
	Version      string `json:"version"`
	CommitSHA    string `json:"commit_sha,omitempty"`
	BuildDate    string `json:"build_date,omitempty"`
	Cwd          string `json:"cwd"`
	Watching     bool   `json:"watching"`
	AgentRunning bool   `json:"agent_running"`
	DashboardURL string `json:"dashboard_url"`
}

// SnapshotPayload is sent to new SSE subscribers to give them current state.
type SnapshotPayload struct {
	Status    StatusResponse `json:"status"`
	Agents    []*AgentStatus `json:"agents"`
	RecentLog []string       `json:"recent_log"`
}

// AppDeps holds the application-level dependencies the dashboard needs.
type AppDeps struct {
	Version   string
	CommitSHA string
	BuildDate string
	Cfg      *config.Config
	Registry *provider.Registry
	Resolver *guide.Resolver
	// Watching and AgentRunning are read via callbacks to avoid coupling.
	IsWatching     func() bool
	IsAgentRunning func() bool
	OnConfigReload func(*config.Config)
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
