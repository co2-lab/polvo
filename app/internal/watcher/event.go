package watcher

// Op represents the type of file operation.
type Op string

const (
	OpCreate Op = "create"
	OpModify Op = "modify"
	OpDelete Op = "delete"
)

// WatchEvent is emitted by a named watcher when a file changes.
type WatchEvent struct {
	WatcherName string
	Path        string
	Op          Op
}
