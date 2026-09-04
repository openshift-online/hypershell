package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
	"github.com/openshift-online/hypershell/components/cli/pkg/dump"
	"github.com/openshift-online/hypershell/components/cli/pkg/urls"
)

var showConnection bool

var Cmd = &cobra.Command{
	Use:     "gateway ID",
	Aliases: []string{"gateways"},
	Short:   "Get a gateway by ID",
	Long:    "Get a gateway by ID and display its details",
	Args:    cobra.ExactArgs(1),
	RunE:    run,
}

func init() {
	Cmd.Flags().BoolVar(&showConnection, "show-connection", false, "Print openshell connection instructions for the gateway")
}

func run(cmd *cobra.Command, argv []string) error {
	id := argv[0]

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	conn, err := connection.NewConnection().Config(cfg).Build()
	if err != nil {
		return err
	}
	defer conn.Close()

	resp, err := conn.Get(urls.GatewayPath(id), nil)
	if err != nil {
		return fmt.Errorf("can't retrieve gateway: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("can't read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	if showConnection {
		return printConnectionInstructions(os.Stdout, body)
	}

	return dump.Pretty(os.Stdout, body)
}

type gatewayResponse struct {
	Name         string  `json:"name"`
	Phase        string  `json:"phase"`
	ExternalDNS  string  `json:"external_dns"`
	RouteAddress string  `json:"route_address"`
	Oidc         *string `json:"oidc"`
}

type oidcConfig struct {
	Issuer   string `json:"issuer"`
	ClientID string `json:"client_id"`
	Audience string `json:"audience"`
}

const pending = "<PENDING>"

func printConnectionInstructions(w io.Writer, body []byte) error {
	var gw gatewayResponse
	if err := json.Unmarshal(body, &gw); err != nil {
		return fmt.Errorf("can't parse gateway response: %w", err)
	}

	var oidc oidcConfig
	if gw.Oidc != nil {
		_ = json.Unmarshal([]byte(*gw.Oidc), &oidc)
	}

	endpoint := resolveEndpoint(gw)
	if endpoint == "" {
		endpoint = pending
	}
	if oidc.Issuer == "" {
		oidc.Issuer = pending
	}
	if oidc.ClientID == "" {
		oidc.ClientID = pending
	}
	if oidc.Audience == "" {
		oidc.Audience = pending
	}

	fmt.Fprintln(w, buildConnectionScript(gw.Name, endpoint, oidc))
	return nil
}

func resolveEndpoint(gw gatewayResponse) string {
	if dns := strings.TrimSpace(gw.ExternalDNS); dns != "" {
		if strings.HasPrefix(dns, "https://") || strings.HasPrefix(dns, "http://") {
			return dns
		}
		return "https://" + dns
	}
	if addr := strings.TrimSpace(gw.RouteAddress); addr != "" {
		addr = strings.TrimPrefix(addr, "grpcs://")
		addr = strings.TrimPrefix(addr, "grpc://")
		return addr
	}
	return ""
}

func buildConnectionScript(name, endpoint string, oidc oidcConfig) string {
	const (
		providerName = "my-gcp"
		model        = "claude-haiku-4-5"
		sandboxName  = "mysand"
	)

	addParts := []string{
		"openshell gateway add",
		"  --name " + shellArg(name),
		"  --oidc-issuer " + shellArg(oidc.Issuer),
		"  --oidc-client-id " + shellArg(oidc.ClientID),
		"  --oidc-audience " + shellArg(oidc.Audience),
		"  " + shellArg(endpoint),
	}

	lines := []string{
		"# Install openshell: https://docs.nvidia.com/openshell/about/installation",
		"",
		"# 1. Log in to the gateway",
		strings.Join(addParts, " \\\n"),
		"",
		"# Steps 2-4 below show a GCP/Vertex AI example. Adjust provider type and config for your environment.",
		"",
		"# 2. Add the Claude on Vertex AI provider",
		"openshell provider create \\",
		"  --name " + providerName + " \\",
		"  --type google-vertex-ai \\",
		"  --from-gcloud-adc \\",
		`  --config VERTEX_AI_PROJECT_ID="$ANTHROPIC_VERTEX_PROJECT_ID" \`,
		"  --config VERTEX_AI_REGION=global",
		"",
		"# 3. Select the model",
		"openshell inference set --provider " + providerName + " --model " + model,
		"",
		"# 4. Create a sandbox",
		`DRIVER_CONFIG='{"kubernetes":{"containers":{"agent":{"resources":{"requests":{"cpu":"100m","memory":"512Mi"},"limits":{"cpu":"500m","memory":"512Mi"}}}}}}'`,
		"",
		"openshell sandbox create \\",
		"  --name " + sandboxName + " \\",
		`  --driver-config-json "$DRIVER_CONFIG" \`,
		"  --env=ANTHROPIC_BASE_URL=https://inference.local \\",
		"  --env=ANTHROPIC_API_KEY=unused \\",
		"  --no-auto-providers \\",
		"  -- claude --bare --model " + model,
	}

	return strings.Join(lines, "\n")
}

var safeShellArg = regexp.MustCompile(`^[A-Za-z0-9_./:@%+=,-]+$`)

func shellArg(value string) string {
	if value == pending || safeShellArg.MatchString(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
