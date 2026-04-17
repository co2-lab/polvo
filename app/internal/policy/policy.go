// Package policy provides a session/permanent permission policy engine.
package policy

import (
	"sync"
	"time"

	"github.com/co2-lab/polvo/internal/risk"
)

// PolicyDecision is the outcome of a policy evaluation.
type PolicyDecision int

const (
	PolicyAllow PolicyDecision = iota
	PolicyDeny
)

// TTLKind controls how long a policy lives.
type TTLKind int

const (
	TTLSession   TTLKind = iota // in-memory only; lost when polvo exits
	TTLPermanent                // no expiry; written to disk
	TTLTimed                    // expires at ExpiresAt; written to disk
)

// PolicyScope defines which tool calls a policy applies to.
// Empty strings are wildcards (match any).
type PolicyScope struct {
	AgentName string
	ToolName  string
	MinRisk   risk.RiskLevel // 0 = match any risk level
}

// Policy is a single allow/deny rule.
type Policy struct {
	ID        string
	Scope     PolicyScope
	Decision  PolicyDecision
	TTL       TTLKind
	ExpiresAt time.Time
	CreatedAt time.Time
}

// PolicyStore is an in-memory policy store backed by an optional JSON file.
type PolicyStore struct {
	mu       sync.RWMutex
	policies []Policy
	filePath string
}

// NewPolicyStore creates a PolicyStore. filePath is where permanent/timed
// policies are persisted. If filePath is empty, persistence is disabled.
func NewPolicyStore(filePath string) *PolicyStore {
	ps := &PolicyStore{filePath: filePath}
	if filePath != "" {
		_ = ps.load()
	}
	return ps
}

// Upsert adds or replaces a policy. A new policy with the same Scope replaces
// the existing one regardless of Decision (each scope has exactly one policy).
// Session policies are never written to disk.
func (ps *PolicyStore) Upsert(p Policy) error {
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	if p.ID == "" {
		p.ID = generateID()
	}

	ps.mu.Lock()
	defer ps.mu.Unlock()

	// Replace existing policy with same scope.
	for i, existing := range ps.policies {
		if existing.Scope == p.Scope {
			ps.policies[i] = p
			if p.TTL != TTLSession && ps.filePath != "" {
				return ps.saveLocked()
			}
			return nil
		}
	}
	ps.policies = append(ps.policies, p)
	if p.TTL != TTLSession && ps.filePath != "" {
		return ps.saveLocked()
	}
	return nil
}

// Evaluate returns the best matching policy decision for the given context.
// Returns (decision, true) if a matching non-expired policy is found,
// (PolicyAllow, false) if no policy matches.
func (ps *PolicyStore) Evaluate(agentName, toolName string, riskLevel risk.RiskLevel) (PolicyDecision, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	now := time.Now()
	bestScore := -1
	var bestDecision PolicyDecision
	var found bool

	for _, p := range ps.policies {
		// Skip expired policies.
		if p.TTL == TTLTimed && !p.ExpiresAt.IsZero() && now.After(p.ExpiresAt) {
			continue
		}
		// Check risk level threshold.
		if p.Scope.MinRisk > 0 && riskLevel < p.Scope.MinRisk {
			continue
		}
		score := scopeScore(p.Scope, agentName, toolName)
		if score < 0 {
			continue
		}
		if score > bestScore || (score == bestScore && p.CreatedAt.After(ps.policies[0].CreatedAt)) {
			bestScore = score
			bestDecision = p.Decision
			found = true
		}
	}
	return bestDecision, found
}

// Purge removes expired policies from memory and disk.
func (ps *PolicyStore) Purge() {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	now := time.Now()
	active := ps.policies[:0]
	for _, p := range ps.policies {
		if p.TTL == TTLTimed && !p.ExpiresAt.IsZero() && now.After(p.ExpiresAt) {
			continue
		}
		active = append(active, p)
	}
	ps.policies = active
	if ps.filePath != "" {
		_ = ps.saveLocked()
	}
}

// All returns a copy of all current policies.
func (ps *PolicyStore) All() []Policy {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	out := make([]Policy, len(ps.policies))
	copy(out, ps.policies)
	return out
}

// scopeScore returns how specifically a scope matches the given agent/tool.
// Returns -1 if the scope does not match.
//
//	3 = AgentName + ToolName both match
//	2 = AgentName matches, ToolName is wildcard
//	1 = ToolName matches, AgentName is wildcard
//	0 = both wildcards (global)
func scopeScore(s PolicyScope, agentName, toolName string) int {
	agentMatch := s.AgentName == "" || s.AgentName == agentName
	toolMatch := s.ToolName == "" || s.ToolName == toolName
	if !agentMatch || !toolMatch {
		return -1
	}
	score := 0
	if s.AgentName != "" {
		score += 2
	}
	if s.ToolName != "" {
		score += 1
	}
	return score
}

func generateID() string {
	return time.Now().Format("20060102150405.999999999")
}
