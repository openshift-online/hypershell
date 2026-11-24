package platformGatewayPolicy

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
	"github.com/openshift-online/hypershell/components/cli/pkg/platformpolicy"
	"github.com/openshift-online/hypershell/components/cli/pkg/urls"
)

var args struct {
	yes bool
}

var Cmd = &cobra.Command{
	Use:     "platformGatewayPolicy ID [flags]",
	Aliases: []string{"platform-gateway-policy"},
	Short:   "Delete a platform gateway policy",
	Long: "Delete a platform gateway policy by ID. Built-in policies cannot be deleted.\n" +
		"Requires " + platformpolicy.WriteRoles + ".\n\n" +
		"Examples:\n" +
		"  hsctl delete platform-gateway-policy 2abc123\n" +
		"  hsctl delete platform-gateway-policy 2abc123 --yes",
	Args: cobra.ExactArgs(1),
	RunE: run,
}

func init() {
	Cmd.Flags().BoolVar(&args.yes, "yes", false, "Skip confirmation prompt.")
}

func run(cmd *cobra.Command, argv []string) error {
	id := argv[0]

	if !args.yes {
		fmt.Fprintf(cmd.ErrOrStderr(), "Delete platform gateway policy %s? [y/N]: ", id)
		var response string
		_, _ = fmt.Fscanln(cmd.InOrStdin(), &response)
		if response != "y" && response != "Y" && response != "yes" {
			fmt.Fprintf(cmd.ErrOrStderr(), "Cancelled.\n")
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

	if _, err := platformpolicy.Request(conn, http.MethodDelete, urls.PlatformGatewayPolicyPath(id), nil, nil,
		"delete platform gateway policy", platformpolicy.WriteRoles, http.StatusNoContent, http.StatusOK); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Platform gateway policy %s deleted.\n", id)
	return nil
}
