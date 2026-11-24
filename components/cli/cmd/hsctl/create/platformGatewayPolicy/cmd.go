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
	name     string
	fields   platformpolicy.Fields
	bodyFile string
}

var Cmd = &cobra.Command{
	Use:     "platformGatewayPolicy [flags]",
	Aliases: []string{"platform-gateway-policy"},
	Short:   "Create a platform gateway policy",
	Long: "Create a platform gateway policy. A new policy is always stored in audit mode;\n" +
		"use 'hsctl update platform-gateway-policy ID --enforcement-mode enforce' to enforce it.\n" +
		"Requires " + platformpolicy.WriteRoles + ".\n\n" +
		"Examples:\n" +
		"  hsctl create platform-gateway-policy --name cpu-cap --cedar-file cpu-cap.cedar --user-message 'CPU request is capped at 4'\n" +
		"  hsctl create platform-gateway-policy --name cpu-cap --cedar-file cpu-cap.cedar --gateway-id 2abc123 --gateway-id 2def456\n" +
		"  hsctl create platform-gateway-policy --body request.json",
	Args: cobra.NoArgs,
	RunE: run,
}

func init() {
	fs := Cmd.Flags()
	fs.StringVar(&args.name, "name", "", "Policy name (stable identifier shown to users).")
	args.fields.AddFlags(fs)
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
		if args.name == "" {
			return fmt.Errorf("--name is required")
		}
		if _, ok := request["cedar"]; !ok {
			return fmt.Errorf("one of --cedar or --cedar-file is required")
		}
		request["name"] = args.name
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

	respBody, err := platformpolicy.Request(conn, http.MethodPost, urls.PlatformGatewayPoliciesPath, nil, bytes.NewReader(body),
		"create platform gateway policy", platformpolicy.WriteRoles, http.StatusCreated, http.StatusOK)
	if err != nil {
		return err
	}
	return dump.Pretty(cmd.OutOrStdout(), respBody)
}
