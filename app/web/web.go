// Package web holds the embedded static files for the polvo dashboard.
package web

import "embed"

//go:embed index.html
var FS embed.FS
