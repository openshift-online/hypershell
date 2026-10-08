package providerBinding

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
	name           string
	workspaceId    string
	providerSpecId string
	secretSourceId string
	bodyFile       string
}

var Cmd = &cobra.Command{
	Use:   "providerBinding [flags]",
	Short: "Create a provider binding",
	Long: "Create a new provider binding.\n\n" +
		"Examples:\n" +
		"  hsctl create providerBinding --name my-binding --workspace-id <id> --provider-spec-id <id> --secret-source-id <id>\n" +
		"  hsctl create providerBinding --body request.json",
	Args: cobra.NoArgs,
	RunE: run,
}

func init() {
	fs := Cmd.Flags()
	fs.StringVar(&args.name, "name", "", "Provider binding name.")
	fs.StringVar(&args.workspaceId, "workspace-id", "", "Agent workspace ID.")
	fs.StringVar(&args.providerSpecId, "provider-spec-id", "", "Provider spec ID to bind.")
	fs.StringVar(&args.secretSourceId, "secret-source-id", "", "Secret source ID for credentials.")
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
		if args.name != "" {
			request["name"] = args.name
		}
		if args.workspaceId != "" {
			request["workspace_id"] = args.workspaceId
		}
		if args.providerSpecId != "" {
			request["provider_spec_id"] = args.providerSpecId
		}
		if args.secretSourceId != "" {
			request["secret_source_id"] = args.secretSourceId
		}
		body, err = json.Marshal(request)
		if err != nil {
			return fmt.Errorf("can't marshal request: %w", err)
		}
	}

	resp, err := conn.Post(urls.ProviderBindingsPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("can't create provider binding: %w", err)
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
