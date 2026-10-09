package gatewayAccess

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
	"github.com/openshift-online/hypershell/components/cli/pkg/gatewayaccess"
)

var args struct {
	gatewayID string
	role      string
	output    string
}

var Cmd = &cobra.Command{
	Use:     "gatewayAccess USER_ID [flags]",
	Aliases: []string{"gateway-access"},
	Short:   "Change a user's access role on a gateway",
	Long:    "Change a user's access to owner, admin, or user. Only owners may assign, change, or revoke the owner role.",
	Args:    cobra.ExactArgs(1),
	RunE:    run,
}

func init() {
	flags := Cmd.Flags()
	flags.StringVar(&args.gatewayID, "gateway-id", "", "Gateway ID.")
	flags.StringVar(&args.role, "role", "", "New access role: owner, admin, or user.")
	flags.StringVarP(&args.output, "output", "o", "json", "Structured output format (json).")
}

func run(cmd *cobra.Command, argv []string) error {
	if args.gatewayID == "" {
		return fmt.Errorf("--gateway-id is required")
	}
	if err := gatewayaccess.ValidateRole(args.role); err != nil {
		return err
	}
	if err := gatewayaccess.ValidateOutput(args.output); err != nil {
		return err
	}
	body, err := json.Marshal(map[string]any{"role": args.role})
	if err != nil {
		return fmt.Errorf("can't encode request: %w", err)
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
	response, _, err := gatewayaccess.Request(conn, http.MethodPatch, gatewayaccess.ItemPath(args.gatewayID, argv[0]), nil, bytes.NewReader(body), http.StatusOK)
	if err != nil {
		return fmt.Errorf("can't change gateway access: %w", err)
	}
	return gatewayaccess.WriteJSON(cmd.OutOrStdout(), response)
}
