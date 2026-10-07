package agentRuntime

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
	"github.com/openshift-online/hypershell/components/cli/pkg/urls"
)

var args struct {
	yes bool
}

var Cmd = &cobra.Command{
	Use:   "agentRuntime ID [flags]",
	Short: "Delete an agent runtime",
	Long: "Delete an agent runtime by ID.\n\n" +
		"Examples:\n" +
		"  hsctl delete agentRuntime 2abc123\n" +
		"  hsctl delete agentRuntime 2abc123 --yes",
	Args: cobra.ExactArgs(1),
	RunE: run,
}

func init() {
	fs := Cmd.Flags()
	fs.BoolVar(&args.yes, "yes", false, "Skip confirmation prompt.")
}

func run(cmd *cobra.Command, argv []string) error {
	id := argv[0]

	if !args.yes {
		fmt.Fprintf(os.Stderr, "Delete agent runtime %s? [y/N]: ", id)
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" && response != "yes" {
			fmt.Fprintf(os.Stderr, "Cancelled.\n")
			return nil
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	conn, err := connection.NewConnection().Config(cfg).Build()
	if err != nil {
		return err
	}
	defer conn.Close()

	resp, err := conn.Delete(urls.AgentRuntimePath(id))
	if err != nil {
		return fmt.Errorf("can't delete agent runtime: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 && resp.StatusCode != 200 {
		return fmt.Errorf("API returned %d", resp.StatusCode)
	}

	fmt.Fprintf(os.Stdout, "Agent runtime %s deleted.\n", id)
	return nil
}
