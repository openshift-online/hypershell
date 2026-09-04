export interface GatewayConnection {
  activeSandboxCount?: number;
  clusterId?: string;
  clusterName: string;
  consoleUrl?: string;
  createdAt?: string;
  createdBy?: string;
  endpoint?: string;
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
  return (
    gateway.phase?.trim().toLocaleLowerCase() === "running" &&
    Boolean(gateway.endpoint)
  );
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

export const installDocsUrl =
  "https://docs.nvidia.com/openshell/about/installation";

export const sandboxConnectDocsUrl =
  "https://docs.nvidia.com/openshell/sandboxes/manage-sandboxes#connect-to-a-sandbox";

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
 * Primary "add a provider" command. Pulls credentials from Application Default
 * Credentials (`--from-gcloud-adc`) and reads the project id from the shell, so
 * no secret has to be pasted into the browser. `providerName` is parameterized so
 * the caller can substitute an edit marker (for highlighting) or the operator's
 * chosen name (for copy) without diverging from this single command template.
 */
export function buildProviderCreateCommand(
  providerName: string = vertexProviderName,
): string {
  return [
    "openshell provider create",
    `--name ${providerName}`,
    "--type google-vertex-ai",
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
    "--env=ANTHROPIC_BASE_URL=https://inference.local",
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
 * One-time setup script that logs in to the gateway, adds the Claude on Vertex AI
 * provider, and selects the model, combined into a single copyable block so
 * operators paste the whole preamble at once instead of stepping through three
 * commands. Returns `undefined` until the gateway is ready to connect, because
 * registration requires a running gateway endpoint; the caller renders a pending
 * state in that case.
 */
export function buildSetupScript(
  gateway: GatewayConnection,
  overrides: { model?: string; providerName?: string } = {},
): string | undefined {
  const gatewayAdd = buildGatewayAddCommand(gateway);
  if (!gatewayAdd) {
    return undefined;
  }

  const { model = claudeModel, providerName = vertexProviderName } = overrides;

  return [
    "# 1. Log in to the gateway",
    gatewayAdd,
    "",
    "# 2. Add the Claude on Vertex AI provider",
    buildProviderCreateCommand(providerName),
    "",
    "# 3. Select the model",
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
