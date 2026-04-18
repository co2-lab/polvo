package agent

import (
	"sync"
	"sync/atomic"
)

// Reducer merges a next value into existing for a typed channel.
type Reducer[T any] func(existing, next T) T

// Channel holds a typed value with an associated reducer. Thread-safe.
type Channel[T any] struct {
	mu      sync.Mutex
	value   T
	reducer Reducer[T]
	Version int64 // incremented on each Update; enables incremental execution in future
}

// NewChannel creates a Channel with the given initial value and reducer.
func NewChannel[T any](initial T, r Reducer[T]) *Channel[T] {
	return &Channel[T]{value: initial, reducer: r}
}

// Update applies the reducer and stores the result, bumping Version.
func (c *Channel[T]) Update(next T) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value = c.reducer(c.value, next)
	atomic.AddInt64(&c.Version, 1)
}

// Get returns the current value.
func (c *Channel[T]) Get() T {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.value
}

// ── Built-in reducers ────────────────────────────────────────────────────────

// StringReplaceReducer is a last-write-wins reducer for strings.
func StringReplaceReducer(_, next string) string { return next }

// StringAppendReducer appends next to existing using sep as separator.
func StringAppendReducer(sep string) Reducer[string] {
	return func(existing, next string) string {
		if existing == "" {
			return next
		}
		if next == "" {
			return existing
		}
		return existing + sep + next
	}
}

// SliceAppendReducer appends next elements to existing for any slice type.
func SliceAppendReducer[T any]() Reducer[[]T] {
	return func(existing, next []T) []T {
		return append(existing, next...)
	}
}

// ── StateGraph ────────────────────────────────────────────────────────────────

// StateGraph is the typed, mutable agent state with reducer-backed channels.
// It mirrors the flat Input struct but supports safe concurrent updates.
type StateGraph struct {
	File            *Channel[string]
	Content         *Channel[string]
	Diff            *Channel[string]
	Event           *Channel[string]
	ProjectRoot     *Channel[string]
	Interface       *Channel[string]
	Spec            *Channel[string]
	Feature         *Channel[string]
	PRDiff          *Channel[string]
	PRComments      *Channel[string]
	PreviousReports *Channel[string]
	FileHistory     *Channel[string]
	Findings        *Channel[[]Finding]
	Summary         *Channel[string]
}

// NewStateGraph creates a StateGraph with all string channels in last-write-wins
// mode and a Findings channel that appends.
func NewStateGraph() *StateGraph {
	return &StateGraph{
		File:            NewChannel("", StringReplaceReducer),
		Content:         NewChannel("", StringReplaceReducer),
		Diff:            NewChannel("", StringReplaceReducer),
		Event:           NewChannel("", StringReplaceReducer),
		ProjectRoot:     NewChannel("", StringReplaceReducer),
		Interface:       NewChannel("", StringReplaceReducer),
		Spec:            NewChannel("", StringReplaceReducer),
		Feature:         NewChannel("", StringReplaceReducer),
		PRDiff:          NewChannel("", StringReplaceReducer),
		PRComments:      NewChannel("", StringReplaceReducer),
		PreviousReports: NewChannel("", StringReplaceReducer),
		FileHistory:     NewChannel("", StringReplaceReducer),
		Findings:        NewChannel([]Finding(nil), SliceAppendReducer[Finding]()),
		Summary:         NewChannel("", StringAppendReducer("\n")),
	}
}

// FromInput initialises a StateGraph from a flat Input.
// All string channels use last-write-wins; Findings uses append.
func FromInput(in *Input) *StateGraph {
	sg := NewStateGraph()
	if in == nil {
		return sg
	}
	sg.File.Update(in.File)
	sg.Content.Update(in.Content)
	sg.Diff.Update(in.Diff)
	sg.Event.Update(in.Event)
	sg.ProjectRoot.Update(in.ProjectRoot)
	sg.Interface.Update(in.Interface)
	sg.Spec.Update(in.Spec)
	sg.Feature.Update(in.Feature)
	sg.PRDiff.Update(in.PRDiff)
	sg.PRComments.Update(in.PRComments)
	sg.PreviousReports.Update(in.PreviousReports)
	sg.FileHistory.Update(in.FileHistory)
	return sg
}

// ToInput converts the StateGraph back to the flat Input used by existing callers.
func (sg *StateGraph) ToInput() *Input {
	return &Input{
		File:            sg.File.Get(),
		Content:         sg.Content.Get(),
		Diff:            sg.Diff.Get(),
		Event:           sg.Event.Get(),
		ProjectRoot:     sg.ProjectRoot.Get(),
		Interface:       sg.Interface.Get(),
		Spec:            sg.Spec.Get(),
		Feature:         sg.Feature.Get(),
		PRDiff:          sg.PRDiff.Get(),
		PRComments:      sg.PRComments.Get(),
		PreviousReports: sg.PreviousReports.Get(),
		FileHistory:     sg.FileHistory.Get(),
	}
}
