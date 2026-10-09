package gatewayAccess

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
	"github.com/openshift-online/hypershell/components/cli/pkg/gatewayaccess"
)

var args struct {
	gatewayID string
	yes       bool
	output    string
}

var Cmd = &cobra.Command{
	Use:     "gatewayAccess USER_ID [flags]",
	Aliases: []string{"gateway-access"},
	Short:   "Revoke a user's access to a gateway",
	Long:    "Revoke all of a user's access to a gateway. Only owners may revoke an owner, and the last owner cannot be removed.",
	Args:    cobra.ExactArgs(1),
	RunE:    run,
}

func init() {
	flags := Cmd.Flags()
	flags.StringVar(&args.gatewayID, "gateway-id", "", "Gateway ID.")
	flags.BoolVar(&args.yes, "yes", false, "Skip confirmation.")
	flags.StringVarP(&args.output, "output", "o", "json", "Structured output format (json).")
}

func run(cmd *cobra.Command, argv []string) error {
	if args.gatewayID == "" {
		return fmt.Errorf("--gateway-id is required")
	}
	if err := gatewayaccess.ValidateOutput(args.output); err != nil {
		return err
	}
	if !args.yes {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Revoke access for user %s on gateway %s? [y/N]: ", argv[0], args.gatewayID)
		var response string
		_, _ = fmt.Fscanln(cmd.InOrStdin(), &response)
		if response != "y" && response != "Y" && response != "yes" {
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
	_, _, err = gatewayaccess.Request(conn, http.MethodDelete, gatewayaccess.ItemPath(args.gatewayID, argv[0]), nil, nil, http.StatusNoContent)
	if err != nil {
		return fmt.Errorf("can't revoke gateway access: %w", err)
	}
	return gatewayaccess.WriteJSON(cmd.OutOrStdout(), gatewayaccess.EmptyJSON("revoked", argv[0]))
}
