package inferenceRoute

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
	workspaceId       string
	providerBindingId string
	model             string
	bodyFile          string
}

var Cmd = &cobra.Command{
	Use:   "inferenceRoute [flags]",
	Short: "Create an inference route",
	Long: "Create a new inference route.\n\n" +
		"Examples:\n" +
		"  hsctl create inferenceRoute --workspace-id <id> --provider-binding-id <id> --model claude-sonnet-4-5\n" +
		"  hsctl create inferenceRoute --body request.json",
	Args: cobra.NoArgs,
	RunE: run,
}

func init() {
	fs := Cmd.Flags()
	fs.StringVar(&args.workspaceId, "workspace-id", "", "Agent workspace ID.")
	fs.StringVar(&args.providerBindingId, "provider-binding-id", "", "Provider binding ID to route inference through.")
	fs.StringVar(&args.model, "model", "", "Model alias for LLM inference.")
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
			return fmt.Errorf("can't read body file: %w", err)
		}
	} else {
		request := map[string]interface{}{}
		if args.workspaceId != "" {
			request["workspace_id"] = args.workspaceId
		}
		if args.providerBindingId != "" {
			request["provider_binding_id"] = args.providerBindingId
		}
		if args.model != "" {
			request["model"] = args.model
		}
		body, err = json.Marshal(request)
		if err != nil {
			return fmt.Errorf("can't marshal request: %w", err)
		}
	}

	resp, err := conn.Post(urls.InferenceRoutesPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("can't create inference route: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("can't read response: %w", err)
	}

	if resp.StatusCode != 201 && resp.StatusCode != 200 {
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBody))
	}

	return dump.Pretty(os.Stdout, respBody)
}
