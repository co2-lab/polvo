package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/co2-lab/polvo/internal/config"
	"github.com/co2-lab/polvo/internal/provider"
)

var skipNames = map[string]bool{
	".git":        true,
	"node_modules": true,
	"target":      true,
	".polvo":      true,
	"bin":         true,
	".DS_Store":   true,
	"__pycache__": true,
	".venv":       true,
	"vendor":      true,
	"dist":        true,
	".next":       true,
	".nuxt":       true,
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	projectName, projectColor, projectIcon := "", "", ""
	if s.deps.Cfg != nil {
		if _, err := os.Stat("polvo.yaml"); err == nil {
			projectName = s.deps.Cfg.Project.Name
			projectColor = s.deps.Cfg.Project.Color
			projectIcon = s.deps.Cfg.Project.Icon
		}
	}
	cwd, _ := os.Getwd()
	resp := StatusResponse{
		Project:      projectName,
		ProjectColor: projectColor,
		ProjectIcon:  projectIcon,
		Version:      s.deps.Version,
		CommitSHA:    s.deps.CommitSHA,
		BuildDate:    s.deps.BuildDate,
		Cwd:          cwd,
		Watching:     s.bus.Watching(),
		AgentRunning: s.agentRunning(),
		DashboardURL: fmt.Sprintf("http://%s", s.addr),
	}
	writeJSON(w, resp)
}

func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request) {
	if s.deps.Registry == nil {
		writeJSON(w, []ProviderStatus{})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var statuses []ProviderStatus
	for name, p := range s.deps.Registry.All() {
		ps := ProviderStatus{Name: name}
		if s.deps.Cfg != nil {
			if pc, ok := s.deps.Cfg.Providers[name]; ok {
				ps.Type = pc.Type
			}
		}
		if err := p.Available(ctx); err != nil {
			ps.Error = err.Error()
		} else {
			ps.OK = true
		}
		statuses = append(statuses, ps)
	}
	writeJSON(w, statuses)
}

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	agents := s.bus.ActiveAgents()
	if agents == nil {
		agents = []*AgentStatus{}
	}
	writeJSON(w, agents)
}

func (s *Server) handleReports(w http.ResponseWriter, r *http.Request) {
	if s.reportDir == "" {
		writeJSON(w, []any{})
		return
	}
	file := r.URL.Query().Get("file")
	reports, err := loadReports(s.reportDir, file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, reports)
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Send snapshot of current state immediately
	snap := s.bus.Snapshot()
	if s.deps.Cfg != nil {
		// Only report project name if polvo.yaml still exists on disk.
		if _, err := os.Stat("polvo.yaml"); err == nil {
			snap.Status.Project = s.deps.Cfg.Project.Name
		}
	}
	snap.Status.AgentRunning = s.agentRunning()
	writeSSEEvent(w, flusher, Event{
		Kind:    EventSnapshot,
		Payload: mustJSON(snap),
	})

	ch, unsub := s.bus.Subscribe()
	defer unsub()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			writeSSEEvent(w, flusher, event)
		}
	}
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile("polvo.yaml")
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, map[string]string{"yaml": ""})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"yaml": string(data)})
}

func (s *Server) handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		YAML string `json:"yaml"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := os.WriteFile("polvo.yaml", []byte(req.YAML), 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Reload config
	cfg, err := config.Load("polvo.yaml")
	if err != nil {
		http.Error(w, fmt.Sprintf("config saved but failed to reload: %v", err), http.StatusBadRequest)
		return
	}

	// Update dependencies
	s.deps.Cfg = cfg
	if s.deps.OnConfigReload != nil {
		s.deps.OnConfigReload(cfg)
	}

	// Re-read polvo.yaml metadata (color/icon) into the registry for the root project.
	s.registry.autoRegister(s.root)

	writeJSON(w, map[string]bool{"ok": true})
}

// handleProjects handles GET /api/projects and POST /api/projects.
func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		projects := s.registry.list()
		if projects == nil {
			projects = []Project{}
		}
		writeJSON(w, projects)

	case http.MethodPost:
		var req struct {
			Name string `json:"name"`
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		proj, err := s.registry.add(req.Name, req.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, proj)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleProjectByID handles DELETE /api/projects/{id}.
func (s *Server) handleProjectByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/projects/")
	if id == "" {
		http.Error(w, "project id required", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodDelete:
		if err := s.registry.remove(id); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleFSList(w http.ResponseWriter, r *http.Request) {
	rawPath := r.URL.Query().Get("path")
	if rawPath == "" {
		rawPath = "."
	}

	projectID := r.URL.Query().Get("project")
	absPath, err := s.resolvePathForProject(projectID, rawPath)
	if err != nil {
		http.Error(w, "path traversal rejected", http.StatusBadRequest)
		return
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type Entry struct {
		Name  string `json:"name"`
		IsDir bool   `json:"is_dir"`
	}
	var dirs, files []Entry
	for _, e := range entries {
		if skipNames[e.Name()] {
			continue
		}
		entry := Entry{Name: e.Name(), IsDir: e.IsDir()}
		if e.IsDir() {
			dirs = append(dirs, entry)
		} else {
			files = append(files, entry)
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	resp := append(dirs, files...)
	writeJSON(w, resp)
}

func (s *Server) handleFSRead(w http.ResponseWriter, r *http.Request) {
	rawPath := r.URL.Query().Get("path")
	if rawPath == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}

	projectID := r.URL.Query().Get("project")
	absPath, err := s.resolvePathForProject(projectID, rawPath)
	if err != nil {
		http.Error(w, "path traversal rejected", http.StatusBadRequest)
		return
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if r.URL.Query().Get("raw") == "true" {
		contentType := http.DetectContentType(data)
		w.Header().Set("Content-Type", contentType)
		w.Write(data) //nolint:errcheck
		return
	}

	writeJSON(w, map[string]string{"content": string(data)})
}

func (s *Server) handleFSWrite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	projectID := r.URL.Query().Get("project")
	absPath, err := s.resolvePathForProject(projectID, req.Path)
	if err != nil {
		http.Error(w, "path traversal rejected", http.StatusBadRequest)
		return
	}

	if err := os.WriteFile(absPath, []byte(req.Content), 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handleFSDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	projectID := r.URL.Query().Get("project")
	absPath, err := s.resolvePathForProject(projectID, req.Path)
	if err != nil {
		http.Error(w, "path traversal rejected", http.StatusBadRequest)
		return
	}
	if err := os.RemoveAll(absPath); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handleFSRename(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		OldPath string `json:"old_path"`
		NewPath string `json:"new_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	projectID := r.URL.Query().Get("project")
	oldAbs, err := s.resolvePathForProject(projectID, req.OldPath)
	if err != nil {
		http.Error(w, "path traversal rejected", http.StatusBadRequest)
		return
	}
	newAbs, err := s.resolvePathForProject(projectID, req.NewPath)
	if err != nil {
		http.Error(w, "path traversal rejected", http.StatusBadRequest)
		return
	}
	if err := os.Rename(oldAbs, newAbs); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handleFSMkdir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	projectID := r.URL.Query().Get("project")
	absPath, err := s.resolvePathForProject(projectID, req.Path)
	if err != nil {
		http.Error(w, "path traversal rejected", http.StatusBadRequest)
		return
	}
	if err := os.MkdirAll(absPath, 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handleFSReveal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	projectID := r.URL.Query().Get("project")
	absPath, err := s.resolvePathForProject(projectID, req.Path)
	if err != nil {
		http.Error(w, "path traversal rejected", http.StatusBadRequest)
		return
	}
	// Open the parent directory in the system file manager.
	dir := absPath
	if info, err := os.Stat(absPath); err == nil && !info.IsDir() {
		dir = filepath.Dir(absPath)
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", dir)
	case "windows":
		cmd = exec.Command("explorer", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	_ = cmd.Start()
	writeJSON(w, map[string]bool{"ok": true})
}

// draftPath returns the path to the draft file for the given file path.
// It creates the drafts directory if needed.
func (s *Server) draftPath(filePath string) string {
	sanitized := strings.ReplaceAll(filepath.ToSlash(filePath), "/", "_")
	return filepath.Join(s.root, ".polvo", "drafts", sanitized)
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Message      string   `json:"message"`
		ContextFiles []string `json:"context_files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if s.deps.Registry == nil {
		http.Error(w, "no provider registry configured", http.StatusServiceUnavailable)
		return
	}

	p, err := s.deps.Registry.Default()
	if err != nil {
		http.Error(w, fmt.Sprintf("no default provider: %v", err), http.StatusServiceUnavailable)
		return
	}

	// Build user message with optional file context
	userContent := req.Message
	if len(req.ContextFiles) > 0 {
		var sb strings.Builder
		sb.WriteString(req.Message)
		for _, cf := range req.ContextFiles {
			abs, err := s.resolvePath(cf)
			if err != nil {
				continue
			}
			data, err := os.ReadFile(abs)
			if err != nil {
				continue
			}
			sb.WriteString(fmt.Sprintf("\n\n--- %s ---\n%s", cf, string(data)))
		}
		userContent = sb.String()
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	chatReq := provider.ChatRequest{
		System: "You are a helpful AI assistant integrated into Polvo IDE. Answer questions about the user's code and project.",
		Messages: []provider.Message{
			{Role: "user", Content: userContent},
		},
		MaxTokens: 4096,
	}

	writeChatSSE := func(kind EventKind, payload any) {
		data, err := json.Marshal(Event{Kind: kind, Payload: mustJSON(payload)})
		if err != nil {
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	// Try streaming first
	if sp, ok := p.(provider.StreamProvider); ok {
		_, err := sp.ChatStream(r.Context(), chatReq, func(e provider.StreamEvent) {
			if e.Type == "text_delta" {
				writeChatSSE(EventChatToken, map[string]string{"token": e.TextDelta})
			}
		})
		if err != nil {
			writeChatSSE(EventChatError, map[string]string{"error": err.Error()})
			return
		}
		writeChatSSE(EventChatDone, struct{}{})
		return
	}

	// Fall back to ChatProvider (non-streaming)
	if cp, ok := p.(provider.ChatProvider); ok {
		resp, err := cp.Chat(r.Context(), chatReq)
		if err != nil {
			writeChatSSE(EventChatError, map[string]string{"error": err.Error()})
			return
		}
		writeChatSSE(EventChatToken, map[string]string{"token": resp.Message.Content})
		writeChatSSE(EventChatDone, struct{}{})
		return
	}

	// Plain LLM fallback
	resp, err := p.Complete(r.Context(), provider.Request{
		System:    "You are a helpful AI assistant integrated into Polvo IDE.",
		Prompt:    userContent,
		MaxTokens: 4096,
	})
	if err != nil {
		writeChatSSE(EventChatError, map[string]string{"error": err.Error()})
		return
	}
	writeChatSSE(EventChatToken, map[string]string{"token": resp.Content})
	writeChatSSE(EventChatDone, struct{}{})
}

func (s *Server) handleDiff(w http.ResponseWriter, r *http.Request) {
	rawPath := r.URL.Query().Get("path")
	if rawPath == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}

	absPath, err := s.resolvePath(rawPath)
	if err != nil {
		http.Error(w, "path traversal rejected", http.StatusBadRequest)
		return
	}

	draftFile := s.draftPath(rawPath)
	if _, err := os.Stat(draftFile); os.IsNotExist(err) {
		writeJSON(w, map[string]any{"has_draft": false})
		return
	}

	current, err := os.ReadFile(absPath)
	if err != nil {
		// File may not exist yet; treat as empty
		current = []byte{}
	}

	draft, err := os.ReadFile(draftFile)
	if err != nil {
		http.Error(w, fmt.Sprintf("reading draft: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{
		"has_draft": true,
		"current":   string(current),
		"draft":     string(draft),
		"path":      rawPath,
	})
}

func (s *Server) handleDiffAccept(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	absPath, err := s.resolvePath(req.Path)
	if err != nil {
		http.Error(w, "path traversal rejected", http.StatusBadRequest)
		return
	}

	draftFile := s.draftPath(req.Path)
	draft, err := os.ReadFile(draftFile)
	if err != nil {
		http.Error(w, fmt.Sprintf("reading draft: %v", err), http.StatusInternalServerError)
		return
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		http.Error(w, fmt.Sprintf("creating directories: %v", err), http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(absPath, draft, 0644); err != nil {
		http.Error(w, fmt.Sprintf("writing file: %v", err), http.StatusInternalServerError)
		return
	}

	if err := os.Remove(draftFile); err != nil {
		// Non-fatal; log but continue
		_ = err
	}

	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handleDiffReject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	draftFile := s.draftPath(req.Path)
	if err := os.Remove(draftFile); err != nil && !os.IsNotExist(err) {
		http.Error(w, fmt.Sprintf("removing draft: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]bool{"ok": true})
}

func writeSSEEvent(w http.ResponseWriter, f http.Flusher, e Event) {
	data, err := json.Marshal(e)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", data)
	f.Flush()
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
