package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/co2-lab/polvo/internal/config"
)

// initRequest holds the data collected by the Init wizard.
type initRequest struct {
	ProjectName string          `json:"project_name"`
	Providers   []initProvider  `json:"providers"`
	Interfaces  []initInterface `json:"interfaces"`
	Guides      []initGuide     `json:"guides"`
}

type initProvider struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	EnvVar       string `json:"env_var"`        // e.g. ANTHROPIC_API_KEY
	APIKeyValue  string `json:"api_key_value"`  // actual secret — goes to .env only
	BaseURL      string `json:"base_url"`
	DefaultModel string `json:"default_model"`
}

// initGuide mirrors GuideConfig in config.go.
// Base is the builtin guide this instance extends (e.g. "lint" for a custom "go-lint").
// If Base is empty and Name matches a builtin, the builtin itself is used as base.
type initGuide struct {
	Name     string `json:"name"`
	Base     string `json:"base"`
	Mode     string `json:"mode"`
	File     string `json:"file"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Prompt   string `json:"prompt"`
	Role     string `json:"role"`
	UseTools bool   `json:"use_tools"`
}

type initInterface struct {
	Name     string      `json:"name"`
	Patterns []string    `json:"patterns"`
	SpecPath string      `json:"spec_path"`
	Provider string      `json:"provider"`
	Model    string      `json:"model"`
	Guides   []initGuide `json:"guides"`
}

func (s *Server) handleInit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req initRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.ProjectName == "" {
		http.Error(w, "project_name is required", http.StatusBadRequest)
		return
	}

	yaml := buildPolvoYAML(req)

	if err := os.WriteFile("polvo.yaml", []byte(yaml), 0644); err != nil {
		http.Error(w, "failed to write polvo.yaml: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := os.MkdirAll(".polvo/reports", 0755); err != nil {
		http.Error(w, "failed to create .polvo directory: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := writeEnvFile(req.Providers); err != nil {
		http.Error(w, "failed to write .env: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := ensureGitignore(); err != nil {
		http.Error(w, "failed to update .gitignore: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Reload config so the server reflects the new project immediately.
	cfg, err := config.Load("polvo.yaml")
	if err == nil {
		s.deps.Cfg = cfg
		if s.deps.OnConfigReload != nil {
			s.deps.OnConfigReload(cfg)
		}
		// Publish a snapshot event so connected clients update without reload.
		snap := s.bus.Snapshot()
		snap.Status.Project = cfg.Project.Name
		snap.Status.Version = s.deps.Version
		s.bus.Publish(Event{Kind: EventSnapshot, Payload: mustJSON(snap)})
	}

	w.WriteHeader(http.StatusNoContent)
}

// writeEnvFile creates or appends to .env with the provider secrets.
func writeEnvFile(providers []initProvider) error {
	var entries []string
	for _, p := range providers {
		if p.EnvVar != "" && p.APIKeyValue != "" {
			entries = append(entries, fmt.Sprintf("%s=%s", p.EnvVar, p.APIKeyValue))
		}
	}
	if len(entries) == 0 {
		return nil
	}

	// Append to existing .env or create it.
	f, err := os.OpenFile(".env", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	// Check if file already has content so we prepend a newline.
	info, _ := f.Stat()
	if info.Size() > 0 {
		fmt.Fprintln(f)
	}
	fmt.Fprintln(f, "# polvo provider keys")
	for _, e := range entries {
		fmt.Fprintln(f, e)
	}
	return nil
}

// ensureGitignore adds .env to .gitignore if not already present.
func ensureGitignore() error {
	const entry = ".env"
	data, err := os.ReadFile(".gitignore")
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == entry {
			return nil // already present
		}
	}

	f, err := os.OpenFile(".gitignore", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
		fmt.Fprintln(f)
	}
	fmt.Fprintln(f, entry)
	return nil
}

func buildPolvoYAML(req initRequest) string {
	var b strings.Builder

	b.WriteString("project:\n")
	b.WriteString(fmt.Sprintf("  name: %q\n", req.ProjectName))
	b.WriteString("\n")

	if len(req.Providers) > 0 {
		b.WriteString("providers:\n")
		for _, p := range req.Providers {
			b.WriteString(fmt.Sprintf("  %s:\n", p.Name))
			b.WriteString(fmt.Sprintf("    type: %s\n", p.Type))
			if p.EnvVar != "" {
				b.WriteString(fmt.Sprintf("    api_key: \"${%s}\"\n", p.EnvVar))
			}
			if p.BaseURL != "" {
				b.WriteString(fmt.Sprintf("    base_url: %q\n", p.BaseURL))
			}
			if p.DefaultModel != "" {
				b.WriteString(fmt.Sprintf("    default_model: %q\n", p.DefaultModel))
			}
		}
		b.WriteString("\n")
	}

	if len(req.Interfaces) > 0 {
		b.WriteString("interfaces:\n")
		for _, iface := range req.Interfaces {
			b.WriteString(fmt.Sprintf("  %s:\n", iface.Name))
			b.WriteString("    patterns:\n")
			for _, pat := range iface.Patterns {
				b.WriteString(fmt.Sprintf("      - %q\n", pat))
			}
			if iface.SpecPath != "" {
				b.WriteString("    derived:\n")
				b.WriteString(fmt.Sprintf("      spec: %q\n", iface.SpecPath))
			}
			if iface.Provider != "" {
				b.WriteString(fmt.Sprintf("    provider: %s\n", iface.Provider))
			}
			if iface.Model != "" {
				b.WriteString(fmt.Sprintf("    model: %q\n", iface.Model))
			}
			if len(iface.Guides) > 0 {
				b.WriteString("    guides:\n")
				for _, g := range iface.Guides {
					writeGuideYAML(&b, g, 6)
				}
			}
		}
		b.WriteString("\n")
	}

	if len(req.Guides) > 0 {
		b.WriteString("guides:\n")
		for _, g := range req.Guides {
			writeGuideYAML(&b, g, 2)
		}
	}

	return b.String()
}

func writeGuideYAML(b *strings.Builder, g initGuide, indent int) {
	pad := strings.Repeat(" ", indent)
	inner := strings.Repeat(" ", indent+2)

	// For a custom guide (name != builtin), if it has a base builtin,
	// default mode to "extend" so the resolver knows where to look.
	mode := g.Mode
	if mode == "" && g.Base != "" {
		mode = "extend"
	}

	hasFields := mode != "" || g.Base != "" || g.File != "" || g.Provider != "" ||
		g.Model != "" || g.Prompt != "" || g.Role != "" || g.UseTools

	if !hasFields {
		b.WriteString(fmt.Sprintf("%s%s: {}\n", pad, g.Name))
		return
	}

	b.WriteString(fmt.Sprintf("%s%s:\n", pad, g.Name))
	if mode != "" {
		b.WriteString(fmt.Sprintf("%smode: %s\n", inner, mode))
	}
	// base is not a real YAML field in GuideConfig — the resolver uses the name.
	// For a custom guide that extends a builtin, the file should point to the
	// project-specific content; the builtin content is pulled by the resolver
	// via the base name stored as a comment for human reference.
	if g.Base != "" && g.Name != g.Base {
		b.WriteString(fmt.Sprintf("%s# extends builtin: %s\n", inner, g.Base))
	}
	if g.File != "" {
		b.WriteString(fmt.Sprintf("%sfile: %q\n", inner, g.File))
	}
	if g.Provider != "" {
		b.WriteString(fmt.Sprintf("%sprovider: %s\n", inner, g.Provider))
	}
	if g.Model != "" {
		b.WriteString(fmt.Sprintf("%smodel: %q\n", inner, g.Model))
	}
	if g.Role != "" {
		b.WriteString(fmt.Sprintf("%srole: %s\n", inner, g.Role))
	}
	if g.UseTools {
		b.WriteString(fmt.Sprintf("%suse_tools: true\n", inner))
	}
	if g.Prompt != "" {
		b.WriteString(fmt.Sprintf("%sprompt: |\n", inner))
		for _, line := range strings.Split(g.Prompt, "\n") {
			b.WriteString(fmt.Sprintf("%s  %s\n", inner, line))
		}
	}
}
