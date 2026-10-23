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
	name     string
	bodyFile string
}

var Cmd = &cobra.Command{
	Use:     "gateway ID [flags]",
	Aliases: []string{"gateways"},
	Short:   "Update a gateway by ID",
	Long: "Update a gateway by ID using PATCH. Only the flags that are set are sent.\n\n" +
		"Examples:\n" +
		"  hsctl update gateway ID --name <value>\n" +
		"  hsctl update gateway ID --body request.json",
	Args: cobra.ExactArgs(1),
	RunE: run,
}

func init() {
	fs := Cmd.Flags()
	fs.StringVar(&args.name, "name", "", "name value.")
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
		if cmd.Flags().Changed("name") {
			request["name"] = args.name
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

	resp, err := conn.Patch(urls.GatewayPath(id), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("can't update gateway: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("can't read response: %v", err)
	}

	switch resp.StatusCode {
	case 200, 202:
	case 204:
		fmt.Fprintf(os.Stdout, "Gateway %s updated.\n", id)
		return nil
	default:
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBody))
	}

	return dump.Pretty(os.Stdout, respBody)
}
