package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

// installScriptURL is the public URL the UI's buildOpenShellInstallCommand
// renders. Keep in sync with
// packages/gateway-management-ui/src/gateways/gateway-connections.ts.
const installScriptURL = "https://raw.githubusercontent.com/openshift-online/hypershell/main/scripts/install-openshell.sh"

var safeShellArgumentRE = regexp.MustCompile(`^[A-Za-z0-9_./:@%+=,-]+$`)

// shellArgument ports shellArgument from gateway-connections.ts.
func shellArgument(value string) string {
	if safeShellArgumentRE.MatchString(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

// uiInstallerVersion ports the version-derivation half of
// buildOpenShellInstallCommand (gateway-connections.ts): trimmed, with a
// leading "v" added when absent. The suffix, if any, is preserved.
func uiInstallerVersion(gatewayVersion string) (string, error) {
	raw := strings.TrimSpace(gatewayVersion)
	if raw == "" {
		return "", fmt.Errorf("gateway_version is empty")
	}
	if strings.HasPrefix(raw, "v") {
		return raw, nil
	}
	return "v" + raw, nil
}

// suiteInstallerVersion ports openshell_cli_image_tag (tests/e2e/lib.sh): the
// same trim-and-prefix rule, implemented independently from uiInstallerVersion
// so the two stay a real cross-check rather than one function called twice.
func suiteInstallerVersion(gatewayVersion string) (string, error) {
	raw := strings.TrimSpace(gatewayVersion)
	if raw == "" {
		return "", fmt.Errorf("gateway_version is empty")
	}
	if strings.HasPrefix(raw, "v") {
		return raw, nil
	}
	return "v" + raw, nil
}

// buildOpenShellInstallCommand ports buildOpenShellInstallCommand
// (gateway-connections.ts) byte-for-byte, given an already-derived installer
// version (a "v..." string).
func buildOpenShellInstallCommand(installerVersion string) string {
	return strings.Join([]string{
		`curl -LsSf \`,
		"  " + installScriptURL + ` \`,
		"  | OPENSHELL_VERSION=" + shellArgument(installerVersion) + " sh",
		`export PATH="$HOME/.local/bin:$PATH"`,
	}, "\n")
}

// buildProviderCreateCommand ports buildProviderCreateCommand
// (gateway-connections.ts) for the fixed vertexProviderName default used there.
func buildProviderCreateCommand(providerName string) string {
	return strings.Join([]string{
		"openshell provider create",
		"--name " + providerName,
		"--type google-vertex-ai",
		"--from-gcloud-adc",
		`--config VERTEX_AI_PROJECT_ID="$ANTHROPIC_VERTEX_PROJECT_ID"`,
		"--config VERTEX_AI_REGION=global",
	}, " \\\n  ")
}

// suffixedVersionRE matches install-openshell.sh's own case pattern for a
// downstream-suffixed version (the quay/container install path).
var suffixedVersionRE = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+-`)

// openshellCLIMatchesVersion ports openshell_cli_matches_version
// (tests/e2e/lib.sh): reportedVersion is `openshell --version` output
// ("openshell <version>"), compared to gatewayVersion with the "v" prefix
// insignificant on either side.
func openshellCLIMatchesVersion(reportedVersion, gatewayVersion string) bool {
	fields := strings.Fields(stripANSI(reportedVersion))
	if len(fields) < 2 || fields[0] != "openshell" {
		return false
	}
	return strings.TrimPrefix(fields[1], "v") == strings.TrimPrefix(strings.TrimSpace(gatewayVersion), "v")
}

// p2_11CLIUIParity validates that the CLI commands the web console renders are
// the commands the suite executes, driven by the live reconciled
// gateway_version rather than a pinned constant (E2E-COV-05).
func (s *E2ESuite) p2_11CLIUIParity(t *testing.T) {
	ctx := t.Context()
	// gateway_version is written back asynchronously by the health reconciler once
	// the live gateway reports its runtime version (api-server SetGatewayVersion),
	// so poll for it rather than reading once: a freshly provisioned gateway can
	// still have an empty gateway_version when this parallel phase first runs.
	waitErr := pollNoCtx(durationSecondsEnv("E2E_GATEWAY_VERSION_TIMEOUT", 120*time.Second), 5*time.Second, func() bool {
		gw, err := s.admin.Gateways().Get(ctx, s.primary.ID)
		return err == nil && gw.GatewayVersion != ""
	})
	s.Require().NoError(waitErr, "primary gateway did not report a reconciled gateway_version")

	running, err := s.admin.Gateways().Get(ctx, s.primary.ID)
	s.Require().NoError(err, "get primary gateway for its live reconciled gateway_version")
	s.Require().NotEmpty(running.GatewayVersion, "primary gateway has a reconciled gateway_version")

	uiVersion, err := uiInstallerVersion(running.GatewayVersion)
	s.Require().NoError(err, "derive installer version from gateway_version")
	// E2E-COV-05.01/05.04 - that the command the UI RENDERS equals the real
	// version-derived command - is verified against the LIVE console by the
	// console.live Playwright scrape ("shows placement and CLI installation
	// choices"), which reads the exact command the UI hands the operator (via its
	// copy button). Comparing two copies of the Go port of buildOpenShellInstallCommand
	// here would be tautological and blind to UI drift, so this phase instead proves
	// the version end to end: it executes the derived installer and the rendered
	// provider-create command. The Go port's own correctness is unit-tested in
	// cli_parity_phases_test.go.

	t.Run("install-produces-matching-cli", func(t *testing.T) {
		s.assertInstallProducesMatchingCLI(t, uiVersion, running.GatewayVersion)
	})
	t.Run("provider-create", func(t *testing.T) {
		s.assertProviderCreateCommand(t)
	})
}

// assertInstallProducesMatchingCLI executes install-openshell.sh with
// installerVersion into an isolated temp dir (so it does not perturb the
// shared CLI binary other phases resolve through resolveOpenshellBin) and
// asserts the installed CLI reports gatewayVersion (E2E-COV-05.02).
func (s *E2ESuite) assertInstallProducesMatchingCLI(t *testing.T, installerVersion, gatewayVersion string) {
	if runtime.GOOS != "linux" && suffixedVersionRE.MatchString(installerVersion) {
		t.Skipf("install-openshell.sh's quay/container path for the downstream build %s requires a Linux host (this host is %s)", installerVersion, runtime.GOOS)
	}
	scriptPath := filepath.Join(repoRoot(), "scripts", "install-openshell.sh")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Skipf("install-openshell.sh not found at %s", scriptPath)
	}

	installDir, err := os.MkdirTemp("", "e2e-openshell-install-*")
	s.Require().NoError(err, "create isolated install dir")
	defer func() { _ = os.RemoveAll(installDir) }()

	env := append(append([]string(nil), os.Environ()...), "OPENSHELL_VERSION="+installerVersion, "OPENSHELL_INSTALL_DIR="+installDir)
	out, err := s.runCmdEnv(t.Context(), env, "sh", scriptPath)
	s.Require().NoErrorf(err, "install-openshell.sh %s: %s", installerVersion, out)

	versionOut, err := s.runCmdEnv(t.Context(), os.Environ(), filepath.Join(installDir, "openshell"), "--version")
	s.Require().NoErrorf(err, "run installed openshell --version: %s", versionOut)
	s.Assert().Truef(openshellCLIMatchesVersion(versionOut, gatewayVersion), "installed CLI %q does not match deployed gateway version %q", strings.TrimSpace(versionOut), gatewayVersion)
}

// assertProviderCreateCommand executes the flags buildProviderCreateCommand
// renders against the connected primary gateway (E2E-COV-05.03), gated on real
// GCP Application Default Credentials because --from-gcloud-adc has none that
// are credential-free; invoked as structured CLI args (s.cli execs the binary
// directly, not through a shell) rather than the rendered shell text.
func (s *E2ESuite) assertProviderCreateCommand(t *testing.T) {
	projectID := os.Getenv("ANTHROPIC_VERTEX_PROJECT_ID")
	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") == "" || projectID == "" {
		t.Skip("provider-create parity requires GOOGLE_APPLICATION_CREDENTIALS (gcloud ADC) and ANTHROPIC_VERTEX_PROJECT_ID; neither --from-gcloud-adc nor a credential-free equivalent is available")
	}
	providerName := "e2e-parity-" + s.runID
	// s.cli execs the openshell binary's argv directly rather than through a
	// shell, so the UI-rendered command is executed as these equivalent
	// structured args rather than its literal backslash-continued text.
	s.runner.Show("%s  # executed as structured CLI args", buildProviderCreateCommand(providerName))
	out, err := s.cli(t, s.primaryLocalName, "provider", "create",
		"--name", providerName,
		"--type", "google-vertex-ai",
		"--from-gcloud-adc",
		"--config", "VERTEX_AI_PROJECT_ID="+projectID,
		"--config", "VERTEX_AI_REGION=global",
	)
	s.Require().NoErrorf(err, "UI-rendered provider create command: %s", out)
	defer func() { _, _ = s.cli(t, s.primaryLocalName, "provider", "delete", providerName) }()

	out, err = s.cli(t, s.primaryLocalName, "provider", "get", providerName)
	s.Require().NoErrorf(err, "verify provider %s was created: %s", providerName, out)
}
