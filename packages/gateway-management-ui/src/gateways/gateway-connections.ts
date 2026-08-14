export interface GatewayConnection {
  clusterId?: string;
  clusterName: string;
  consoleUrl?: string;
  createdAt?: string;
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

function shellArgument(value: string) {
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
 * the create, inference-routing, and sandbox commands refer to the same provider.
 */
export const vertexClaudeProviderName = "vertex-claude";

/** Placeholder a user replaces with their sandbox name. */
export const sandboxNamePlaceholder = "<sandbox-name>";

/** One-time prerequisite that writes Application Default Credentials locally. */
export const gcloudAdcLoginCommand = "gcloud auth application-default login";

/**
 * Primary "add a provider" command. Pulls credentials from Application Default
 * Credentials and the project from the user's active gcloud configuration, so no
 * secret or project value has to be pasted into the browser or edited by hand.
 */
export function buildProviderCreateCommand(): string {
  return [
    "openshell provider create",
    `--name ${vertexClaudeProviderName}`,
    "--type google-vertex-ai",
    "--from-gcloud-adc",
    `--config VERTEX_AI_PROJECT_ID="$(gcloud config get-value project)"`,
    "--config VERTEX_AI_REGION=global",
  ].join(" \\\n  ");
}

/**
 * Alternative that reads every credential and config value from the shell
 * environment (`VERTEX_AI_PROJECT_ID`, `VERTEX_AI_REGION`, and the credential),
 * for users who already export the OpenShell Vertex variables.
 */
export function buildProviderFromExistingCommand(): string {
  return [
    "openshell provider create",
    `--name ${vertexClaudeProviderName}`,
    "--type google-vertex-ai",
    "--from-existing",
  ].join(" \\\n  ");
}

/**
 * Creates a sandbox that launches Claude through the Vertex AI provider. The
 * sandbox name is a template value the user substitutes before running.
 */
export function buildSandboxCreateCommand(
  sandboxName: string = sandboxNamePlaceholder,
): string {
  return [
    "openshell sandbox create",
    `--name ${sandboxName}`,
    `--provider ${vertexClaudeProviderName}`,
    "--no-auto-providers",
    "-- claude",
  ].join(" \\\n  ");
}

export type GatewayStatusAppearance =
  "danger" | "pending" | "progress" | "success" | "unknown" | "warning";

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
    case "pending":
      return "pending";
    case "provisioning":
    case "reconciling":
    case "updating":
      return "progress";
    case "error":
    case "failed":
      return "danger";
    default:
      return "unknown";
  }
}
