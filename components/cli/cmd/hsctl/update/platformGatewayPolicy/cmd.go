package platformGatewayPolicy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
	"github.com/openshift-online/hypershell/components/cli/pkg/dump"
	"github.com/openshift-online/hypershell/components/cli/pkg/platformpolicy"
	"github.com/openshift-online/hypershell/components/cli/pkg/urls"
)

var args struct {
	fields          platformpolicy.Fields
	enforcementMode string
	bodyFile        string
}

var Cmd = &cobra.Command{
	Use:     "platformGatewayPolicy ID [flags]",
	Aliases: []string{"platform-gateway-policy"},
	Short:   "Update a platform gateway policy",
	Long: "Update a platform gateway policy by ID, sending only the fields that were set.\n" +
		"Setting any of --all, --gateway-id, or --exclude replaces the whole target.\n" +
		"The name cannot be changed. Requires " + platformpolicy.WriteRoles + ".\n\n" +
		"Examples:\n" +
		"  hsctl update platform-gateway-policy 2abc123 --enforcement-mode enforce\n" +
		"  hsctl update platform-gateway-policy 2abc123 --cedar-file cpu-cap.cedar --exclude 2def456\n" +
		"  hsctl update platform-gateway-policy 2abc123 --body patch.json",
	Args: cobra.ExactArgs(1),
	RunE: run,
}

func init() {
	fs := Cmd.Flags()
	args.fields.AddFlags(fs)
	fs.StringVar(&args.enforcementMode, "enforcement-mode", "", "Enforcement mode: audit or enforce.")
	fs.StringVar(&args.bodyFile, "body", "", "File containing the request body as JSON.")
}

func run(cmd *cobra.Command, argv []string) error {
	var body []byte
	if args.bodyFile != "" {
		data, err := os.ReadFile(args.bodyFile)
		if err != nil {
			return fmt.Errorf("can't read body file: %w", err)
		}
		body = data
	} else {
		request, err := args.fields.Body(cmd)
		if err != nil {
			return err
		}
		if cmd.Flags().Changed("enforcement-mode") {
			if args.enforcementMode != "audit" && args.enforcementMode != "enforce" {
				return fmt.Errorf("--enforcement-mode must be audit or enforce")
			}
			request["enforcement_mode"] = args.enforcementMode
		}
		if len(request) == 0 {
			return fmt.Errorf("nothing to update: set at least one field flag or --body")
		}
		body, err = json.Marshal(request)
		if err != nil {
			return fmt.Errorf("can't marshal request: %w", err)
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

	respBody, err := platformpolicy.Request(conn, http.MethodPatch, urls.PlatformGatewayPolicyPath(argv[0]), nil, bytes.NewReader(body),
		"update platform gateway policy", platformpolicy.WriteRoles, http.StatusOK)
	if err != nil {
		return err
	}
	return dump.Pretty(cmd.OutOrStdout(), respBody)
}
