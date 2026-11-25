//go:build js && wasm

package commands

import "github.com/spf13/cobra"

// addNativeCommands is empty in the browser module: hsctl tui needs a local
// terminal, so the browser tree has a local-only stub instead (CLI-WASM-04).
func addNativeCommands(*cobra.Command) {}
