package roleBinding

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
	"github.com/openshift-online/hypershell/components/cli/pkg/dump"
	"github.com/openshift-online/hypershell/components/cli/pkg/urls"
)

var args struct {
	fleetId   string
	gatewayId string
	roleId    string
	scope     string
	userId    string
	bodyFile  string
}

var Cmd = &cobra.Command{
	Use:   "roleBinding [flags]",
	Short: "Create a roleBinding",
	Long: "Create a new roleBinding.\n\n" +
		"Examples:\n" +
		"  hypershell create roleBinding --fleet-id <value> --gateway-id <value> --role-id <value> --scope <value> --user-id <value> \n" +
		"  hypershell create roleBinding --body request.json",
	Args: cobra.NoArgs,
	RunE: run,
}

func init() {
	fs := Cmd.Flags()
	fs.StringVar(&args.fleetId, "fleet-id", "", "fleet_id value.")
	fs.StringVar(&args.gatewayId, "gateway-id", "", "gateway_id value.")
	fs.StringVar(&args.roleId, "role-id", "", "role_id value.")
	fs.StringVar(&args.scope, "scope", "", "scope value.")
	fs.StringVar(&args.userId, "user-id", "", "user_id value.")
	fs.StringVar(&args.bodyFile, "body", "", "File containing the request body as JSON.")
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

	var body []byte

	if args.bodyFile != "" {
		body, err = os.ReadFile(args.bodyFile)
		if err != nil {
			return fmt.Errorf("can't read body file: %v", err)
		}
	} else {
		request := map[string]interface{}{}
		if args.fleetId != "" {
			request["fleet_id"] = args.fleetId
		}
		if args.gatewayId != "" {
			request["gateway_id"] = args.gatewayId
		}
		if args.roleId != "" {
			request["role_id"] = args.roleId
		}
		if args.scope != "" {
			request["scope"] = args.scope
		}
		if args.userId != "" {
			request["user_id"] = args.userId
		}
		body, err = json.Marshal(request)
		if err != nil {
			return fmt.Errorf("can't marshal request: %v", err)
		}
	}

	resp, err := conn.Post(urls.RoleBindingsPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("can't create roleBinding: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("can't read response: %v", err)
	}

	if resp.StatusCode != 201 && resp.StatusCode != 200 {
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBody))
	}

	return dump.Pretty(os.Stdout, respBody)
}
