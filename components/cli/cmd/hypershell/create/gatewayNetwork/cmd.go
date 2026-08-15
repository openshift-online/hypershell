package gatewayNetwork

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
	fleetId string
	hubGatewayId string
	name string
	status string
	topology string
	tunnelMode string
	bodyFile string
}

var Cmd = &cobra.Command{
	Use:   "gatewayNetwork [flags]",
	Short: "Create a gatewayNetwork",
	Long: "Create a new gatewayNetwork.\n\n" +
		"Examples:\n" +
		"  hypershell create gatewayNetwork --fleet-id <value> --hub-gateway-id <value> --name <value> --status <value> --topology <value> --tunnel-mode <value> \n" +
		"  hypershell create gatewayNetwork --body request.json",
	Args: cobra.NoArgs,
	RunE: run,
}

func init() {
	fs := Cmd.Flags()
	fs.StringVar(&args.fleetId, "fleet-id", "", "fleet_id value.")
	fs.StringVar(&args.hubGatewayId, "hub-gateway-id", "", "hub_gateway_id value.")
	fs.StringVar(&args.name, "name", "", "name value.")
	fs.StringVar(&args.status, "status", "", "status value.")
	fs.StringVar(&args.topology, "topology", "", "topology value.")
	fs.StringVar(&args.tunnelMode, "tunnel-mode", "", "tunnel_mode value.")
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
		if args.hubGatewayId != "" {
			request["hub_gateway_id"] = args.hubGatewayId
		}
		if args.name != "" {
			request["name"] = args.name
		}
		if args.status != "" {
			request["status"] = args.status
		}
		if args.topology != "" {
			request["topology"] = args.topology
		}
		if args.tunnelMode != "" {
			request["tunnel_mode"] = args.tunnelMode
		}
		body, err = json.Marshal(request)
		if err != nil {
			return fmt.Errorf("can't marshal request: %v", err)
		}
	}

	resp, err := conn.Post(urls.GatewayNetworksPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("can't create gatewayNetwork: %v", err)
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
