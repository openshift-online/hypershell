package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/openshift-online/hypershell/tests/e2e/driver"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// stripANSI removes ANSI color escape sequences from CLI output.
func stripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }

// containsFold reports whether sub is in s, case-insensitively.
func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

// This file wires the suite to the openshell CLI (the one product binary the suite
// invokes directly). On Kind/macOS the CLI runs through the repo's container
// wrapper (scripts/kind/openshell-container.sh) selected via OPENSHELL_BIN; the
// wrapper bind-mounts ~/.config/openshell and joins the kind network so it can
// reach the gateway. Gateway "registration" is writing the CLI's per-gateway
// metadata.json + oidc_token.json, matching the Bash suite.

// cliGatewayMetadata is the on-disk metadata.json the CLI reads per gateway.
type cliGatewayMetadata struct {
	Name            string `json:"name"`
	GatewayEndpoint string `json:"gateway_endpoint"`
	IsRemote        bool   `json:"is_remote"`
	GatewayPort     int    `json:"gateway_port"`
	AuthMode        string `json:"auth_mode"`
	OIDCIssuer      string `json:"oidc_issuer"`
	OIDCClientID    string `json:"oidc_client_id"`
	GatewayInsecure bool   `json:"gateway_insecure"`
}

// cliOIDCToken is the on-disk oidc_token.json the CLI reads per gateway.
type cliOIDCToken struct {
	AccessToken string `json:"access_token"`
	Issuer      string `json:"issuer"`
	ClientID    string `json:"client_id"`
}

// cliLocalName is the CLI's local gateway handle, "<namespace>-openshell".
func cliLocalName(namespace string) string { return namespace + "-openshell" }

// resolveOpenshellBin returns the openshell CLI command. OPENSHELL_BIN wins; on
// kind it defaults to the repo container wrapper (the native binary is not
// installed on macOS).
func (s *E2ESuite) resolveOpenshellBin() string {
	if bin := os.Getenv("OPENSHELL_BIN"); bin != "" && bin != "openshell" {
		return bin
	}
	if s.driver.Name() == "kind" {
		if root := repoRoot(); root != "" {
			wrapper := filepath.Join(root, "scripts", "kind", "openshell-container.sh")
			if _, err := os.Stat(wrapper); err == nil {
				return wrapper
			}
		}
	}
	return "openshell"
}

// repoRoot walks up from the working directory to the repo root (the dir holding
// go.mod's module github.com/openshift-online/hypershell, i.e. containing
// components/ and scripts/).
func repoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "scripts", "kind", "openshell-container.sh")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// caFile writes the cluster CA to a temp file and returns its path, so the CLI
// process (which cannot share the suite's in-process cert pool) trusts the
// issuer's TLS via SSL_CERT_FILE. Cached for the suite lifetime.
func (s *E2ESuite) caFile(t *testing.T) string {
	if s.caFilePath != "" {
		return s.caFilePath
	}
	secret, err := s.clients.Kube.CoreV1().Secrets(s.driver.PlatformNamespace()).Get(t.Context(), "hypershell-ca-secret", metav1.GetOptions{})
	s.Require().NoError(err, "get CA secret for SSL_CERT_FILE")
	ca := secret.Data["ca.crt"]
	s.Require().NotEmpty(ca, "CA secret ca.crt")
	f, err := os.CreateTemp("", "e2e-hypershell-ca-*.crt")
	s.Require().NoError(err, "create CA temp file")
	_, err = f.Write(ca)
	s.Require().NoError(err, "write CA temp file")
	s.Require().NoError(f.Close())
	s.caFilePath = f.Name()
	return s.caFilePath
}

// cliEnv returns the environment the openshell CLI process needs on kind: the
// self-signed gateway TLS bypass, the CA for issuer TLS, and HOME so the wrapper
// finds ~/.config/openshell.
func (s *E2ESuite) cliEnv(t *testing.T) []string {
	env := append(os.Environ(),
		"OPENSHELL_GATEWAY_INSECURE=true",
		"SSL_CERT_FILE="+s.caFile(t),
		"E2E_HS_NAMESPACE="+s.driver.PlatformNamespace(),
	)
	// The CLI wrapper shells out to kubectl (to find the gateway LB IP) using the
	// current kubeconfig context. When the suite is pinned to a specific context via
	// E2E_KUBECONTEXT, point the subprocess at a temp kubeconfig whose current
	// context matches, so the wrapper targets the same cluster the suite does.
	if kc := s.cliKubeconfig(t); kc != "" {
		env = append(env, "KUBECONFIG="+kc)
	}
	return env
}

// cliKubeconfig returns the path to a temp kubeconfig whose current-context is
// E2E_KUBECONTEXT, so the CLI wrapper's kubectl targets the pinned cluster. Returns
// "" when E2E_KUBECONTEXT is unset (the wrapper then uses the current context, as
// in CI). Cached for the suite lifetime.
func (s *E2ESuite) cliKubeconfig(t *testing.T) string {
	ctxName := os.Getenv("E2E_KUBECONTEXT")
	if ctxName == "" {
		return ""
	}
	if s.cliKubeconfigPath != "" {
		return s.cliKubeconfigPath
	}
	raw, err := clientcmd.NewDefaultClientConfigLoadingRules().Load()
	s.Require().NoError(err, "load kubeconfig for CLI context pin")
	raw.CurrentContext = ctxName
	f, err := os.CreateTemp("", "e2e-kubeconfig-*.yaml")
	s.Require().NoError(err, "create temp kubeconfig")
	s.Require().NoError(f.Close())
	s.Require().NoError(clientcmd.WriteToFile(*raw, f.Name()), "write temp kubeconfig")
	s.cliKubeconfigPath = f.Name()
	return s.cliKubeconfigPath
}

// registerGatewayCLI writes the CLI's per-gateway config (metadata.json +
// oidc_token.json), the Go equivalent of the Bash Area-5 registration.
func (s *E2ESuite) registerGatewayCLI(t *testing.T, gw cliGatewayMetadata, token string) string {
	local := gw.Name
	home, err := os.UserHomeDir()
	s.Require().NoError(err, "resolve home dir")
	dir := filepath.Join(home, ".config", "openshell", "gateways", local)
	s.Require().NoError(os.MkdirAll(dir, 0o700), "create gateway config dir")

	writeJSON := func(name string, v any) {
		b, err := json.MarshalIndent(v, "", "  ")
		s.Require().NoError(err, "marshal "+name)
		s.Require().NoError(os.WriteFile(filepath.Join(dir, name), b, 0o600), "write "+name)
	}
	s.runner.Show("write ~/.config/openshell/gateways/%s/{metadata.json,oidc_token.json}", local)
	writeJSON("metadata.json", gw)
	writeJSON("oidc_token.json", cliOIDCToken{AccessToken: token, Issuer: gw.OIDCIssuer, ClientID: gw.OIDCClientID})
	return dir
}

// registerGateway builds the CLI metadata for a gateway and writes its config,
// returning the CLI local handle. endpoint is the discovered gateway endpoint and
// token the per-gateway bearer token.
func (s *E2ESuite) registerGateway(t *testing.T, ref driver.GatewayRef, endpoint, token string) string {
	meta := cliGatewayMetadata{
		Name:            cliLocalName(ref.Namespace),
		GatewayEndpoint: endpoint,
		IsRemote:        true,
		GatewayPort:     0,
		AuthMode:        "oidc",
		OIDCIssuer:      envOrDefault("E2E_OIDC_ISSUER", "https://keycloak.hypershell.localhost/realms/hypershell"),
		OIDCClientID:    fmt.Sprintf("%s-%s", ref.Name, ref.ID),
		GatewayInsecure: true,
	}
	s.registerGatewayCLI(t, meta, token)
	return meta.Name
}

// cli runs `openshell -g <local> <args...>` with the CLI environment and returns
// combined output.
func (s *E2ESuite) cli(t *testing.T, local string, args ...string) (string, error) {
	full := append([]string{"-g", local}, args...)
	bin := s.resolveOpenshellBin()
	return s.runCmdEnv(t.Context(), s.cliEnv(t), bin, full...)
}

// runCmdEnv runs a command with an explicit environment through the CommandRunner,
// echoing it first and capturing combined output.
func (s *E2ESuite) runCmdEnv(ctx context.Context, env []string, name string, args ...string) (string, error) {
	return s.runner.RunWithEnv(ctx, env, name, args...)
}

// cliStatusConnected polls `openshell -g <local> status` until the output reports
// Connected or the timeout elapses.
func (s *E2ESuite) cliStatusConnected(t *testing.T, local string, timeout time.Duration) {
	err := pollNoCtx(timeout, 5*time.Second, func() bool {
		out, err := s.cli(t, local, "status")
		if err != nil {
			return false
		}
		return containsFold(stripANSI(out), "connected")
	})
	s.Require().NoErrorf(err, "openshell -g %s status never reported Connected", local)
}

// pollNoCtx is a simple time-bounded poll for CLI retries (the CLI is a child
// process; the suite context still bounds the overall go test timeout).
func pollNoCtx(timeout, interval time.Duration, fn func() bool) error {
	deadline := time.Now().Add(timeout)
	for {
		if fn() {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("condition not met within %s", timeout)
		}
		time.Sleep(interval)
	}
}
