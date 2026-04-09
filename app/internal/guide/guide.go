// Package guide resolves guides by merging base (embedded) and project-level guides.
package guide

// Guide represents a resolved guide with its content and metadata.
type Guide struct {
	Name    string
	Content string
	Mode    string // "extend" or "replace"
	Role    string // "author" or "reviewer"
}

// BaseGuideNames lists all built-in guide names.
var BaseGuideNames = []string{
	"spec",
	"features",
	"tests",
	"lint",
	"best-practices",
	"review",
	"docs",
}
