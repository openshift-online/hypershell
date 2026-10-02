// Package version carries build stamps injected via -ldflags at build time
// (see the component Dockerfile). Values are intentionally generic - a commit
// SHA and timestamp - and reveal nothing about fleet structure (§3.5).
package version

// Set via -X at build time; default to "unknown" for local/dev builds.
var (
	Version   = "unknown"
	BuildTime = "unknown"
)
