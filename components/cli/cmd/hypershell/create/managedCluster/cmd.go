package managedCluster

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
	apiServerUrl     string
	fleetId          string
	kubeconfigSecret string
	name             string
	provider         string
	region           string
	status           string
	bodyFile         string
}

var Cmd = &cobra.Command{
	Use:   "managedCluster [flags]",
	Short: "Create a managedCluster",
	Long: "Create a new managedCluster.\n\n" +
		"Examples:\n" +
		"  hypershell create managedCluster --api-server-url <value> --fleet-id <value> --kubeconfig-secret <value> --name <value> --provider <value> --region <value> --status <value> \n" +
		"  hypershell create managedCluster --body request.json",
	Args: cobra.NoArgs,
	RunE: run,
}

func init() {
	fs := Cmd.Flags()
	fs.StringVar(&args.apiServerUrl, "api-server-url", "", "api_server_url value.")
	fs.StringVar(&args.fleetId, "fleet-id", "", "fleet_id value.")
	fs.StringVar(&args.kubeconfigSecret, "kubeconfig-secret", "", "kubeconfig_secret value.")
	fs.StringVar(&args.name, "name", "", "name value.")
	fs.StringVar(&args.provider, "provider", "", "provider value.")
	fs.StringVar(&args.region, "region", "", "region value.")
	fs.StringVar(&args.status, "status", "", "status value.")
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
		if args.apiServerUrl != "" {
			request["api_server_url"] = args.apiServerUrl
		}
		if args.fleetId != "" {
			request["fleet_id"] = args.fleetId
		}
		if args.kubeconfigSecret != "" {
			request["kubeconfig_secret"] = args.kubeconfigSecret
		}
		if args.name != "" {
			request["name"] = args.name
		}
		if args.provider != "" {
			request["provider"] = args.provider
		}
		if args.region != "" {
			request["region"] = args.region
		}
		if args.status != "" {
			request["status"] = args.status
		}
		body, err = json.Marshal(request)
		if err != nil {
			return fmt.Errorf("can't marshal request: %v", err)
		}
	}

	resp, err := conn.Post(urls.ManagedClustersPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("can't create managedCluster: %v", err)
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
