package role

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
	Use:   "role ID [flags]",
	Short: "Delete a role",
	Long: "Delete a role by ID.\n\n" +
		"Examples:\n" +
		"  hsctl delete role 2abc123\n" +
		"  hsctl delete role 2abc123 --yes",
	Args: cobra.ExactArgs(1),
	RunE: run,
}

func init() {
	fs := Cmd.Flags()
	fs.BoolVar(&args.yes, "yes", false, "Skip confirmation prompt.")
}

func run(cmd *cobra.Command, argv []string) error {
	roleID := argv[0]

	if !args.yes {
		fmt.Fprintf(os.Stderr, "Delete role %s? [y/N]: ", roleID)
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

	path := urls.RolesPath + "/" + roleID
	resp, err := conn.Delete(path)
	if err != nil {
		return fmt.Errorf("can't delete role: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 && resp.StatusCode != 200 {
		return fmt.Errorf("API returned %d", resp.StatusCode)
	}

	fmt.Fprintf(os.Stdout, "Role %s deleted.\n", roleID)
	return nil
}
