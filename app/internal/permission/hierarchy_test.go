package permission

import "testing"

func TestHierarchy_DenyWinsAcrossLevels(t *testing.T) {
	// System allows bash; Project denies bash → overall result must be Deny.
	h := NewHierarchy(
		Rules{Allow: []string{"bash"}},     // system
		Rules{Deny: []string{"bash"}},      // project
		Rules{},                            // session
	)
	if got := h.Resolve("bash"); got != DecisionDeny {
		t.Errorf("expected Deny, got %s", got)
	}
}

func TestHierarchy_DenyAtSystemBeatsSessionAllow(t *testing.T) {
	// Deny at any level — even system — must win over session allow.
	h := NewHierarchy(
		Rules{Deny: []string{"write"}},  // system
		Rules{},                         // project
		Rules{Allow: []string{"write"}}, // session
	)
	if got := h.Resolve("write"); got != DecisionDeny {
		t.Errorf("expected Deny, got %s", got)
	}
}

func TestHierarchy_SessionOverridesProject(t *testing.T) {
	// System: Ask write, Project: Ask write, Session: Allow write → Allow.
	h := NewHierarchy(
		Rules{Ask: []string{"write"}},   // system
		Rules{Ask: []string{"write"}},   // project
		Rules{Allow: []string{"write"}}, // session
	)
	if got := h.Resolve("write"); got != DecisionAllow {
		t.Errorf("expected Allow, got %s", got)
	}
}

func TestHierarchy_ProjectOverridesSystem(t *testing.T) {
	// System: Ask edit, Project: Allow edit, Session: empty → Allow.
	h := NewHierarchy(
		Rules{Ask: []string{"edit"}},   // system
		Rules{Allow: []string{"edit"}}, // project
		Rules{},                        // session
	)
	if got := h.Resolve("edit"); got != DecisionAllow {
		t.Errorf("expected Allow, got %s", got)
	}
}

func TestHierarchy_DefaultIsAsk(t *testing.T) {
	// No rules at any level → Ask.
	h := NewHierarchy(Rules{}, Rules{}, Rules{})
	if got := h.Resolve("bash"); got != DecisionAsk {
		t.Errorf("expected Ask, got %s", got)
	}
}

func TestHierarchy_AllWildcard(t *testing.T) {
	// System Allow: ["all"] → any tool is Allowed.
	h := NewHierarchy(
		Rules{Allow: []string{"all"}}, // system
		Rules{},                       // project
		Rules{},                       // session
	)
	for _, tool := range []string{"bash", "read", "write", "edit", "glob"} {
		if got := h.Resolve(tool); got != DecisionAllow {
			t.Errorf("tool %q: expected Allow with 'all' wildcard, got %s", tool, got)
		}
	}
}

func TestHierarchy_ExplicitDenyBeatsAllWildcard(t *testing.T) {
	// System Allow: ["all"], Session Deny: ["bash"] → bash is Denied.
	h := NewHierarchy(
		Rules{Allow: []string{"all"}}, // system
		Rules{},                       // project
		Rules{Deny: []string{"bash"}}, // session
	)
	if got := h.Resolve("bash"); got != DecisionDeny {
		t.Errorf("expected Deny for bash, got %s", got)
	}
	// Other tools should still be allowed via "all".
	if got := h.Resolve("read"); got != DecisionAllow {
		t.Errorf("expected Allow for read, got %s", got)
	}
}

func TestHierarchy_SetLevel(t *testing.T) {
	// Start with no rules, then update session level at runtime.
	h := NewHierarchy(Rules{}, Rules{}, Rules{})
	if got := h.Resolve("bash"); got != DecisionAsk {
		t.Errorf("before SetLevel: expected Ask, got %s", got)
	}

	h.SetLevel(LevelSession, Rules{Allow: []string{"bash"}})
	if got := h.Resolve("bash"); got != DecisionAllow {
		t.Errorf("after SetLevel: expected Allow, got %s", got)
	}
}

func TestHierarchy_DecisionString(t *testing.T) {
	cases := []struct {
		d    Decision
		want string
	}{
		{DecisionAllow, "allow"},
		{DecisionDeny, "deny"},
		{DecisionAsk, "ask"},
	}
	for _, tc := range cases {
		if got := tc.d.String(); got != tc.want {
			t.Errorf("Decision(%d).String() = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestHierarchy_AskExplicitList(t *testing.T) {
	// System Ask: ["bash"] and no allow anywhere → Ask.
	h := NewHierarchy(
		Rules{Ask: []string{"bash"}}, // system
		Rules{},                      // project
		Rules{},                      // session
	)
	if got := h.Resolve("bash"); got != DecisionAsk {
		t.Errorf("expected Ask, got %s", got)
	}
}
