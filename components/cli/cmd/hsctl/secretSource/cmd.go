// Package secretSource holds the hand-maintained `hsctl secret-source` command
// group. Its subcommands act on a SecretSource's sub-resources (for example the
// write-only material upload) rather than the generated top-level CRUD, so they
// live here and are registered from main (CLI-EXT-05, consistent with CLI-EXT-01);
// no generated parent command file is hand-edited to add them.
package secretSource

import (
	"github.com/spf13/cobra"
)

// Cmd is the `hsctl secret-source` parent command.
var Cmd = &cobra.Command{
	Use:   "secret-source",
	Short: "Manage secret sources",
	Long:  "Commands that act on SecretSource sub-resources, such as uploading durable material.",
}

func init() {
	Cmd.AddCommand(uploadCmd)
}
