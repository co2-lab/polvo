package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/co2-lab/polvo/internal/config"
	"github.com/co2-lab/polvo/internal/provider"
)

const addProviderLabel = "  + Add new provider…"

// splashResult is what the splash screen returns to the caller.
type splashResult struct {
	ProviderName string
	Provider     provider.ChatProvider
	Model        string
}

// provEntry is a configured provider ready to display.
type provEntry struct {
	alias string
	pcfg  config.ProviderConfig
	prov  provider.LLMProvider
}

// ProviderRegistry is the subset of provider.Registry used by splash.
type ProviderRegistry interface {
	Get(string) (provider.LLMProvider, error)
	All() map[string]provider.LLMProvider
}

// showSplash renders the startup screen, lets the user pick provider and model,
// and returns the selection. Returning nil means "run setup wizard instead".
// If the user picks "Add new provider", runs the wizard then loops back.
func showSplash(version, workDir string, cfg *config.Config, registry ProviderRegistry) (*splashResult, error) {
	isTTY := term.IsTerminal(int(os.Stdin.Fd()))

	for {
		p := loadPrefs()

		var entries []provEntry
		if cfg != nil {
			for alias, pcfg := range cfg.Providers {
				prov, err := registry.Get(alias)
				if err != nil {
					continue
				}
				entries = append(entries, provEntry{alias, pcfg, prov})
			}
		}

		// Sort: last-used first, then alphabetical.
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].alias == p.LastProvider {
				return true
			}
			if entries[j].alias == p.LastProvider {
				return false
			}
			return entries[i].alias < entries[j].alias
		})

		printHeader(version, workDir, isTTY)

		// No providers → caller runs setup wizard.
		if len(entries) == 0 {
			return nil, nil
		}

		// ── Provider selection ────────────────────────────────────────────────

		provLabels := make([]string, len(entries)+1)
		for i, e := range entries {
			model := p.LastModel[e.alias]
			if model == "" {
				model = e.pcfg.DefaultModel
			}
			provLabels[i] = fmt.Sprintf("%-20s  \033[2m%s\033[0m  \033[2m%s\033[0m",
				e.alias, e.pcfg.Type, model)
		}
		provLabels[len(entries)] = addProviderLabel

		defaultProvIdx := 0
		for i, e := range entries {
			if e.alias == p.LastProvider {
				defaultProvIdx = i
				break
			}
		}

		provIdx, err := arrowSelect("Provider", provLabels, defaultProvIdx)
		if err != nil {
			return nil, err
		}
		fmt.Println()

		// "Add new provider" selected → run wizard, loop back.
		if provIdx == len(entries) {
			if _, wizErr := runSetupWizard(false); wizErr != nil {
				return nil, wizErr
			}
			// ...
			newCfg, _ := config.LoadWithUser("")
			if newCfg != nil && len(newCfg.Providers) > 0 {
				cfg = newCfg
				newRegistry, regErr := provider.NewRegistry(newCfg.Providers)
				if regErr == nil {
					registry = newRegistry
				}
			}
			// Clear again before showing splash.
			fmt.Print("\033[2J\033[3J\033[H")
			continue
		}

		chosen := entries[provIdx]

		// ── Model selection ───────────────────────────────────────────────────

		lastModel := p.LastModel[chosen.alias]
		if lastModel == "" {
			lastModel = chosen.pcfg.DefaultModel
		}

		model, err := pickModelForProvider(chosen.alias, chosen.prov, lastModel)
		if err != nil {
			return nil, err
		}
		fmt.Println()

		// ── Persist prefs ─────────────────────────────────────────────────────

		p.LastProvider = chosen.alias
		if p.LastModel == nil {
			p.LastModel = map[string]string{}
		}
		p.LastModel[chosen.alias] = model
		savePrefs(p)

		cp, ok := chosen.prov.(provider.ChatProvider)
		if !ok {
			return nil, fmt.Errorf("provider %q does not support chat", chosen.alias)
		}

		return &splashResult{
			ProviderName: chosen.alias,
			Provider:     cp,
			Model:        model,
		}, nil
	}
}

// autoSelectProvider picks the sole configured provider without showing the
// selection UI. Used after the setup wizard when only one provider exists.
func autoSelectProvider(cfg *config.Config, registry ProviderRegistry) (*splashResult, error) {
	for alias, pcfg := range cfg.Providers {
		prov, err := registry.Get(alias)
		if err != nil {
			return nil, fmt.Errorf("loading provider %q: %w", alias, err)
		}
		cp, ok := prov.(provider.ChatProvider)
		if !ok {
			return nil, fmt.Errorf("provider %q does not support chat", alias)
		}
		model := pcfg.DefaultModel
		p := loadPrefs()
		if p.LastModel[alias] != "" {
			model = p.LastModel[alias]
		}
		return &splashResult{ProviderName: alias, Provider: cp, Model: model}, nil
	}
	return nil, fmt.Errorf("no provider found")
}

// printHeader prints the half-block logo + session metadata.
func printHeader(version, workDir string, isTTY bool) {
	projectName := filepath.Base(workDir)
	homeDir, _ := os.UserHomeDir()
	displayDir := workDir
	if homeDir != "" && strings.HasPrefix(workDir, homeDir) {
		displayDir = "~" + workDir[len(homeDir):]
	}

	r := "\033[0m"
	d := "\033[2m"
	b := "\033[1m"
	if !isTTY {
		r, d, b = "", "", ""
	}

	// "polvo" wordmark reconstructed from the SVG path (title.svg).
	// viewBox 244×44, 6.25-unit grid → 7 cols × 6 pixel rows per letter.
	// Half-block rendering: rows 0+1, 2+3, 4+5 → 3 terminal lines.
	// Gradient: green (#00ffab) → cyan (#00c4ff) → purple (#7b61ff).
	g1 := "\033[38;5;48m"  // green  — P
	g2 := "\033[38;5;45m"  // cyan   — O1, L
	g3 := "\033[38;5;63m"  // purple — V, O2
	sp := "  "             // inter-letter spacing
	logo := []string{
		// pixel rows 0+1: P-top+bowl-open, O-empty, L-top, V-empty, O-empty
		g1 + `██▀▀▀█▄` + sp + g2 + `       ` + sp + g2 + ` ▀██  ` + sp + g3 + `      ` + sp + g3 + `       ` + r,
		// pixel rows 2+3: P-bowl-close, O-ring, L-stem, V-legs, O-ring
		g1 + `██▄▄▄█▀` + sp + g2 + `▄█▀▀▀█▄` + sp + g2 + `  ██  ` + sp + g3 + `██  ██` + sp + g3 + `▄█▀▀▀█▄` + r,
		// pixel rows 4+5: P-stem, O-bottom, L-base, V-converge, O-bottom
		g1 + `██     ` + sp + g2 + `▀█▄▄▄█▀` + sp + g2 + `▄▄██▄▄` + sp + g3 + ` ▀██▀ ` + sp + g3 + `▀█▄▄▄█▀` + r,
	}

	fmt.Println()
	for _, l := range logo {
		fmt.Println(l)
	}
	fmt.Println()
	fmt.Printf("  %sproject%s   %s%s%s\n", d, r, b, projectName, r)
	fmt.Printf("  %sversion%s   %s%s%s\n", d, r, b, version, r)
	fmt.Printf("  %sdirectory%s %s%s%s\n", d, r, b, displayDir, r)
	fmt.Println()
	fmt.Println(d + "  ──────────────────────────────────────────────" + r)
	fmt.Println()
}

// pickModelForProvider fetches models from API and presents arrow selector.
func pickModelForProvider(alias string, prov provider.LLMProvider, lastModel string) (string, error) {
	fmt.Printf("  \033[2mFetching models for %s…\033[0m", alias)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	var modelIDs []string
	if lister, ok := prov.(provider.ModelLister); ok {
		if models, err := lister.ListModels(ctx); err == nil {
			for _, m := range models {
				modelIDs = append(modelIDs, m.ID)
			}
		}
	}

	// Clear the fetching line.
	fmt.Printf("\r%s\r", strings.Repeat(" ", 60))

	const customOpt = "⌨  Type a custom model name…"

	if len(modelIDs) == 0 {
		fmt.Printf("  Model [\033[1m%s\033[0m]: ", lastModel)
		var val string
		fmt.Scanln(&val)
		if strings.TrimSpace(val) == "" {
			return lastModel, nil
		}
		return strings.TrimSpace(val), nil
	}

	labels := make([]string, len(modelIDs)+1)
	defaultIdx := 0
	for i, id := range modelIDs {
		if id == lastModel {
			labels[i] = id + "  \033[2m← last used\033[0m"
			defaultIdx = i
		} else {
			labels[i] = id
		}
	}
	labels[len(modelIDs)] = customOpt

	fmt.Printf("  \033[2m%d models  ·  last: %s\033[0m\n\n", len(modelIDs), lastModel)

	idx, err := arrowSelect("Model", labels, defaultIdx)
	if err != nil {
		return "", err
	}

	if idx == len(modelIDs) {
		fmt.Printf("  Model name [\033[1m%s\033[0m]: ", lastModel)
		var val string
		fmt.Scanln(&val)
		if strings.TrimSpace(val) == "" {
			return lastModel, nil
		}
		return strings.TrimSpace(val), nil
	}

	return modelIDs[idx], nil
}
