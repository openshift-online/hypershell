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
	// ExpiresAt (Unix seconds) is set only for short-lived gateway service-account
	// tokens, which the suite renews; identity-mode files omit it.
	ExpiresAt int64 `json:"expires_at,omitempty"`
}

// cliGatewayConfigDir is the CLI's per-gateway config directory for a handle.
func cliGatewayConfigDir(local string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", "openshell", "gateways", local), nil
}

// writeCLIOIDCToken atomically replaces the CLI's oidc_token.json in dir with
// mode 0600, so a CLI process that starts while the token is renewed reads either
// the old or the new token and never a partial file.
func writeCLIOIDCToken(dir string, token cliOIDCToken) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create gateway config dir: %w", err)
	}
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal oidc_token.json: %w", err)
	}
	return writePrivateFile(dir, "oidc_token.json", data)
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

// readSandboxImage reads the same plain shell assignment that Kind sources.
// There is no fallback literal: a missing pin must fail before test provisioning.
func readSandboxImage(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read sandbox image pin: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || key != "OPENSHELL_SANDBOX_IMAGE" {
			continue
		}
		if value == "" || strings.ContainsAny(value, " \t\r'\"") {
			return "", fmt.Errorf("%s: OPENSHELL_SANDBOX_IMAGE must be a nonempty unquoted image reference", path)
		}
		return value, nil
	}
	return "", fmt.Errorf("%s: OPENSHELL_SANDBOX_IMAGE is missing", path)
}

// caFile writes the cluster CA to a temp file and returns its path, so the CLI
// process (which cannot share the suite's in-process cert pool) trusts the
// issuer's TLS via SSL_CERT_FILE. Cached for the suite lifetime.
func (s *E2ESuite) caFile(t *testing.T) string {
	s.cliCache.mu.Lock()
	defer s.cliCache.mu.Unlock()
	if s.cliCache.caFilePath != "" {
		return s.cliCache.caFilePath
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
	s.cliCache.caFilePath = f.Name()
	return s.cliCache.caFilePath
}

// cliEnv returns the environment the openshell CLI process needs on Kind: the
// trusted local CA and namespace used by the wrapper. OpenShift uses its publicly trusted Gateway API
// endpoint by default and inherits the caller environment unchanged.
func (s *E2ESuite) cliEnv(t *testing.T) []string {
	env := append([]string(nil), os.Environ()...)
	if s.driver.Name() != "kind" {
		return env
	}
	env = append(env,
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
	ctxName := s.kubeContext
	if ctxName == "" {
		ctxName = os.Getenv("E2E_KUBECONTEXT")
	}
	if ctxName == "" {
		return ""
	}
	s.cliCache.mu.Lock()
	defer s.cliCache.mu.Unlock()
	if s.cliCache.cliKubeconfigPath != "" {
		return s.cliCache.cliKubeconfigPath
	}
	raw, err := clientcmd.NewDefaultClientConfigLoadingRules().Load()
	s.Require().NoError(err, "load kubeconfig for CLI context pin")
	raw.CurrentContext = ctxName
	f, err := os.CreateTemp("", "e2e-kubeconfig-*.yaml")
	s.Require().NoError(err, "create temp kubeconfig")
	s.Require().NoError(f.Close())
	s.Require().NoError(clientcmd.WriteToFile(*raw, f.Name()), "write temp kubeconfig")
	s.cliCache.cliKubeconfigPath = f.Name()
	return s.cliCache.cliKubeconfigPath
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
		GatewayInsecure: gatewayTLSInsecure(s.driver.Name()),
	}
	if s.usesGatewayServiceAccount() {
		// The CLI authenticates as the gateway service account, so its config names
		// that account's client, as the retired Bash suite's mode did. The token
		// passed in may be minutes old; use the current one.
		cred, err := s.serviceAccounts.Acquire(t.Context(), s.serviceAccountClient(), ref)
		s.Require().NoError(err, "gateway service-account token for CLI registration")
		meta.OIDCClientID, token = cred.ClientID, cred.AccessToken
	}
	s.registerGatewayCLI(t, meta, token)
	return meta.Name
}

// cliConfigDirInCLI returns the openshell config directory as the CLI process
// sees it. Both container wrappers bind-mount the host ~/.config/openshell, but
// at different in-container paths (the kind wrapper at /tmp/osh-home/..., the
// OpenShift image at /home/cli/...); a natively-run CLI sees the host path
// itself. A file the suite stages into the host config dir is handed to a CLI
// flag by translating its path through this - the host path is invisible inside
// the container, so passing it directly fails with "No such file or directory".
func (s *E2ESuite) cliConfigDirInCLI() string {
	switch {
	case usesGatewayMatchedCLIImage(s.driver.Name(), os.Getenv("OPENSHELL_BIN")):
		return "/home/cli/.config/openshell"
	case strings.HasSuffix(s.resolveOpenshellBin(), "openshell-container.sh"):
		return "/tmp/osh-home/.config/openshell"
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(".config", "openshell")
		}
		return filepath.Join(home, ".config", "openshell")
	}
}

// stageCLIFile writes content into the host openshell config directory (the one
// directory both CLI container wrappers bind-mount) under name and returns the
// path the CLI process sees for it, so a fixture on the host can be handed to a
// CLI flag like `provider profile import --file`.
func (s *E2ESuite) stageCLIFile(t *testing.T, name string, content []byte) string {
	home, err := os.UserHomeDir()
	s.Require().NoError(err, "resolve home dir")
	hostDir := filepath.Join(home, ".config", "openshell")
	s.Require().NoError(os.MkdirAll(hostDir, 0o700), "create openshell config dir")
	s.Require().NoError(os.WriteFile(filepath.Join(hostDir, name), content, 0o600), "stage CLI file %s", name)
	return filepath.Join(s.cliConfigDirInCLI(), name)
}

// cli runs `openshell -g <local> <args...>` with the CLI environment and returns
// combined output.
func (s *E2ESuite) cli(t *testing.T, local string, args ...string) (string, error) {
	// Service-account tokens live about five minutes; renew before every CLI call
	// (a no-op in identity mode).
	if err := s.renewGatewayCLIToken(t.Context(), local); err != nil {
		return "", fmt.Errorf("renew gateway service-account token for the CLI: %w", err)
	}
	// On OpenShift, run the gateway-version-matched CLI image unless an operator
	// supplied a real path/image override. A generic "openshell" value means the
	// default binary and is not a version pin; using it can skew the sandbox
	// protobuf from the downstream gateway. The gateway is reachable over public
	// DNS, so no cluster network wiring is needed.
	if usesGatewayMatchedCLIImage(s.driver.Name(), os.Getenv("OPENSHELL_BIN")) {
		return s.cliOpenShift(t, local, args...)
	}
	full := append([]string{"-g", local}, args...)
	return s.runCmdEnv(t.Context(), s.cliEnv(t), s.resolveOpenshellBin(), full...)
}

func usesGatewayMatchedCLIImage(driverName, bin string) bool {
	return driverName == "openshift" && (bin == "" || bin == "openshell")
}

// cliOpenShift runs the openshell CLI image (matched to the gateway version) under
// the container engine, mounting the host openshell config.
func (s *E2ESuite) cliOpenShift(t *testing.T, local string, args ...string) (string, error) {
	image := s.gatewayCLIImage(t, strings.TrimSuffix(local, "-openshell"))
	home, err := os.UserHomeDir()
	s.Require().NoError(err, "resolve home dir")
	engine := envOrDefault("CONTAINER_ENGINE", "podman")
	full := openShiftCLICommand(
		engine, home, os.Getuid(), os.Getgid(), image, local,
		gatewayTLSInsecure(s.driver.Name()), args...,
	)
	return s.runCmdEnv(t.Context(), os.Environ(), engine, full...)
}

// openShiftCLICommand constructs a container command that can read the caller's
// private openshell configuration. The CLI image runs as UID 1000, while the CI
// runner normally owns ~/.config with a different UID and mode 0700. Running as
// the caller (and retaining that mapping for Podman) preserves config access.
func openShiftCLICommand(engine, home string, uid, gid int, image, local string, insecure bool, args ...string) []string {
	full := []string{"run", "--rm"}
	isPodman := strings.Contains(filepath.Base(engine), "podman")
	if isPodman {
		full = append(full, "--userns=keep-id")
	}
	full = append(full, "--user", fmt.Sprintf("%d:%d", uid, gid))
	if insecure {
		full = append(full, "-e", "OPENSHELL_GATEWAY_INSECURE=true")
	}
	volume := filepath.Join(home, ".config", "openshell") + ":/home/cli/.config/openshell"
	if isPodman {
		volume += ":z"
	}
	full = append(full,
		"-e", "HOME=/home/cli",
		"-v", volume,
		image, "-g", local,
	)
	return append(full, args...)
}

// gatewayCLIImage derives the openshell CLI image matching the gateway deployed in
// ns (same registry/tag as the gateway image, with the component swapped to -cli).
// Cached for the suite lifetime.
func (s *E2ESuite) gatewayCLIImage(t *testing.T, ns string) string {
	s.cliCache.mu.Lock()
	defer s.cliCache.mu.Unlock()
	if s.cliCache.openshiftCLIImage != "" {
		return s.cliCache.openshiftCLIImage
	}
	dep, err := s.clients.Kube.AppsV1().Deployments(ns).Get(t.Context(), "openshell-gateway", metav1.GetOptions{})
	s.Require().NoError(err, "get gateway deployment to derive CLI image")
	gwImage := dep.Spec.Template.Spec.Containers[0].Image
	// Strip any @sha256 digest, swap the component name to the CLI image.
	if i := strings.Index(gwImage, "@"); i >= 0 {
		gwImage = gwImage[:i]
	}
	cliImage := strings.Replace(gwImage, "odh-openshell-gateway", "odh-openshell-cli", 1)
	s.cliCache.openshiftCLIImage = cliImage
	s.Logf("openshift CLI image: %s", cliImage)
	return cliImage
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
