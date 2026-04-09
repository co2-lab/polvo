// Package embed provides the embedded filesystem containing guides, prompts, and base config.
package embed

import "embed"

//go:embed guides prompts config.yaml
var FS embed.FS
