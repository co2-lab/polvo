package policy

import (
	"encoding/json"
	"os"
)

// load reads policies from disk into ps.policies (called at startup).
// Session-only policies are never persisted, so all loaded policies are
// Permanent or Timed.
func (ps *PolicyStore) load() error {
	data, err := os.ReadFile(ps.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var policies []Policy
	if err := json.Unmarshal(data, &policies); err != nil {
		return err
	}
	ps.policies = policies
	return nil
}

// saveLocked writes all non-session policies to disk.
// Caller must hold ps.mu (write lock).
func (ps *PolicyStore) saveLocked() error {
	var persistent []Policy
	for _, p := range ps.policies {
		if p.TTL != TTLSession {
			persistent = append(persistent, p)
		}
	}
	data, err := json.MarshalIndent(persistent, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ps.filePath, data, 0o600)
}
