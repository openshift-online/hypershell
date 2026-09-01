package gatewayRelease

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
	canaryDuration  string
	canaryPercent   int
	image           string
	name            string
	rolloutStrategy string
	status          string
	bodyFile        string
}

var Cmd = &cobra.Command{
	Use:   "gatewayRelease [flags]",
	Short: "Create a gatewayRelease",
	Long: "Create a new gatewayRelease.\n\n" +
		"Examples:\n" +
		"  hsctl create gatewayRelease --canary-duration <value> --canary-percent <value> --image <value> --name <value> --rollout-strategy <value> --status <value> \n" +
		"  hsctl create gatewayRelease --body request.json",
	Args: cobra.NoArgs,
	RunE: run,
}

func init() {
	fs := Cmd.Flags()
	fs.StringVar(&args.canaryDuration, "canary-duration", "", "canary_duration value.")
	fs.IntVar(&args.canaryPercent, "canary-percent", 0, "canary_percent value.")
	fs.StringVar(&args.image, "image", "", "image value.")
	fs.StringVar(&args.name, "name", "", "name value.")
	fs.StringVar(&args.rolloutStrategy, "rollout-strategy", "", "rollout_strategy value.")
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
		if args.canaryDuration != "" {
			request["canary_duration"] = args.canaryDuration
		}
		if args.canaryPercent != 0 {
			request["canary_percent"] = args.canaryPercent
		}
		if args.image != "" {
			request["image"] = args.image
		}
		if args.name != "" {
			request["name"] = args.name
		}
		if args.rolloutStrategy != "" {
			request["rollout_strategy"] = args.rolloutStrategy
		}
		if args.status != "" {
			request["status"] = args.status
		}
		body, err = json.Marshal(request)
		if err != nil {
			return fmt.Errorf("can't marshal request: %v", err)
		}
	}

	resp, err := conn.Post(urls.GatewayReleasesPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("can't create gatewayRelease: %v", err)
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
