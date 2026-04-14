package microagent

// TriggerType identifies the kind of activation criterion for a microagent.
type TriggerType string

const (
	// TriggerAlways means the microagent is always injected.
	TriggerAlways TriggerType = "always"
	// TriggerKeyword activates on case-insensitive substring match in user message.
	TriggerKeyword TriggerType = "keyword"
	// TriggerFileMatch activates when a session file matches a glob pattern.
	TriggerFileMatch TriggerType = "file_match"
	// TriggerContentRegex activates when file content matches a regex.
	TriggerContentRegex TriggerType = "content_regex"
	// TriggerAgentDecision delegates the decision to an LLM (batch, expensive).
	TriggerAgentDecision TriggerType = "agent_decision"
	// TriggerManual is only activated by explicit user reference (#name or /name).
	TriggerManual TriggerType = "manual"
)

// Trigger defines one activation criterion for a microagent.
type Trigger struct {
	Type        TriggerType `yaml:"type"`
	Words       []string    `yaml:"words,omitempty"`       // keyword: substring list
	Patterns    []string    `yaml:"patterns,omitempty"`    // file_match or content_regex
	Exclude     []string    `yaml:"exclude,omitempty"`     // file_match: negation globs
	Description string      `yaml:"description,omitempty"` // agent_decision: hint for LLM
}

// Microagent is a loaded microagent definition ready for evaluation.
type Microagent struct {
	Name     string    // unique identifier
	Scope    string    // "workspace" | "user"
	Priority int       // higher = injected first when limit exceeded
	Triggers []Trigger // OR-evaluated: any match fires the microagent
	Content  string    // Markdown body injected into the prompt
	Path     string    // source file path (for debugging and reload)
}

// EvalContext holds the inputs for trigger evaluation for a single agent turn.
type EvalContext struct {
	UserMessage  string            // current user message
	SessionFiles []string          // files read or written during this session
	FileContents map[string]string // file path → content, populated lazily for content_regex
}
