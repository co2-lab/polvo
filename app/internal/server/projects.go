package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	goyaml "gopkg.in/yaml.v3"
)

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Project represents a registered project.
type Project struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Path  string `json:"path"`
	Color string `json:"color,omitempty"`
	Icon  string `json:"icon,omitempty"`
}

// projectRegistry holds all registered projects with thread-safe access.
type projectRegistry struct {
	mu       sync.RWMutex
	projects map[string]Project
	filePath string
}

// newProjectRegistry loads (or creates) the registry from configDir/projects.json.
func newProjectRegistry(configDir string) *projectRegistry {
	r := &projectRegistry{
		projects: make(map[string]Project),
		filePath: filepath.Join(configDir, "projects.json"),
	}
	r.load()
	return r
}

// load reads projects from the JSON file. Missing file is not an error.
func (r *projectRegistry) load() {
	data, err := os.ReadFile(r.filePath)
	if err != nil {
		// File may not exist yet; start with empty registry.
		return
	}
	var projects []Project
	if err := json.Unmarshal(data, &projects); err != nil {
		return
	}
	for _, p := range projects {
		r.projects[p.ID] = p
	}
}

// list returns all projects as a slice.
func (r *projectRegistry) list() []Project {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Project, 0, len(r.projects))
	for _, p := range r.projects {
		out = append(out, p)
	}
	return out
}

// add validates and registers a new project. Returns the created Project or an error.
func (r *projectRegistry) add(name, path string) (Project, error) {
	// Validate path exists.
	if _, err := os.Stat(path); err != nil {
		return Project{}, fmt.Errorf("path does not exist: %w", err)
	}

	// Normalise to absolute path.
	abs, err := filepath.Abs(path)
	if err != nil {
		return Project{}, fmt.Errorf("resolving absolute path: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Reject duplicate paths.
	for _, p := range r.projects {
		if p.Path == abs {
			return Project{}, fmt.Errorf("project with path %q already registered", abs)
		}
	}

	proj := Project{
		ID:   newID(),
		Name: name,
		Path: abs,
	}
	r.projects[proj.ID] = proj
	r.save()
	return proj, nil
}

// remove deletes a project by id and persists the change.
func (r *projectRegistry) remove(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.projects[id]; !ok {
		return fmt.Errorf("project %q not found", id)
	}
	delete(r.projects, id)
	r.save()
	return nil
}

// get returns a project by id.
func (r *projectRegistry) get(id string) (Project, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.projects[id]
	return p, ok
}

// save writes the current registry to disk atomically.
// Caller must hold r.mu (write or read lock is acceptable if no concurrent write).
func (r *projectRegistry) save() {
	projects := make([]Project, 0, len(r.projects))
	for _, p := range r.projects {
		projects = append(projects, p)
	}
	data, err := json.MarshalIndent(projects, "", "  ")
	if err != nil {
		return
	}
	tmp := r.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return
	}
	_ = os.Rename(tmp, r.filePath)
}

// readProjectMeta reads optional color/icon from <projectPath>/polvo.yaml.
func readProjectMeta(projectPath string) (color, icon string) {
	data, err := os.ReadFile(filepath.Join(projectPath, "polvo.yaml"))
	if err != nil {
		return
	}
	var cfg struct {
		Project struct {
			Color string `yaml:"color"`
			Icon  string `yaml:"icon"`
		} `yaml:"project"`
	}
	if err := goyaml.Unmarshal(data, &cfg); err != nil {
		return
	}
	return cfg.Project.Color, cfg.Project.Icon
}

// autoRegister adds path as a project (using dirName as the name) if no project
// with that path is already registered. It also updates color/icon if they changed.
func (r *projectRegistry) autoRegister(path string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return
	}

	color, icon := readProjectMeta(abs)

	r.mu.Lock()
	defer r.mu.Unlock()
	// Update existing entry if path matches.
	for id, p := range r.projects {
		if p.Path == abs {
			if p.Color != color || p.Icon != icon {
				p.Color = color
				p.Icon = icon
				r.projects[id] = p
				r.save()
			}
			return
		}
	}
	proj := Project{
		ID:    newID(),
		Name:  filepath.Base(abs),
		Path:  abs,
		Color: color,
		Icon:  icon,
	}
	r.projects[proj.ID] = proj
	r.save()
}
