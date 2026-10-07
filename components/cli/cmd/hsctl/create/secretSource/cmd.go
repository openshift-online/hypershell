package secretSource

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
	agentRuntimeId string
	purpose        string
	backend        string
	path           string
	keyMappings    string
	bodyFile       string
}

var Cmd = &cobra.Command{
	Use:   "secretSource [flags]",
	Short: "Create a secret source",
	Long: "Create a new secret source.\n\n" +
		"Examples:\n" +
		"  hsctl create secretSource --name my-secret --agent-runtime-id <id> --purpose api_key --backend vault --path secret/data/myapp\n" +
		"  hsctl create secretSource --body request.json",
	Args: cobra.NoArgs,
	RunE: run,
}

func init() {
	fs := Cmd.Flags()
	fs.StringVar(&args.name, "name", "", "Secret source name.")
	fs.StringVar(&args.agentRuntimeId, "agent-runtime-id", "", "Agent runtime ID that owns this secret source.")
	fs.StringVar(&args.purpose, "purpose", "", "Secret purpose (e.g. api_key, tls_cert).")
	fs.StringVar(&args.backend, "backend", "", "Secret backend (vault, aws_secrets_manager, kubernetes).")
	fs.StringVar(&args.path, "path", "", "Path to the secret in the backend.")
	fs.StringVar(&args.keyMappings, "key-mappings", "", "Key mappings as a JSON string.")
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
		if args.agentRuntimeId != "" {
			request["agent_runtime_id"] = args.agentRuntimeId
		}
		if args.purpose != "" {
			request["purpose"] = args.purpose
		}
		if args.backend != "" {
			request["backend"] = args.backend
		}
		if args.path != "" {
			request["path"] = args.path
		}
		if args.keyMappings != "" {
			request["key_mappings"] = json.RawMessage(args.keyMappings)
		}
		body, err = json.Marshal(request)
		if err != nil {
			return fmt.Errorf("can't marshal request: %v", err)
		}
	}

	resp, err := conn.Post(urls.SecretSourcesPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("can't create secret source: %v", err)
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
