// Package assets provides access to embedded files (guides, prompts, config).
package assets

import (
	"io/fs"

	polvoEmbed "github.com/co2-lab/polvo/embed"
)

// ReadGuide reads a guide file from the embedded filesystem.
func ReadGuide(name string) ([]byte, error) {
	return fs.ReadFile(polvoEmbed.FS, "guides/"+name+".md")
}

// ReadPrompt reads a prompt template from the embedded filesystem.
func ReadPrompt(name string) ([]byte, error) {
	return fs.ReadFile(polvoEmbed.FS, "prompts/"+name+".md")
}

// ReadConfig reads the base config from the embedded filesystem.
func ReadConfig() ([]byte, error) {
	return fs.ReadFile(polvoEmbed.FS, "config.yaml")
}
