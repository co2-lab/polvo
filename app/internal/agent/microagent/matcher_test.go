package microagent

import (
	"testing"
)

func agentWith(name string, priority int, triggers ...Trigger) Microagent {
	return Microagent{
		Name:     name,
		Scope:    "workspace",
		Priority: priority,
		Triggers: triggers,
		Content:  "content for " + name,
	}
}

func trigger(typ TriggerType) Trigger { return Trigger{Type: typ} }

func keywordTrigger(words ...string) Trigger {
	return Trigger{Type: TriggerKeyword, Words: words}
}

func fileMatchTrigger(patterns []string, exclude []string) Trigger {
	return Trigger{Type: TriggerFileMatch, Patterns: patterns, Exclude: exclude}
}

func contentRegexTrigger(patterns ...string) Trigger {
	return Trigger{Type: TriggerContentRegex, Patterns: patterns}
}

// --- TriggerAlways ---

func TestMatch_Always(t *testing.T) {
	ma := agentWith("always-agent", 5, trigger(TriggerAlways))
	eval := EvalContext{UserMessage: "hello world"}

	results := Match([]Microagent{ma}, eval)
	if len(results) != 1 {
		t.Fatalf("expected 1 match, got %d", len(results))
	}
	if results[0].Trigger != TriggerAlways {
		t.Errorf("expected TriggerAlways, got %q", results[0].Trigger)
	}
	if results[0].Score != 1.0 {
		t.Errorf("expected score 1.0, got %f", results[0].Score)
	}
}

func TestMatch_Always_EmptyMessage(t *testing.T) {
	ma := agentWith("always-agent", 5, trigger(TriggerAlways))
	eval := EvalContext{}

	results := Match([]Microagent{ma}, eval)
	if len(results) != 1 {
		t.Fatalf("TriggerAlways must fire regardless of message, got %d results", len(results))
	}
}

// --- TriggerKeyword ---

func TestMatch_Keyword_Matches(t *testing.T) {
	ma := agentWith("redis-agent", 10, keywordTrigger("redis", "cache"))
	eval := EvalContext{UserMessage: "I need help with Redis connection pooling"}

	results := Match([]Microagent{ma}, eval)
	if len(results) != 1 {
		t.Fatalf("expected 1 match, got %d", len(results))
	}
	if results[0].Trigger != TriggerKeyword {
		t.Errorf("expected TriggerKeyword, got %q", results[0].Trigger)
	}
}

func TestMatch_Keyword_CaseInsensitive(t *testing.T) {
	ma := agentWith("redis-agent", 10, keywordTrigger("REDIS"))
	eval := EvalContext{UserMessage: "use redis please"}

	results := Match([]Microagent{ma}, eval)
	if len(results) != 1 {
		t.Fatalf("expected case-insensitive match, got %d results", len(results))
	}
}

func TestMatch_Keyword_NoMatch(t *testing.T) {
	ma := agentWith("redis-agent", 10, keywordTrigger("redis", "cache"))
	eval := EvalContext{UserMessage: "fix the login form"}

	results := Match([]Microagent{ma}, eval)
	if len(results) != 0 {
		t.Fatalf("expected no match, got %d", len(results))
	}
}

func TestMatch_Keyword_SecondWordMatches(t *testing.T) {
	ma := agentWith("redis-agent", 10, keywordTrigger("redis", "cache"))
	eval := EvalContext{UserMessage: "clear the cache"}

	results := Match([]Microagent{ma}, eval)
	if len(results) != 1 {
		t.Fatalf("expected match on second keyword, got %d", len(results))
	}
}

// --- TriggerFileMatch ---

func TestMatch_FileMatch_Matches(t *testing.T) {
	ma := agentWith("redis-agent", 10, fileMatchTrigger([]string{"**/redis/**"}, nil))
	eval := EvalContext{
		UserMessage:  "fix the bug",
		SessionFiles: []string{"pkg/redis/client.go"},
	}

	results := Match([]Microagent{ma}, eval)
	if len(results) != 1 {
		t.Fatalf("expected 1 match, got %d", len(results))
	}
	if results[0].Trigger != TriggerFileMatch {
		t.Errorf("expected TriggerFileMatch, got %q", results[0].Trigger)
	}
}

func TestMatch_FileMatch_NoMatch(t *testing.T) {
	ma := agentWith("redis-agent", 10, fileMatchTrigger([]string{"**/redis/**"}, nil))
	eval := EvalContext{
		UserMessage:  "fix the bug",
		SessionFiles: []string{"pkg/auth/handler.go"},
	}

	results := Match([]Microagent{ma}, eval)
	if len(results) != 0 {
		t.Fatalf("expected no match, got %d", len(results))
	}
}

func TestMatch_FileMatch_ExcludeNegatesMatch(t *testing.T) {
	ma := agentWith("redis-agent", 10, fileMatchTrigger(
		[]string{"**/redis/**"},
		[]string{"**/*_test.go"},
	))
	eval := EvalContext{
		UserMessage:  "fix",
		SessionFiles: []string{"pkg/redis/client_test.go"},
	}

	results := Match([]Microagent{ma}, eval)
	if len(results) != 0 {
		t.Fatalf("excluded file should not match, got %d results", len(results))
	}
}

func TestMatch_FileMatch_NonExcludedStillMatches(t *testing.T) {
	ma := agentWith("redis-agent", 10, fileMatchTrigger(
		[]string{"**/redis/**"},
		[]string{"**/*_test.go"},
	))
	eval := EvalContext{
		UserMessage:  "fix",
		SessionFiles: []string{"pkg/redis/client_test.go", "pkg/redis/client.go"},
	}

	results := Match([]Microagent{ma}, eval)
	if len(results) != 1 {
		t.Fatalf("non-excluded file should match, got %d results", len(results))
	}
}

// --- TriggerManual ---

func TestMatch_Manual_NeverAutoTriggered(t *testing.T) {
	ma := agentWith("manual-agent", 10, trigger(TriggerManual))
	eval := EvalContext{
		UserMessage:  "manual-agent please help",
		SessionFiles: []string{"anything.go"},
	}

	results := Match([]Microagent{ma}, eval)
	if len(results) != 0 {
		t.Fatalf("TriggerManual should never auto-trigger, got %d results", len(results))
	}
}

// --- TriggerContentRegex ---

func TestMatch_ContentRegex_Matches(t *testing.T) {
	ma := agentWith("redis-agent", 10, contentRegexTrigger(`redis\.NewClient`))
	eval := EvalContext{
		UserMessage: "help",
		FileContents: map[string]string{
			"main.go": `client := redis.NewClient(&redis.Options{})`,
		},
	}

	results := Match([]Microagent{ma}, eval)
	if len(results) != 1 {
		t.Fatalf("expected 1 match for content regex, got %d", len(results))
	}
	if results[0].Trigger != TriggerContentRegex {
		t.Errorf("expected TriggerContentRegex, got %q", results[0].Trigger)
	}
}

func TestMatch_ContentRegex_NoMatch(t *testing.T) {
	ma := agentWith("redis-agent", 10, contentRegexTrigger(`redis\.NewClient`))
	eval := EvalContext{
		UserMessage: "help",
		FileContents: map[string]string{
			"main.go": `db := postgres.Connect()`,
		},
	}

	results := Match([]Microagent{ma}, eval)
	if len(results) != 0 {
		t.Fatalf("expected no match, got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// New gap-coverage tests
// ---------------------------------------------------------------------------

// TestMatcher_KeywordNoPartialWordMatch documents the current substring-match
// behavior: keyword "red" DOES match text containing "redis" because the
// implementation uses strings.Contains (case-insensitive substring, not word
// boundary).
func TestMatcher_KeywordNoPartialWordMatch(t *testing.T) {
	ma := agentWith("color-agent", 10, keywordTrigger("red"))
	eval := EvalContext{UserMessage: "connect to redis"}

	results := Match([]Microagent{ma}, eval)
	// Document current behavior: substring match — "red" is contained in "redis".
	if len(results) != 1 {
		t.Errorf("expected keyword 'red' to substring-match 'redis' (got %d results); "+
			"if word-boundary matching is added, update this test", len(results))
	}
}

// TestMatcher_MultipleTriggersORSemantic verifies that triggers are OR-evaluated:
// a microagent with both TriggerKeyword and TriggerFileMatch fires when only
// the keyword trigger matches (no matching session files provided).
func TestMatcher_MultipleTriggersORSemantic(t *testing.T) {
	ma := agentWith("multi-trigger-agent", 10,
		keywordTrigger("deploy"),
		fileMatchTrigger([]string{"**/infra/**"}, nil),
	)
	// Only the keyword trigger fires — no session files provided.
	eval := EvalContext{UserMessage: "please deploy the service"}

	results := Match([]Microagent{ma}, eval)
	if len(results) != 1 {
		t.Fatalf("expected 1 match (keyword trigger fires; OR semantics), got %d", len(results))
	}
	if results[0].Trigger != TriggerKeyword {
		t.Errorf("expected TriggerKeyword, got %q", results[0].Trigger)
	}
}

// TestMatcher_ContentRegexAllFiles verifies that a content_regex trigger fires
// when the pattern matches ANY file in FileContents (first match sufficient).
func TestMatcher_ContentRegexAllFiles(t *testing.T) {
	ma := agentWith("regex-agent", 10, contentRegexTrigger(`redis\.NewClient`))
	eval := EvalContext{
		UserMessage: "help",
		FileContents: map[string]string{
			"auth.go":    `func Login() {}`,
			"cache.go":   `client := redis.NewClient(&redis.Options{})`,
			"handler.go": `func Handle() {}`,
		},
	}

	results := Match([]Microagent{ma}, eval)
	if len(results) != 1 {
		t.Fatalf("expected 1 match (regex matches second file), got %d", len(results))
	}
	if results[0].Trigger != TriggerContentRegex {
		t.Errorf("expected TriggerContentRegex, got %q", results[0].Trigger)
	}
}

// --- Priority ordering ---

func TestMatch_SortedByPriorityDesc(t *testing.T) {
	agents := []Microagent{
		agentWith("low", 1, trigger(TriggerAlways)),
		agentWith("high", 100, trigger(TriggerAlways)),
		agentWith("mid", 50, trigger(TriggerAlways)),
	}
	eval := EvalContext{UserMessage: "anything"}

	results := Match(agents, eval)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Microagent.Name != "high" {
		t.Errorf("first result should be highest priority, got %q", results[0].Microagent.Name)
	}
	if results[1].Microagent.Name != "mid" {
		t.Errorf("second result should be mid priority, got %q", results[1].Microagent.Name)
	}
	if results[2].Microagent.Name != "low" {
		t.Errorf("third result should be low priority, got %q", results[2].Microagent.Name)
	}
}
