package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// prefs holds lightweight user preferences persisted across sessions.
// Stored in ~/.polvo/prefs.json — small, human-readable, never committed.
type prefs struct {
	// LastProvider is the name (alias) of the last used provider.
	LastProvider string `json:"last_provider,omitempty"`
	// LastModel maps provider alias → last used model for that provider.
	LastModel map[string]string `json:"last_model,omitempty"`
}

func prefsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".polvo", "prefs.json")
}

func loadPrefs() prefs {
	p := prefs{LastModel: map[string]string{}}
	data, err := os.ReadFile(prefsPath())
	if err != nil {
		return p
	}
	_ = json.Unmarshal(data, &p)
	if p.LastModel == nil {
		p.LastModel = map[string]string{}
	}
	return p
}

func savePrefs(p prefs) {
	path := prefsPath()
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o600)
}
