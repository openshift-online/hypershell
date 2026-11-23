package platformPolicySettings

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
	Use:     "platformPolicySettings",
	Aliases: []string{"platform-policy-settings"},
	Short:   "Get the global platform policy enforcement toggle",
	Long: "Get the platform policy settings: the global minimum enforcement mode\n" +
		"(audit or enforce), who changed it last, and the fleet sandbox defaults.\n\n" +
		"Examples:\n" +
		"  hsctl get platform-policy-settings",
	Args: cobra.NoArgs,
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

	body, err := platformpolicy.Request(conn, http.MethodGet, urls.PlatformPolicySettingsPath, nil, nil,
		"get platform policy settings", "", http.StatusOK)
	if err != nil {
		return err
	}
	return dump.Pretty(cmd.OutOrStdout(), body)
}
