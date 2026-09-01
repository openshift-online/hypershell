package managedDatabase

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
	connectionSecret string
	engine           string
	engineVersion    string
	instanceClass    string
	name             string
	provider         string
	region           string
	status           string
	bodyFile         string
}

var Cmd = &cobra.Command{
	Use:   "managedDatabase [flags]",
	Short: "Create a managedDatabase",
	Long: "Create a new managedDatabase.\n\n" +
		"Examples:\n" +
		"  hsctl create managedDatabase --connection-secret <value> --engine <value> --engine-version <value> --instance-class <value> --name <value> --provider <value> --region <value> --status <value> \n" +
		"  hsctl create managedDatabase --body request.json",
	Args: cobra.NoArgs,
	RunE: run,
}

func init() {
	fs := Cmd.Flags()
	fs.StringVar(&args.connectionSecret, "connection-secret", "", "connection_secret value.")
	fs.StringVar(&args.engine, "engine", "", "engine value.")
	fs.StringVar(&args.engineVersion, "engine-version", "", "engine_version value.")
	fs.StringVar(&args.instanceClass, "instance-class", "", "instance_class value.")
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
		if args.connectionSecret != "" {
			request["connection_secret"] = args.connectionSecret
		}
		if args.engine != "" {
			request["engine"] = args.engine
		}
		if args.engineVersion != "" {
			request["engine_version"] = args.engineVersion
		}
		if args.instanceClass != "" {
			request["instance_class"] = args.instanceClass
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

	resp, err := conn.Post(urls.ManagedDatabasesPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("can't create managedDatabase: %v", err)
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
