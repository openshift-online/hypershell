package agentRuntime

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
	name               string
	clusterId          string
	gatewayId          string
	sandboxTemplateId  string
	description        string
	cron               string
	coordinatorImage   string
	concurrencyPolicy  string
	bodyFile           string
}

var Cmd = &cobra.Command{
	Use:   "agentRuntime [flags]",
	Short: "Create an agent runtime",
	Long: "Create a new agent runtime.\n\n" +
		"Examples:\n" +
		"  hsctl create agentRuntime --name my-runtime --cluster-id <id> --gateway-id <id> --sandbox-template-id <id>\n" +
		"  hsctl create agentRuntime --body request.json",
	Args: cobra.NoArgs,
	RunE: run,
}

func init() {
	fs := Cmd.Flags()
	fs.StringVar(&args.name, "name", "", "Agent runtime name.")
	fs.StringVar(&args.clusterId, "cluster-id", "", "Cluster ID to run the agent on.")
	fs.StringVar(&args.gatewayId, "gateway-id", "", "Gateway ID the agent connects to.")
	fs.StringVar(&args.sandboxTemplateId, "sandbox-template-id", "", "Sandbox template ID for agent worker sandboxes.")
	fs.StringVar(&args.description, "description", "", "Description.")
	fs.StringVar(&args.cron, "cron", "", "Cron schedule expression.")
	fs.StringVar(&args.coordinatorImage, "coordinator-image", "", "Coordinator container image.")
	fs.StringVar(&args.concurrencyPolicy, "concurrency-policy", "", "Concurrency policy (Allow, Forbid, Replace).")
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
		if args.name != "" {
			request["name"] = args.name
		}
		if args.clusterId != "" {
			request["cluster_id"] = args.clusterId
		}
		if args.gatewayId != "" {
			request["gateway_id"] = args.gatewayId
		}
		if args.sandboxTemplateId != "" {
			request["sandbox_template_id"] = args.sandboxTemplateId
		}
		if args.description != "" {
			request["description"] = args.description
		}
		if args.cron != "" {
			request["cron"] = args.cron
		}
		if args.coordinatorImage != "" {
			request["coordinator_image"] = args.coordinatorImage
		}
		if args.concurrencyPolicy != "" {
			request["concurrency_policy"] = args.concurrencyPolicy
		}
		body, err = json.Marshal(request)
		if err != nil {
			return fmt.Errorf("can't marshal request: %v", err)
		}
	}

	resp, err := conn.Post(urls.AgentRuntimesPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("can't create agent runtime: %v", err)
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
