// Package web embeds the built React fleet-dashboard-ui bundle so the BFF ships
// as a single binary (data-architecture.spec §3.3: the Go BFF embeds the SPA and
// serves it with an index.html fallback). The dist/ directory is populated by the
// UI build; a placeholder index.html is checked in so the binary always builds.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the embedded UI bundle rooted at dist/.
func FS() (fs.FS, error) {
	return fs.Sub(dist, "dist")
}
