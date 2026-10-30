package gateway

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
	clusterId      string
	dedicatedRoute bool
	name           string
	bodyFile       string
}

var Cmd = &cobra.Command{
	Use:   "gateway [flags]",
	Short: "Create a gateway",
	Long: "Create a new gateway.\n\n" +
		"Examples:\n" +
		"  hsctl create gateway --cluster-id <value> --dedicated-route <value> --name <value> \n" +
		"  hsctl create gateway --body request.json",
	Args: cobra.NoArgs,
	RunE: run,
}

func init() {
	fs := Cmd.Flags()
	fs.StringVar(&args.clusterId, "cluster-id", "", "cluster_id value.")
	fs.BoolVar(&args.dedicatedRoute, "dedicated-route", false, "dedicated_route value.")
	fs.StringVar(&args.name, "name", "", "name value.")
	fs.StringVar(&args.bodyFile, "body", "", "File containing the request body as JSON.")
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

	var body []byte

	if args.bodyFile != "" {
		body, err = os.ReadFile(args.bodyFile)
		if err != nil {
			return fmt.Errorf("can't read body file: %v", err)
		}
	} else {
		request := map[string]interface{}{}
		if args.clusterId != "" {
			request["cluster_id"] = args.clusterId
		}
		if args.dedicatedRoute {
			request["dedicated_route"] = args.dedicatedRoute
		}
		if args.name != "" {
			request["name"] = args.name
		}
		body, err = json.Marshal(request)
		if err != nil {
			return fmt.Errorf("can't marshal request: %v", err)
		}
	}

	resp, err := conn.Post(urls.GatewaysPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("can't create gateway: %v", err)
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
