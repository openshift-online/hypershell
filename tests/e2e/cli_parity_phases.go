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

// uiInstallCommandVersionSentinel is the placeholder the shared UI install-command
// fixture uses in place of the installer version; see
// packages/gateway-management-ui/src/gateways/ui-install-command.golden.
const uiInstallCommandVersionSentinel = "__OPENSHELL_INSTALLER_VERSION__"

// uiInstallCommandFromFixture returns the UI-rendered install command for
// installerVersion, read from the shared fixture the gateway-management-ui unit
// test keeps in sync with the real TS buildOpenShellInstallCommand. This lets the
// merge gate (which has no browser) assert UI parity without scraping a console.
func (s *E2ESuite) uiInstallCommandFromFixture(installerVersion string) string {
	raw, err := os.ReadFile(filepath.Join(repoRoot(), "packages", "gateway-management-ui", "src", "gateways", "ui-install-command.golden"))
	s.Require().NoError(err, "read shared UI install-command fixture")
	return strings.ReplaceAll(strings.TrimRight(string(raw), "\n"), uiInstallCommandVersionSentinel, installerVersion)
}

// p2_11CLIUIParity validates that the CLI commands the web console renders are
// the commands the suite executes, driven by the live reconciled
// gateway_version rather than a pinned constant (E2E-COV-05).
func (s *E2ESuite) p2_11CLIUIParity(t *testing.T) {
	s.ensurePrimaryGateway(t)
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

	// E2E-COV-05.01: the UI-rendered install command must string-equal the command
	// the suite executes, and this phase runs on the merge gate where no browser is
	// available. The UI command is captured in a shared fixture
	// (packages/gateway-management-ui/src/gateways/ui-install-command.golden) that the gateway-management-ui
	// unit test checks against the real TS buildOpenShellInstallCommand, so the
	// fixture cannot silently drift from the UI. Substitute the live installer
	// version and require the suite's independently derived command to equal it
	// exactly; the install subtest below then executes that version end to end. On
	// pull_request the console.live suite additionally scrapes the real rendered
	// command from the live console. E2E-COV-05.04 (version derivation) is
	// unit-tested in cli_parity_phases_test.go.
	uiCommand := s.uiInstallCommandFromFixture(uiVersion)
	suiteCommand := buildOpenShellInstallCommand(uiVersion)
	s.Require().Equal(uiCommand, suiteCommand, "UI-rendered install command (shared fixture) string-equals the command the suite derives and executes (E2E-COV-05.01)")

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

// assertProviderCreateCommand runs the provider-create path end to end against the
// connected primary gateway (E2E-COV-05.03). The spec explicitly accepts a
// credential-free equivalent of the UI's --from-gcloud-adc command, so this uses
// --runtime-credentials (the provider's credentials are resolved at runtime by the
// gateway/sandbox). That needs no cloud credentials, so the subtest runs
// unconditionally on the Kind merge gate instead of skipping. s.cli execs the
// binary's argv directly, not through a shell.
func (s *E2ESuite) assertProviderCreateCommand(t *testing.T) {
	providerName := "e2e-parity-" + s.runID
	// For reference, log the command the UI renders; the suite executes the
	// credential-free equivalent below.
	s.runner.Show("%s  # UI-rendered form; the suite runs the --runtime-credentials equivalent", buildProviderCreateCommand(providerName))
	// The e2e gateway imports no provider profiles by default (profile sources =
	// ["user"]), and the google-vertex-ai profile the UI example renders is not
	// registered, so seed a minimal profile whose single credential is minted by
	// the gateway at runtime (so --runtime-credentials is valid and needs no static
	// value at create time) and create a provider against it. E2E-COV-05.03 only
	// requires the provider-create path to run end to end, not a specific provider
	// type.
	profileBytes, err := os.ReadFile(filepath.Join(repoRoot(), "tests", "e2e", "testdata", "e2e-provider-profile.yaml"))
	s.Require().NoError(err, "read the e2e provider profile fixture")
	// The CLI runs inside a container that only bind-mounts ~/.config/openshell, so
	// stage the fixture there and pass the path the containerized CLI actually sees.
	profilePath := s.stageCLIFile(t, "e2e-provider-profile-"+s.runID+".yaml", profileBytes)
	out, err := s.cli(t, s.primaryLocalName, "provider", "profile", "import", "--file", profilePath)
	s.Require().NoErrorf(err, "seed the credential-free e2e provider profile: %s", out)
	out, err = s.cli(t, s.primaryLocalName, "provider", "create",
		"--name", providerName,
		"--type", "e2e-parity",
		"--runtime-credentials",
	)
	s.Require().NoErrorf(err, "credential-free provider create (E2E-COV-05.03): %s", out)
	defer func() { _, _ = s.cli(t, s.primaryLocalName, "provider", "delete", providerName) }()

	out, err = s.cli(t, s.primaryLocalName, "provider", "get", providerName)
	s.Require().NoErrorf(err, "verify provider %s was created: %s", providerName, out)
}
