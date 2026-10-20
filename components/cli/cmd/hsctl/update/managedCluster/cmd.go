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
	kubeconfigSecret string
	name             string
	provider         string
	region           string
	status           string
	bodyFile         string
}

var Cmd = &cobra.Command{
	Use:     "managedCluster ID [flags]",
	Aliases: []string{"managedClusters"},
	Short:   "Update a managedCluster by ID",
	Long: "Update a managedCluster by ID using PATCH. Only the flags that are set are sent.\n\n" +
		"Examples:\n" +
		"  hsctl update managedCluster ID --api-server-url <value> --kubeconfig-secret <value> --name <value> --provider <value> --region <value> --status <value>\n" +
		"  hsctl update managedCluster ID --body request.json",
	Args: cobra.ExactArgs(1),
	RunE: run,
}

func init() {
	fs := Cmd.Flags()
	fs.StringVar(&args.apiServerUrl, "api-server-url", "", "api_server_url value.")
	fs.StringVar(&args.kubeconfigSecret, "kubeconfig-secret", "", "kubeconfig_secret value.")
	fs.StringVar(&args.name, "name", "", "name value.")
	fs.StringVar(&args.provider, "provider", "", "provider value.")
	fs.StringVar(&args.region, "region", "", "region value.")
	fs.StringVar(&args.status, "status", "", "status value.")
	fs.StringVar(&args.bodyFile, "body", "", "File containing the request body as JSON.")
}

func run(cmd *cobra.Command, argv []string) error {
	id := argv[0]

	var body []byte
	var err error
	if args.bodyFile != "" {
		body, err = os.ReadFile(args.bodyFile)
		if err != nil {
			return fmt.Errorf("can't read body file: %v", err)
		}
	} else {
		request := map[string]interface{}{}
		if cmd.Flags().Changed("api-server-url") {
			request["api_server_url"] = args.apiServerUrl
		}
		if cmd.Flags().Changed("kubeconfig-secret") {
			request["kubeconfig_secret"] = args.kubeconfigSecret
		}
		if cmd.Flags().Changed("name") {
			request["name"] = args.name
		}
		if cmd.Flags().Changed("provider") {
			request["provider"] = args.provider
		}
		if cmd.Flags().Changed("region") {
			request["region"] = args.region
		}
		if cmd.Flags().Changed("status") {
			request["status"] = args.status
		}
		if len(request) == 0 {
			return fmt.Errorf("nothing to update: set at least one field flag or pass --body")
		}
		body, err = json.Marshal(request)
		if err != nil {
			return fmt.Errorf("can't marshal request: %v", err)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	conn, err := connection.NewConnectionBuilder().Config(cfg).Build()
	if err != nil {
		return err
	}
	defer conn.Close()

	resp, err := conn.Patch(urls.ManagedClusterPath(id), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("can't update managedCluster: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("can't read response: %v", err)
	}

	switch resp.StatusCode {
	case 200, 202:
	case 204:
		fmt.Fprintf(os.Stdout, "ManagedCluster %s updated.\n", id)
		return nil
	default:
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBody))
	}

	return dump.Pretty(os.Stdout, respBody)
}
