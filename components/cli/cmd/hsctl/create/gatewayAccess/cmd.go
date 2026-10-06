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
	user      string
	role      string
	output    string
}

var Cmd = &cobra.Command{
	Use:     "gatewayAccess [flags]",
	Aliases: []string{"gateway-access"},
	Short:   "Grant a user access to a gateway",
	Long:    "Grant a user owner, admin, or user access to a gateway. The user must exist in the identity directory.",
	Args:    cobra.NoArgs,
	RunE:    run,
}

func init() {
	flags := Cmd.Flags()
	flags.StringVar(&args.gatewayID, "gateway-id", "", "Gateway ID.")
	flags.StringVar(&args.user, "user", "", "Target username from the identity directory.")
	flags.StringVar(&args.role, "role", "user", "Access role: owner, admin, or user.")
	flags.StringVarP(&args.output, "output", "o", "json", "Structured output format (json).")
}

func run(cmd *cobra.Command, _ []string) error {
	if args.gatewayID == "" {
		return fmt.Errorf("--gateway-id is required")
	}
	if args.user == "" {
		return fmt.Errorf("--user is required")
	}
	if err := gatewayaccess.ValidateRole(args.role); err != nil {
		return err
	}
	if err := gatewayaccess.ValidateOutput(args.output); err != nil {
		return err
	}
	body, err := json.Marshal(map[string]any{"username": args.user, "role": args.role})
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
	response, _, err := gatewayaccess.Request(conn, http.MethodPost, gatewayaccess.CollectionPath(args.gatewayID), nil, bytes.NewReader(body), http.StatusCreated)
	if err != nil {
		return fmt.Errorf("can't grant gateway access: %w", err)
	}
	return gatewayaccess.WriteJSON(cmd.OutOrStdout(), response)
}
