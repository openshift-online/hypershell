package serviceAccount

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
	"github.com/openshift-online/hypershell/components/cli/pkg/serviceaccount"
)

var args struct {
	gatewayID   string
	role        string
	description string
	output      string
}

var Cmd = &cobra.Command{
	Use:     "serviceAccount ID [flags]",
	Aliases: []string{"service-account"},
	Short:   "Change an OpenShell gateway service account's role",
	Long:    "Change the OpenShell role in place. The requested role is capped at the creator's current gateway role. Client credentials are unchanged; already issued access tokens keep the prior role until they expire.",
	Args:    cobra.ExactArgs(1),
	RunE:    run,
}

func init() {
	flags := Cmd.Flags()
	flags.StringVar(&args.gatewayID, "gateway-id", "", "Gateway ID.")
	flags.StringVar(&args.role, "role", "", "New OpenShell role: openshell-user or openshell-admin.")
	flags.StringVar(&args.description, "description", "", "Optional replacement description.")
	flags.StringVarP(&args.output, "output", "o", "json", "Structured output format (json).")
}

func run(cmd *cobra.Command, argv []string) error {
	if args.gatewayID == "" {
		return fmt.Errorf("--gateway-id is required")
	}
	if args.role == "" {
		return fmt.Errorf("--role is required")
	}
	if err := serviceaccount.ValidateRole(args.role); err != nil {
		return err
	}
	if err := serviceaccount.ValidateOutput(args.output); err != nil {
		return err
	}
	request := map[string]any{"role": args.role}
	if cmd.Flags().Changed("description") {
		request["description"] = args.description
	}
	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("can't encode request: %w", err)
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	conn, err := connection.NewConnectionBuilder().Config(cfg).Build()
	if err != nil {
		return err
	}
	defer conn.Close()
	response, _, err := serviceaccount.Request(conn, http.MethodPost, serviceaccount.RolePath(args.gatewayID, argv[0]), nil, bytes.NewReader(body), http.StatusOK, http.StatusAccepted)
	if err != nil {
		return fmt.Errorf("can't change service-account role: %w", err)
	}
	return serviceaccount.WriteStructured(cmd.OutOrStdout(), "", response)
}
