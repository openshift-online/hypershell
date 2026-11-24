package platformGatewayPolicy

import (
	"net/http"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
	"github.com/openshift-online/hypershell/components/cli/pkg/dump"
	"github.com/openshift-online/hypershell/components/cli/pkg/platformpolicy"
	"github.com/openshift-online/hypershell/components/cli/pkg/urls"
)

var Cmd = &cobra.Command{
	Use:     "platformGatewayPolicy ID",
	Aliases: []string{"platformGatewayPolicies", "platform-gateway-policy", "platform-gateway-policies"},
	Short:   "Get a platform gateway policy by ID",
	Long: "Get a platform gateway policy by ID and display the full object, including its\n" +
		"read-only status: validation, per-gateway rollout condition, and the audit\n" +
		"finding counts of the last 24 hours.\n\n" +
		"Examples:\n" +
		"  hsctl get platform-gateway-policy 2abc123",
	Args: cobra.ExactArgs(1),
	RunE: run,
}

func run(cmd *cobra.Command, argv []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	conn, err := connection.NewConnection().Config(cfg).Build()
	if err != nil {
		return err
	}
	defer conn.Close()

	body, err := platformpolicy.Request(conn, http.MethodGet, urls.PlatformGatewayPolicyPath(argv[0]), nil, nil,
		"get platform gateway policy", "", http.StatusOK)
	if err != nil {
		return err
	}
	return dump.Pretty(cmd.OutOrStdout(), body)
}
