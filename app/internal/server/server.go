package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/co2-lab/polvo/web"
)

// Server is the local dashboard HTTP server.
type Server struct {
	addr      string
	bus       *Bus
	deps      *AppDeps
	reportDir string
	root      string // absolute path; FS API is restricted to this tree
	mux       *http.ServeMux
	term      *termState
	registry  *projectRegistry
}

// NewServer creates a dashboard server. reportDir is the path to .polvo/reports.
// root is the project working directory; all FS API paths are resolved relative to it.
// If root is empty, os.Getwd() is used.
func NewServer(addr string, bus *Bus, reportDir string, deps *AppDeps, root string) *Server {
	if root == "" {
		root, _ = os.Getwd()
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.TempDir(), "polvo")
	} else {
		configDir = filepath.Join(configDir, "polvo")
	}
	_ = os.MkdirAll(configDir, 0755)

	reg := newProjectRegistry(configDir)

	s := &Server{
		addr:      addr,
		bus:       bus,
		deps:      deps,
		reportDir: reportDir,
		root:      root,
		mux:       http.NewServeMux(),
		term:      newTermState(),
		registry:  reg,
	}

	// Auto-register the root project if a root was provided.
	if root != "" {
		reg.autoRegister(root)
	}

	s.routes()
	return s
}

// resolvePath cleans and validates that p stays within s.root.
// Returns the absolute path on success, or an error if traversal is detected.
func (s *Server) resolvePath(p string) (string, error) {
	abs := filepath.Clean(filepath.Join(s.root, p))
	if !strings.HasPrefix(abs, s.root+string(filepath.Separator)) && abs != s.root {
		return "", fmt.Errorf("path outside project root")
	}
	return abs, nil
}

// resolvePathForProject resolves p relative to the project identified by projectID.
// If projectID is empty or the project is not found, it falls back to s.root.
func (s *Server) resolvePathForProject(projectID, p string) (string, error) {
	root := s.root
	if projectID != "" {
		if proj, ok := s.registry.get(projectID); ok {
			root = proj.Path
		}
	}
	abs := filepath.Clean(filepath.Join(root, p))
	if !strings.HasPrefix(abs, root+string(filepath.Separator)) && abs != root {
		return "", fmt.Errorf("path outside project root")
	}
	return abs, nil
}

// corsMiddleware adds permissive CORS headers and handles preflight OPTIONS requests.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Start begins listening. Blocks until the server stops.
func (s *Server) Start() error {
	return http.ListenAndServe(s.addr, corsMiddleware(s.mux))
}

func (s *Server) routes() {
	s.mux.Handle("/", http.FileServer(http.FS(web.FS)))
	s.mux.HandleFunc("/api/status", s.handleStatus)
	s.mux.HandleFunc("/api/providers", s.handleProviders)
	s.mux.HandleFunc("/api/agents", s.handleAgents)
	s.mux.HandleFunc("/api/reports", s.handleReports)
	s.mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			s.handleSaveConfig(w, r)
		} else {
			s.handleGetConfig(w, r)
		}
	})
	s.mux.HandleFunc("/api/projects", s.handleProjects)
	s.mux.HandleFunc("/api/projects/", s.handleProjectByID)
	s.mux.HandleFunc("/api/fs/list", s.handleFSList)
	s.mux.HandleFunc("/api/fs/read", s.handleFSRead)
	s.mux.HandleFunc("/api/fs/write", s.handleFSWrite)
	s.mux.HandleFunc("/api/fs/delete", s.handleFSDelete)
	s.mux.HandleFunc("/api/fs/rename", s.handleFSRename)
	s.mux.HandleFunc("/api/fs/mkdir", s.handleFSMkdir)
	s.mux.HandleFunc("/api/fs/reveal", s.handleFSReveal)
	s.mux.HandleFunc("/api/chat", s.handleChat)
	s.mux.HandleFunc("/api/diff", s.handleDiff)
	s.mux.HandleFunc("/api/diff/accept", s.handleDiffAccept)
	s.mux.HandleFunc("/api/diff/reject", s.handleDiffReject)
	s.mux.HandleFunc("/api/clis", s.handleCLIs)
	s.mux.HandleFunc("/api/init", s.handleInit)
	s.mux.HandleFunc("/api/models", s.handleModels)
	s.mux.HandleFunc("/api/doctor", s.handleDoctor)
	s.mux.HandleFunc("/api/doctor/fix", s.handleDoctorFix)
	s.mux.HandleFunc("/api/exit", s.handleExit)
	s.mux.HandleFunc("/events", s.handleSSE)
	s.registerTerminalRoutes()
}

// agentRunning returns true if any agent is currently not done.
func (s *Server) agentRunning() bool {
	for _, a := range s.bus.ActiveAgents() {
		if !a.Done {
			return true
		}
	}
	if s.deps.IsAgentRunning != nil {
		return s.deps.IsAgentRunning()
	}
	return false
}

// ReportSummary is a compact representation of a report for the dashboard.
type ReportSummary struct {
	ID        string `json:"id"`
	Agent     string `json:"agent"`
	File      string `json:"file"`
	Timestamp string `json:"timestamp"`
	Decision  string `json:"decision"`
	Severity  string `json:"severity"`
	Summary   string `json:"summary"`
}

// loadReports reads all report JSON files from reportDir, optionally filtered by file.
// Returns the 50 most recent reports.
func loadReports(reportDir, filterFile string) ([]ReportSummary, error) {
	if _, err := os.Stat(reportDir); err != nil {
		return []ReportSummary{}, nil
	}

	type rawReport struct {
		ID        string `json:"id"`
		Agent     string `json:"agent"`
		File      string `json:"file"`
		Timestamp string `json:"timestamp"`
		Decision  string `json:"decision"`
		Severity  string `json:"severity"`
		Summary   string `json:"summary"`
	}

	var reports []rawReport
	_ = filepath.WalkDir(reportDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".json" {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var r rawReport
		if json.Unmarshal(data, &r) != nil {
			return nil
		}
		if filterFile != "" && r.File != filterFile {
			return nil
		}
		reports = append(reports, r)
		return nil
	})

	sort.Slice(reports, func(i, j int) bool {
		return reports[i].Timestamp > reports[j].Timestamp
	})
	if len(reports) > 50 {
		reports = reports[:50]
	}

	out := make([]ReportSummary, len(reports))
	for i, r := range reports {
		out[i] = ReportSummary(r)
	}
	return out, nil
}

// PublishFileChanged publishes a file_changed event.
func (b *Bus) PublishFileChanged(path string) {
	b.Publish(Event{Kind: EventFileChanged, Payload: mustJSON(map[string]string{"path": path})})
}

// PublishLog is a helper to publish a log line event to the bus.
func (b *Bus) PublishLog(text string) {
	b.Publish(Event{
		Kind:    EventLogLine,
		Payload: mustJSON(map[string]string{"text": text}),
	})
}

// PublishAgentStarted publishes an agent_started event.
func (b *Bus) PublishAgentStarted(name, file string) {
	a := AgentStatus{Name: name, File: file}
	b.Publish(Event{
		Kind:    EventAgentStarted,
		Payload: mustJSON(a),
	})
}

// PublishAgentDone publishes an agent_done event.
func (b *Bus) PublishAgentDone(name, file string, errMsg string) {
	a := AgentStatus{Name: name, File: file, Done: true, Error: errMsg}
	b.Publish(Event{
		Kind:    EventAgentDone,
		Payload: mustJSON(a),
	})
}

// PublishWatchStarted publishes a watch_started event.
func (b *Bus) PublishWatchStarted() {
	b.Publish(Event{Kind: EventWatchStarted, Payload: mustJSON(struct{}{})})
}

// PublishWatchStopped publishes a watch_stopped event.
func (b *Bus) PublishWatchStopped() {
	b.Publish(Event{Kind: EventWatchStopped, Payload: mustJSON(struct{}{})})
}

// Addr returns the dashboard address string for display.
func (s *Server) Addr() string {
	return fmt.Sprintf("http://%s", s.addr)
}
