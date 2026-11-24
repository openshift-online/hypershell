package platformPolicySettings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
	"github.com/openshift-online/hypershell/components/cli/pkg/dump"
	"github.com/openshift-online/hypershell/components/cli/pkg/platformpolicy"
	"github.com/openshift-online/hypershell/components/cli/pkg/urls"
)

var args struct {
	mode string
}

var Cmd = &cobra.Command{
	Use:     "platformPolicySettings --mode audit|enforce",
	Aliases: []string{"platform-policy-settings"},
	Short:   "Change the global platform policy enforcement toggle",
	Long: "Change the global minimum enforcement mode for platform gateway policies.\n" +
		"In enforce mode every policy is enforced, whatever its own enforcement mode.\n" +
		"Requires " + platformpolicy.WriteRoles + ".\n\n" +
		"Examples:\n" +
		"  hsctl update platform-policy-settings --mode enforce\n" +
		"  hsctl update platform-policy-settings --mode audit",
	Args: cobra.NoArgs,
	RunE: run,
}

func init() {
	Cmd.Flags().StringVar(&args.mode, "mode", "", "Global enforcement mode: audit or enforce.")
	_ = Cmd.MarkFlagRequired("mode")
}

func run(cmd *cobra.Command, argv []string) error {
	if args.mode != "audit" && args.mode != "enforce" {
		return fmt.Errorf("--mode must be audit or enforce")
	}
	body, err := json.Marshal(map[string]string{"mode": args.mode})
	if err != nil {
		return fmt.Errorf("can't marshal request: %w", err)
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

	respBody, err := platformpolicy.Request(conn, http.MethodPatch, urls.PlatformPolicySettingsPath, nil, bytes.NewReader(body),
		"change platform policy settings", platformpolicy.WriteRoles, http.StatusOK)
	if err != nil {
		return err
	}
	return dump.Pretty(cmd.OutOrStdout(), respBody)
}
