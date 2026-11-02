package ciTrust

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
	ciProviderID string
	gatewayID    string
	role         string
	repository   string
	projectPath  string
	ref          string
	environment  string
	refProtected bool
	bodyFile     string
}

var Cmd = &cobra.Command{
	Use:   "ciTrust [flags]",
	Short: "Create a CI trust",
	Long: "Create a CI trust (enable CI federation on a gateway you own).\n\n" +
		"Examples:\n" +
		"  hsctl create ciTrust --ci-provider-id <id> --gateway-id <id> --role openshell-user --repository myorg/myrepo\n" +
		"  hsctl create ciTrust --body request.json",
	Args: cobra.NoArgs,
	RunE: run,
}

func init() {
	fs := Cmd.Flags()
	fs.StringVar(&args.ciProviderID, "ci-provider-id", "", "CI provider ID to trust.")
	fs.StringVar(&args.gatewayID, "gateway-id", "", "Gateway ID to enable CI on (must be owned by caller).")
	fs.StringVar(&args.role, "role", "", "Gateway role granted to federated CI sessions.")
	fs.StringVar(&args.repository, "repository", "", "GitHub repository (owner/name) or Forgejo repository to match.")
	fs.StringVar(&args.projectPath, "project-path", "", "GitLab project path (group/project) to match.")
	fs.StringVar(&args.ref, "ref", "", "Optional ref constraint (e.g. refs/heads/main).")
	fs.StringVar(&args.environment, "environment", "", "Optional environment constraint.")
	fs.BoolVar(&args.refProtected, "ref-protected", false, "Require protected ref (GitLab only).")
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
		if args.ciProviderID != "" {
			request["ci_provider_id"] = args.ciProviderID
		}
		if args.gatewayID != "" {
			request["gateway_id"] = args.gatewayID
		}
		if args.role != "" {
			request["role"] = args.role
		}

		subject := map[string]interface{}{}
		if args.repository != "" {
			subject["repository"] = args.repository
		}
		if args.projectPath != "" {
			subject["project_path"] = args.projectPath
		}
		if args.ref != "" {
			subject["ref"] = args.ref
		}
		if args.environment != "" {
			subject["environment"] = args.environment
		}
		if cmd.Flags().Changed("ref-protected") {
			subject["ref_protected"] = args.refProtected
		}
		if len(subject) > 0 {
			request["subject"] = subject
		}

		body, err = json.Marshal(request)
		if err != nil {
			return fmt.Errorf("can't marshal request: %w", err)
		}
	}

	resp, err := conn.Post(urls.CITrustsPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("can't create CI trust: %w", err)
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
