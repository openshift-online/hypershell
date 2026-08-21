package serviceAccount

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
	"github.com/openshift-online/hypershell/components/cli/pkg/serviceaccount"
)

var args struct {
	gatewayID string
	yes       bool
	output    string
}

var Cmd = &cobra.Command{
	Use:     "serviceAccount ID [flags]",
	Aliases: []string{"service-account"},
	Short:   "Delete an OpenShell gateway service account",
	Long:    "Revoke the identity, remove its Keycloak client, and remove it from normal HyperShell reads.",
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
	if err := serviceaccount.ValidateOutput(args.output); err != nil {
		return err
	}
	if !args.yes {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Delete service account %s? This cannot recover its client secret. [y/N]: ", argv[0])
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
	response, status, err := serviceaccount.Request(conn, http.MethodDelete, serviceaccount.ItemPath(args.gatewayID, argv[0]), nil, nil, http.StatusNoContent, http.StatusAccepted)
	if err != nil {
		return fmt.Errorf("can't delete service account: %w", err)
	}
	if status == http.StatusNoContent {
		response = serviceaccount.EmptyJSON("deleted", argv[0])
	}
	return serviceaccount.WriteStructured(cmd.OutOrStdout(), "", response)
}
