package gateway

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
	"github.com/openshift-online/hypershell/components/cli/pkg/dump"
	"github.com/openshift-online/hypershell/components/cli/pkg/serviceaccount"
	"github.com/openshift-online/hypershell/components/cli/pkg/urls"
)

// Cmd asks the control plane to apply one gateway again with its current chart
// and default images (data-model.spec.md PLAT-DATA-33). platform:admin only.
var Cmd = &cobra.Command{
	Use:     "gateway ID",
	Aliases: []string{"gateways"},
	Short:   "Redeploy a gateway with the control plane's current chart and images",
	Long: "Ask the control plane to apply the gateway again with its current OpenShell chart and default images. " +
		"This is how a converged gateway picks up a new OpenShell release. " +
		"The gateway's generation advances; watch observed_generation and deployed_bundle with 'hsctl get gateway ID'. " +
		"Requires the platform:admin role.",
	Args: cobra.ExactArgs(1),
	RunE: run,
}

// RedeployPath is the administrator redeploy action for one gateway.
func RedeployPath(id string) string {
	return urls.GatewayPath(id) + "/redeploy"
}

func run(cmd *cobra.Command, argv []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	conn, err := connection.NewConnectionBuilder().Config(cfg).Build()
	if err != nil {
		return err
	}
	defer conn.Close()
	body, _, err := serviceaccount.Request(conn, http.MethodPost, RedeployPath(argv[0]), nil, nil, http.StatusOK)
	if err != nil {
		return fmt.Errorf("can't redeploy gateway: %w", err)
	}
	return dump.Pretty(cmd.OutOrStdout(), body)
}
