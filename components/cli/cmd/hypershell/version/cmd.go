package version

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/pkg/info"
)

var Cmd = &cobra.Command{
	Use:   "version",
	Short: "Prints the version",
	Long:  "Prints the version number of the CLI.",
	Args:  cobra.NoArgs,
	RunE:  run,
}

func run(cmd *cobra.Command, argv []string) error {
	fmt.Fprintf(os.Stdout, "%s\n", info.Version)
	return nil
}
