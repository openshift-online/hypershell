package sandboxTemplate

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
	image      string
	namePrefix string
	bodyFile   string
}

var Cmd = &cobra.Command{
	Use:   "sandboxTemplate [flags]",
	Short: "Create a sandbox template",
	Long: "Create a new sandbox template.\n\n" +
		"Examples:\n" +
		"  hsctl create sandboxTemplate --name my-template --image registry.example.com/sandbox:v1\n" +
		"  hsctl create sandboxTemplate --body request.json",
	Args: cobra.NoArgs,
	RunE: run,
}

func init() {
	fs := Cmd.Flags()
	fs.StringVar(&args.name, "name", "", "Sandbox template name.")
	fs.StringVar(&args.image, "image", "", "OCI image reference for the sandbox.")
	fs.StringVar(&args.namePrefix, "name-prefix", "", "Prefix for generated sandbox names.")
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
		if args.image != "" {
			request["image"] = args.image
		}
		if args.namePrefix != "" {
			request["name_prefix"] = args.namePrefix
		}
		body, err = json.Marshal(request)
		if err != nil {
			return fmt.Errorf("can't marshal request: %v", err)
		}
	}

	resp, err := conn.Post(urls.SandboxTemplatesPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("can't create sandbox template: %v", err)
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
