package role

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
	builtIn     bool
	description string
	displayName string
	name        string
	permissions string
	bodyFile    string
}

var Cmd = &cobra.Command{
	Use:   "role [flags]",
	Short: "Create a role",
	Long: "Create a new role.\n\n" +
		"Examples:\n" +
		"  hsctl create role --built-in <value> --description <value> --display-name <value> --name <value> --permissions <value> \n" +
		"  hsctl create role --body request.json",
	Args: cobra.NoArgs,
	RunE: run,
}

func init() {
	fs := Cmd.Flags()
	fs.BoolVar(&args.builtIn, "built-in", false, "built_in value.")
	fs.StringVar(&args.description, "description", "", "description value.")
	fs.StringVar(&args.displayName, "display-name", "", "display_name value.")
	fs.StringVar(&args.name, "name", "", "name value.")
	fs.StringVar(&args.permissions, "permissions", "", "permissions value.")
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
		if args.builtIn {
			request["built_in"] = args.builtIn
		}
		if args.description != "" {
			request["description"] = args.description
		}
		if args.displayName != "" {
			request["display_name"] = args.displayName
		}
		if args.name != "" {
			request["name"] = args.name
		}
		if args.permissions != "" {
			request["permissions"] = args.permissions
		}
		body, err = json.Marshal(request)
		if err != nil {
			return fmt.Errorf("can't marshal request: %v", err)
		}
	}

	resp, err := conn.Post(urls.RolesPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("can't create role: %v", err)
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
