// Package gatewayconnect renders the openshell commands that connect to a
// Gateway. It is shared by `hsctl get gateway --show-connection` and `hsctl ui`.
package gatewayconnect

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
)

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

// Keep in sync with:
//
//	packages/gateway-management-ui/src/gateways/gateway-connections.ts (sandboxResourceDefaults)
//	specs/web-console/architecture.spec.md § Create a sandbox
const sandboxDriverConfig = `{"kubernetes":{"containers":{"agent":{"resources":{"requests":{"cpu":"100m","memory":"512Mi"},"limits":{"cpu":"500m","memory":"512Mi"}}}}}}`

// WriteInstructions writes the openshell commands that connect to the gateway
// described by body, a Gateway resource as returned by the API server.
func WriteInstructions(w io.Writer, body []byte) error {
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
		"DRIVER_CONFIG='" + sandboxDriverConfig + "'",
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
