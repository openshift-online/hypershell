//go:build !(js && wasm)

package commands

import (
	"github.com/spf13/cobra"

	tuicmd "github.com/openshift-online/hypershell/components/cli/cmd/hsctl/tui"
)

// addNativeCommands registers the commands only the native binary has. The
// browser module is built without this file, so it links neither bubbletea nor
// atotto/clipboard (CLI-WASM-01). Every command added here must be local-only in
// the browser (LocalOnlyCommands); the drift test fails otherwise.
func addNativeCommands(root *cobra.Command) {
	root.AddCommand(tuicmd.Cmd)
}
