package fleet

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
	description string
	name        string
	status      string
	bodyFile    string
}

var Cmd = &cobra.Command{
	Use:   "fleet [flags]",
	Short: "Create a fleet",
	Long: "Create a new fleet.\n\n" +
		"Examples:\n" +
		"  hypershell create fleet --description <value> --name <value> --status <value> \n" +
		"  hypershell create fleet --body request.json",
	Args: cobra.NoArgs,
	RunE: run,
}

func init() {
	fs := Cmd.Flags()
	fs.StringVar(&args.description, "description", "", "description value.")
	fs.StringVar(&args.name, "name", "", "name value.")
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
		if args.description != "" {
			request["description"] = args.description
		}
		if args.name != "" {
			request["name"] = args.name
		}
		if args.status != "" {
			request["status"] = args.status
		}
		body, err = json.Marshal(request)
		if err != nil {
			return fmt.Errorf("can't marshal request: %v", err)
		}
	}

	resp, err := conn.Post(urls.FleetsPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("can't create fleet: %v", err)
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
