import { gatewayCanonicalPhases } from "./gateway-data";
import { externalLinkUrls } from "../application/external-links";

export interface GatewayConnection {
  activeSandboxCount?: number;
  canDelete?: boolean;
  canEdit?: boolean;
  clusterId: string;
  clusterName: string;
  consoleUrl?: string;
  createdAt?: string;
  createdBy?: string;
  endpoint?: string;
  gatewayVersion?: string;
  id: string;
  name: string;
  oidcAudience?: string;
  oidcClientId?: string;
  oidcIssuer?: string;
  phase?: string;
  status: string;
}

/**
 * Whether the gateway should be presented as ready to connect. A published
 * `endpoint` (route address) records *where* the gateway will be reachable but
 * does not by itself assert readiness: the endpoint may be populated while the
 * gateway is still `Provisioning` and not yet programmed/routable. The control
 * plane only advances `phase` to `Running` once the workload and, for routed
 * gateways, the external exposure are observed Ready, so `phase === "Running"`
 * is the single readiness gate for surfacing the connection command. See
 * specs/platform/openshell-gateway-health.spec.md § Connection Command Surfaced
 * Only When Ready.
 */
export function isGatewayReadyToConnect(gateway: GatewayConnection): boolean {
  const phase = gateway.phase?.trim().toLocaleLowerCase();
  return phase === gatewayCanonicalPhases.running && Boolean(gateway.endpoint);
}

const safeShellArgument = /^[A-Za-z0-9_./:@%+=,-]+$/;

export function shellArgument(value: string) {
  if (safeShellArgument.test(value)) {
    return value;
  }

  return `'${value.replaceAll("'", `'"'"'`)}'`;
}

export function buildGatewayAddCommand(
  gateway: GatewayConnection,
): string | undefined {
  if (!isGatewayReadyToConnect(gateway) || !gateway.endpoint) {
    return undefined;
  }

  const parts = [
    "openshell gateway add",
    `--name ${shellArgument(gateway.name)}`,
  ];

  if (gateway.oidcIssuer && gateway.oidcClientId && gateway.oidcAudience) {
    parts.push(
      `--oidc-issuer ${shellArgument(gateway.oidcIssuer)}`,
      `--oidc-client-id ${shellArgument(gateway.oidcClientId)}`,
      `--oidc-audience ${shellArgument(gateway.oidcAudience)}`,
    );
  }

  parts.push(shellArgument(gateway.endpoint));

  return parts.join(" \\\n  ");
}

/**
 * Fixed OpenShell provider name used across the Vertex AI connection steps so
 * the provider, inference, and sandbox commands all refer to the same provider.
 */
export const vertexProviderName = "my-gcp";

export const installDocsUrl = externalLinkUrls.openShellCliInstallation;

const installScriptUrl = externalLinkUrls.openShellInstallerScript;

/**
 * Builds an installation command that matches the reconciled gateway version.
 *
 * The full version string is passed through, suffix included (e.g.
 * "v0.0.116-rhaiv.6"), not truncated to its base semver. A suffix identifies a
 * downstream build that can be ahead of the last tagged public OpenShell
 * release and is not guaranteed proto-compatible with it; install-openshell.sh
 * pulls that exact build from quay.io/opendatahub/odh-openshell-cli instead of
 * a public release when the suffix is present. Keep this in sync with
 * openshell_cli_image_tag in tests/e2e/lib.sh.
 */
export function buildOpenShellInstallCommand(
  gateway: GatewayConnection,
): string | undefined {
  const gatewayVersion = gateway.gatewayVersion?.trim();
  if (!isGatewayReadyToConnect(gateway) || !gatewayVersion) {
    return undefined;
  }

  const installerVersion = gatewayVersion.startsWith("v")
    ? gatewayVersion
    : `v${gatewayVersion}`;

  return [
    "curl -LsSf \\",
    `  ${installScriptUrl} \\`,
    `  | OPENSHELL_VERSION=${shellArgument(installerVersion)} sh`,
    'export PATH="$HOME/.local/bin:$PATH"',
  ].join("\n");
}

export const sandboxConnectDocsUrl = externalLinkUrls.sandboxConnectDocs;

/** Default sandbox name shown in the copyable create-sandbox command. */
export const sandboxName = "mysand";

/** Claude model the sandbox runs, shared by the inference and sandbox commands. */
export const claudeModel = "claude-haiku-4-5";

// Sandbox resource defaults -- keep in sync with:
//   components/cli/cmd/hypershell/get/gateway/cmd.go (sandboxDriverConfig)
//   specs/web-console/architecture.spec.md § Create a sandbox
export const sandboxResourceDefaults = {
  requests: { cpu: "100m", memory: "512Mi" },
  limits: { cpu: "500m", memory: "512Mi" },
} as const;

/**
 * Default provider profile file for the Vertex AI example used across the
 * connection steps. Keep the provider type in sync with
 * `buildProviderCreateCommand`'s `--type google-vertex-ai`, and keep this
 * literal in sync with `providerProfile` in
 * components/cli/pkg/gatewayconnect/gatewayconnect.go.
 *
 * See specs/platform/hsctl-gateway-connection.spec.md (CLI-GWC-01) and
 * specs/web-console/architecture.spec.md § WEB-UIEX-05 step 1 (one-time setup).
 */
export const vertexProviderProfileFile = "google-vertex-ai.yaml";

/**
 * Documentation that explains how to obtain and import an OpenShell provider
 * profile for a given provider type (the "command or link that downloads or
 * locates the profile file" required by CLI-GWC-01 / WEB-UIEX-05 step 1).
 */
export const providerProfileDocsUrl =
  "https://docs.nvidia.com/openshell/providers/manage-providers#provider-profiles";

/**
 * Public source for the Vertex AI example's provider profile file. Keep this
 * literal in sync with `providerProfileURL` in
 * components/cli/pkg/gatewayconnect/gatewayconnect.go.
 */
export const vertexProviderProfileUrl =
  "https://raw.githubusercontent.com/NVIDIA/OpenShell/main/providers/google-vertex-ai.yaml";

/**
 * Builds the command that downloads the provider profile file, satisfying the
 * "command ... that downloads or locates that profile file" requirement
 * (CLI-GWC-01 / WEB-UIEX-05 step 1) with an actual command rather than only a
 * documentation link. Must run before `buildProviderProfileImportCommand`,
 * which imports the file this downloads.
 */
export function buildProviderProfileDownloadCommand(
  profileUrl: string = vertexProviderProfileUrl,
): string {
  return `curl -LsSfO ${shellArgument(profileUrl)}`;
}

/**
 * Builds the `openshell provider profile import` command that registers a
 * provider type's profile in the gateway's profile catalog. This MUST run
 * before `buildProviderCreateCommand` for the same provider type, or
 * `provider create` fails with "provider profile '<type>' not found".
 *
 * Uses `--global` scope, which `buildProviderCreateCommand` matches with its
 * own `--global-profile` flag -- the import and create scopes must agree, or
 * the "profile not found" error reproduces even though the profile has
 * already been imported. See
 * specs/platform/hsctl-gateway-connection.spec.md (CLI-GWC-02).
 */
export function buildProviderProfileImportCommand(
  profileFile: string = vertexProviderProfileFile,
): string {
  return `openshell provider profile import -f ${shellArgument(profileFile)} --global`;
}

/**
 * Primary "add a provider" command. Pulls credentials from Application Default
 * Credentials (`--from-gcloud-adc`) and reads the project id from the shell, so
 * no secret has to be pasted into the browser. `providerName` is parameterized so
 * the caller can substitute an edit marker (for highlighting) or the operator's
 * chosen name (for copy) without diverging from this single command template.
 *
 * `--global-profile` resolves the profile imported by
 * `buildProviderProfileImportCommand`'s matching `--global` scope.
 */
export function buildProviderCreateCommand(
  providerName: string = vertexProviderName,
): string {
  return [
    "openshell provider create",
    `--name ${providerName}`,
    "--type google-vertex-ai",
    "--global-profile",
    "--from-gcloud-adc",
    '--config VERTEX_AI_PROJECT_ID="$ANTHROPIC_VERTEX_PROJECT_ID"',
    "--config VERTEX_AI_REGION=global",
  ].join(" \\\n  ");
}

/**
 * Points the provider's inference at the Claude model the sandbox runs, so
 * `claude` inside the sandbox resolves to it without any extra flags at the
 * provider level. `providerName` and `model` are parameterized so the same
 * template drives both the highlighted (marker) and copyable (resolved) forms.
 */
export function buildInferenceSetCommand(
  providerName: string = vertexProviderName,
  model: string = claudeModel,
): string {
  return `openshell inference set --provider ${providerName} --model ${model}`;
}

/**
 * Builds a complete shell script for creating an OpenShell sandbox with resource limits.
 * Returns a multi-line string containing:
 * 1. DRIVER_CONFIG variable assignment with JSON resource specification
 * 2. openshell sandbox create command referencing $DRIVER_CONFIG
 *
 * The DRIVER_CONFIG variable is extracted into a shell variable for readability,
 * avoiding an unwieldy inline JSON argument in the command itself.
 */
export function buildSandboxCreateCommand(
  name: string = sandboxName,
  model: string = claudeModel,
): string {
  const driverConfig = JSON.stringify({
    kubernetes: {
      containers: {
        agent: {
          resources: sandboxResourceDefaults,
        },
      },
    },
  });

  const variable = `DRIVER_CONFIG='${driverConfig}'`;

  const command = [
    "openshell sandbox create",
    `--name ${shellArgument(name)}`,
    '--driver-config-json "$DRIVER_CONFIG"',
    `--env=ANTHROPIC_BASE_URL=${externalLinkUrls.localInferenceEndpoint}`,
    "--env=ANTHROPIC_API_KEY=unused",
    "--no-auto-providers",
    `-- claude --bare --model ${model}`,
  ].join(" \\\n  ");

  return `${variable}\n\n${command}`;
}

export const validEditors = ["cursor", "vscode"] as const;
export const defaultEditor = "cursor";

export function buildSandboxConnectCommand(
  name: string = sandboxName,
  editor: string = defaultEditor,
): string {
  return [
    `openshell sandbox connect ${shellArgument(name)}`,
    `--editor ${shellArgument(editor)}`,
  ].join(" \\\n  ");
}

/**
 * Builds the one-time gateway registration and provider setup commands. The
 * function returns `undefined` until the gateway is ready.
 */
export function buildOneTimeSetupScript(
  gateway: GatewayConnection,
  overrides: { model?: string; providerName?: string } = {},
): string | undefined {
  const gatewayAdd = buildGatewayAddCommand(gateway);
  if (!gatewayAdd) {
    return undefined;
  }

  const { model = claudeModel, providerName = vertexProviderName } = overrides;

  return [
    "# 1. Register the gateway",
    gatewayAdd,
    "",
    "# 2. Download and import the provider profile",
    buildProviderProfileDownloadCommand(),
    buildProviderProfileImportCommand(),
    "",
    "# 3. Add the Claude on Vertex AI provider",
    buildProviderCreateCommand(providerName),
    "",
    "# 4. Select the model",
    buildInferenceSetCommand(providerName, model),
  ].join("\n");
}

export type GatewayStatusAppearance =
  | "danger"
  | "inactive"
  | "pending"
  | "progress"
  | "success"
  | "unknown"
  | "warning";

export function gatewayStatusAppearance(
  status: string,
): GatewayStatusAppearance {
  switch (status.trim().toLocaleLowerCase()) {
    case "active":
    case "available":
    case "healthy":
    case "ready":
    case "running":
    case "succeeded":
      return "success";
    case "degraded":
    case "warning":
      return "warning";
    case "expired":
    case "revoked":
      return "inactive";
    case "pending":
      return "pending";
    case "provisioning":
    case "reconciling":
    case "revoking":
    case "deleting":
    case "updating":
      return "progress";
    case "error":
    case "failed":
      return "danger";
    default:
      return "unknown";
  }
}
