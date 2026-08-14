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
  status: string;
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
  if (!gateway.endpoint) {
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

/** Placeholder a user replaces with a Vertex Claude model ID. */
export const inferenceModelPlaceholder = "<claude-model>";

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
 * Routes a Claude model through the Vertex AI provider. The model is a template
 * value the user substitutes before running, so it is not shell-escaped here.
 */
export function buildInferenceSetCommand(
  model: string = inferenceModelPlaceholder,
): string {
  return [
    "openshell inference set",
    `--provider ${vertexClaudeProviderName}`,
    `--model ${model}`,
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
