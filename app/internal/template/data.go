// Package template handles prompt template rendering.
package template

// Data holds all variables available in prompt templates.
type Data struct {
	// File path of the changed file
	File string
	// Content of the changed file
	Content string
	// Diff of the change
	Diff string
	// Guide content (resolved, merged)
	Guide string
	// Event type (created, modified, deleted, interface_changed)
	Event string
	// Project root directory
	ProjectRoot string
	// Previous reports from earlier pipeline stages
	PreviousReports string
	// File history (past reports for this file)
	FileHistory string
	// Related interface file content
	Interface string
	// Related spec content
	Spec string
	// Related feature content
	Feature string
	// PR diff (for reviewers)
	PRDiff string
	// PR comments (for retry corrections)
	PRComments string
}
