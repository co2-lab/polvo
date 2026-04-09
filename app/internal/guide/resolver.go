package guide

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/co2-lab/polvo/internal/assets"
	"github.com/co2-lab/polvo/internal/config"
)

// Resolver resolves guides by merging base and project layers.
type Resolver struct {
	projectRoot string
	guidesDir   string
	guidesCfg   map[string]config.GuideConfig
}

// NewResolver creates a new guide resolver.
func NewResolver(projectRoot string, cfg *config.Config) *Resolver {
	return &Resolver{
		projectRoot: projectRoot,
		guidesDir:   filepath.Join(projectRoot, "guides"),
		guidesCfg:   cfg.Guides,
	}
}

// Resolve returns the resolved guide for the given name and configuration.
// It applies the extend/replace logic between base and project layers.
func (r *Resolver) Resolve(name string, gcfg config.GuideConfig) (*Guide, error) {
	mode := gcfg.Mode
	if mode == "" {
		mode = "extend"
	}

	// Read base guide
	baseContent, err := assets.ReadGuide(name)
	if err != nil {
		// Not a base guide — must be a custom guide
		baseContent = nil
	}

	// Read project guide
	var projectContent []byte
	projectFile := gcfg.File
	if projectFile == "" {
		projectFile = filepath.Join(r.guidesDir, name+".md")
	}
	if data, err := os.ReadFile(projectFile); err == nil {
		projectContent = data
	}

	// Merge based on mode
	var content string
	switch {
	case projectContent != nil && mode == "replace":
		content = string(projectContent)
	case projectContent != nil && mode == "extend":
		content = string(baseContent) + "\n\n---\n\n# Project-Specific Rules\n\n" + string(projectContent)
	case baseContent != nil:
		content = string(baseContent)
	default:
		return nil, fmt.Errorf("guide %q not found in base or project", name)
	}

	role := gcfg.Role
	if role == "" {
		role = defaultRole(name)
	}

	return &Guide{
		Name:    name,
		Content: content,
		Mode:    mode,
		Role:    role,
	}, nil
}

// ResolveAll resolves all configured guides (base + custom).
func (r *Resolver) ResolveAll() ([]*Guide, error) {
	seen := make(map[string]bool)
	var guides []*Guide

	// Resolve base guides
	for _, name := range BaseGuideNames {
		gcfg := r.guidesCfg[name]
		g, err := r.Resolve(name, gcfg)
		if err != nil {
			continue // Base guide may not exist if replaced
		}
		guides = append(guides, g)
		seen[name] = true
	}

	// Resolve custom guides from config
	for name, gcfg := range r.guidesCfg {
		if seen[name] {
			continue
		}
		g, err := r.Resolve(name, gcfg)
		if err != nil {
			return nil, fmt.Errorf("resolving custom guide %q: %w", name, err)
		}
		guides = append(guides, g)
	}

	return guides, nil
}

func defaultRole(name string) string {
	switch name {
	case "lint", "best-practices", "review":
		return "reviewer"
	default:
		return "author"
	}
}
