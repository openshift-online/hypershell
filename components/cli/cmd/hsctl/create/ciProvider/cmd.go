package ciProvider

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
	forge    string
	issuer   string
	jwksURL  string
	audience string
	bodyFile string
}

var Cmd = &cobra.Command{
	Use:   "ciProvider [flags]",
	Short: "Create a CI provider",
	Long: "Create a new CI provider (OIDC issuer registration for CI federation).\n\n" +
		"Examples:\n" +
		"  hsctl create ciProvider --forge github --issuer https://token.actions.githubusercontent.com --jwks-url https://token.actions.githubusercontent.com/.well-known/jwks --audience openshell-fleet\n" +
		"  hsctl create ciProvider --body request.json",
	Args: cobra.NoArgs,
	RunE: run,
}

func init() {
	fs := Cmd.Flags()
	fs.StringVar(&args.forge, "forge", "", "Forge family (github, gitlab, forgejo).")
	fs.StringVar(&args.issuer, "issuer", "", "OIDC issuer URL.")
	fs.StringVar(&args.jwksURL, "jwks-url", "", "JWKS endpoint URL for signature validation.")
	fs.StringVar(&args.audience, "audience", "", "Platform-pinned audience for CI tokens.")
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
		if args.forge != "" {
			request["forge"] = args.forge
		}
		if args.issuer != "" {
			request["issuer"] = args.issuer
		}
		if args.jwksURL != "" {
			request["jwks_url"] = args.jwksURL
		}
		if args.audience != "" {
			request["audience"] = args.audience
		}
		body, err = json.Marshal(request)
		if err != nil {
			return fmt.Errorf("can't marshal request: %w", err)
		}
	}

	resp, err := conn.Post(urls.CIProvidersPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("can't create CI provider: %w", err)
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
