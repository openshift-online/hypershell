package effectivePolicies

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
	Use:     "effectivePolicies GATEWAY_ID",
	Aliases: []string{"effective-policies"},
	Short:   "Get the platform policies in effect on a gateway",
	Long: "Get the effective policies of one gateway: the platform gateway policies that\n" +
		"apply to it with their effective mode and rollout condition, the global mode,\n" +
		"the fleet defaults, and recent audit findings.\n\n" +
		"Examples:\n" +
		"  hsctl get effective-policies 2abc123",
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

	body, err := platformpolicy.Request(conn, http.MethodGet, urls.GatewayEffectivePoliciesPath(argv[0]), nil, nil,
		"get effective policies", "", http.StatusOK)
	if err != nil {
		return err
	}
	return dump.Pretty(cmd.OutOrStdout(), body)
}
