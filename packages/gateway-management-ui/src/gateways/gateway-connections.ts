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
  if (
    !gateway.endpoint ||
    !gateway.oidcAudience ||
    !gateway.oidcClientId ||
    !gateway.oidcIssuer
  ) {
    return undefined;
  }
  return [
    "openshell gateway add",
    `--name ${shellArgument(gateway.name)}`,
    `--oidc-issuer ${shellArgument(gateway.oidcIssuer)}`,
    `--oidc-client-id ${shellArgument(gateway.oidcClientId)}`,
    `--oidc-audience ${shellArgument(gateway.oidcAudience)}`,
    shellArgument(gateway.endpoint),
  ].join(" ");
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
