export interface GatewayConnection {
  clusterName: string;
  consoleUrl?: string;
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

export function gatewayStatusColor(
  status: string,
): "blue" | "green" | "grey" | "orange" | "yellow" {
  switch (status.trim().toLocaleLowerCase()) {
    case "active":
    case "available":
    case "ready":
    case "succeeded":
      return "green";
    case "degraded":
    case "warning":
      return "yellow";
    case "pending":
    case "provisioning":
    case "reconciling":
    case "updating":
      return "blue";
    case "error":
    case "failed":
      return "orange";
    default:
      return "grey";
  }
}
