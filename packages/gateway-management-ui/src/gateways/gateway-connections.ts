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
 * the provider, inference, and sandbox commands all refer to the same provider.
 */
export const vertexProviderName = "my-gcp";

/** Default sandbox name shown in the copyable create-sandbox command. */
export const sandboxName = "mysand";

/** Claude model the sandbox runs, shared by the inference and sandbox commands. */
export const claudeModel = "claude-haiku-4-5";

/**
 * Primary "add a provider" command. Pulls credentials from Application Default
 * Credentials (`--from-gcloud-adc`) and reads the project id from the shell, so
 * no secret has to be pasted into the browser.
 */
export function buildProviderCreateCommand(): string {
  return [
    "openshell provider create",
    `--name ${vertexProviderName}`,
    "--type google-vertex-ai",
    "--from-gcloud-adc",
    '--config VERTEX_AI_PROJECT_ID="$ANTHROPIC_VERTEX_PROJECT_ID"',
    "--config VERTEX_AI_REGION=global",
  ].join(" \\\n  ");
}

/**
 * Points the provider's inference at the Claude model the sandbox runs, so
 * `claude` inside the sandbox resolves to it without any extra flags at the
 * provider level.
 */
export function buildInferenceSetCommand(): string {
  return `openshell inference set --provider ${vertexProviderName} --model ${claudeModel}`;
}

/**
 * Creates a sandbox that runs Claude against the gateway's local inference
 * endpoint. `ANTHROPIC_BASE_URL` points at the in-sandbox inference proxy and
 * the API key is unused because the provider supplies credentials, so nothing
 * secret is pasted into the browser. The model is selected once in the
 * `inference set` step, so `claude` needs no `--model` flag here.
 */
export function buildSandboxCreateCommand(name: string = sandboxName): string {
  return [
    "openshell sandbox create",
    `--name ${name}`,
    "--env=ANTHROPIC_BASE_URL=https://inference.local",
    "--env=ANTHROPIC_API_KEY=unused",
    "-- claude --bare",
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
): string | undefined {
  const gatewayAdd = buildGatewayAddCommand(gateway);
  if (!gatewayAdd) {
    return undefined;
  }

  return [
    "# 1. Log in to the gateway",
    gatewayAdd,
    "",
    "# 2. Add the Claude on Vertex AI provider",
    buildProviderCreateCommand(),
    "",
    "# 3. Select the model",
    buildInferenceSetCommand(),
  ].join("\n");
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
