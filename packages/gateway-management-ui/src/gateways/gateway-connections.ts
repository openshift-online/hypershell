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
 * the create, sandbox, and policy commands refer to the same provider. The
 * sandbox policy binds credentials to this exact provider name.
 */
export const vertexProviderName = "my-gcp";

/** Default sandbox name shown in the copyable create-sandbox command. */
export const sandboxName = "mysand";

/** File the sandbox policy heredoc writes, referenced by the `--policy` flag. */
export const sandboxPolicyFileName = "vertex-policy.yaml";

/**
 * Primary "add a provider" command. Pulls credentials from Application Default
 * Credentials (`--from-gcloud-adc`) and reads the project id from the shell, so
 * no secret has to be pasted into the browser.
 */
export function buildProviderCreateCommand(): string {
  return [
    "openshell provider create",
    `--name ${vertexProviderName}`,
    "--type google-cloud",
    "--from-gcloud-adc",
    "--config project_id=$ANTHROPIC_VERTEX_PROJECT_ID",
    "--config region=global",
  ].join(" \\\n  ");
}

/**
 * Creates a sandbox that runs Claude through the provider, forwarding the Vertex
 * project id and applying the sandbox network/filesystem policy file.
 */
export function buildSandboxCreateCommand(name: string = sandboxName): string {
  return [
    "openshell sandbox create",
    `--name ${name}`,
    `--provider ${vertexProviderName}`,
    "--env=ANTHROPIC_VERTEX_PROJECT_ID=$ANTHROPIC_VERTEX_PROJECT_ID",
    "--env=CLAUDE_CODE_USE_VERTEX=1",
    `--policy ${sandboxPolicyFileName}`,
    "--no-auto-providers",
    "-- claude",
  ].join(" \\\n  ");
}

/**
 * Sandbox policy that constrains the filesystem and pins outbound network access
 * to the Vertex AI endpoints, binding each to the provider's credentials.
 * Rendered as a heredoc so the whole file can be copied and written in one step
 * before `openshell sandbox create --policy`.
 */
export function buildSandboxPolicyCommand(): string {
  const policy = `version: 1

filesystem_policy:
  include_workdir: true
  read_only:
    - /usr
    - /lib
    - /proc
    - /dev/urandom
    - /app
    - /etc
    - /var/log
  read_write:
    - /tmp
    - /dev/null

landlock:
  compatibility: best_effort

network_policies:
  vertex_direct:
    name: vertex_direct
    endpoints:
      - host: "*-aiplatform.googleapis.com"
        port: 443
        protocol: rest
        tls: terminate
        enforcement: enforce
        access: read-write
        credential_binding:
          provider: ${vertexProviderName}
      - host: "aiplatform.googleapis.com"
        port: 443
        protocol: rest
        tls: terminate
        enforcement: enforce
        access: read-write
        credential_binding:
          provider: ${vertexProviderName}
      - host: "aiplatform.us.rep.googleapis.com"
        port: 443
        protocol: rest
        tls: terminate
        enforcement: enforce
        access: read-write
        credential_binding:
          provider: ${vertexProviderName}
      - host: "aiplatform.eu.rep.googleapis.com"
        port: 443
        protocol: rest
        tls: terminate
        enforcement: enforce
        access: read-write
        credential_binding:
          provider: ${vertexProviderName}
    binaries:
      - { path: /usr/local/bin/claude }`;

  return `cat > ${sandboxPolicyFileName} <<'EOF'\n${policy}\nEOF`;
}

/**
 * One-time setup script that logs in to the gateway, adds the Claude on Vertex AI
 * provider, and writes the sandbox policy file, combined into a single copyable
 * block so operators paste the whole preamble at once instead of stepping through
 * three commands. Returns `undefined` until the gateway is ready to connect,
 * because registration requires a running gateway endpoint; the caller renders a
 * pending state in that case.
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
    "# 3. Save the sandbox policy file (referenced by --policy below)",
    buildSandboxPolicyCommand(),
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
