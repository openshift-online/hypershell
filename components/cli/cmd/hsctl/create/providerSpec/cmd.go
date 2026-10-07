package providerSpec

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
	name       string
	category   string
	capability string
	profile    string
	bodyFile   string
}

var Cmd = &cobra.Command{
	Use:   "providerSpec [flags]",
	Short: "Create a provider spec",
	Long: "Create a new provider spec.\n\n" +
		"Examples:\n" +
		"  hsctl create providerSpec --name my-spec --category inference\n" +
		"  hsctl create providerSpec --body request.json",
	Args: cobra.NoArgs,
	RunE: run,
}

func init() {
	fs := Cmd.Flags()
	fs.StringVar(&args.name, "name", "", "Provider spec name.")
	fs.StringVar(&args.category, "category", "", "Provider category (inference, source_control, knowledge).")
	fs.StringVar(&args.capability, "capability", "", "Capability definition as a JSON string.")
	fs.StringVar(&args.profile, "profile", "", "Profile configuration as a JSON string.")
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
		if args.category != "" {
			request["category"] = args.category
		}
		if args.capability != "" {
			request["capability"] = json.RawMessage(args.capability)
		}
		if args.profile != "" {
			request["profile"] = json.RawMessage(args.profile)
		}
		body, err = json.Marshal(request)
		if err != nil {
			return fmt.Errorf("can't marshal request: %w", err)
		}
	}

	resp, err := conn.Post(urls.ProviderSpecsPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("can't create provider spec: %w", err)
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
