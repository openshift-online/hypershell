export interface GatewayConnection {
  clusterName: string;
  consoleUrl: string;
  endpoint: string;
  id: string;
  name: string;
  oidcAudience: string;
  oidcClientId: string;
  oidcIssuer: string;
  status: string;
}

const safeShellArgument = /^[A-Za-z0-9_./:@%+=,-]+$/;

function shellArgument(value: string) {
  if (safeShellArgument.test(value)) {
    return value;
  }

  return `'${value.replaceAll("'", `'"'"'`)}'`;
}

export function buildGatewayAddCommand(gateway: GatewayConnection) {
  return [
    "openshell gateway add",
    `--name ${shellArgument(gateway.name)}`,
    `--oidc-issuer ${shellArgument(gateway.oidcIssuer)}`,
    `--oidc-client-id ${shellArgument(gateway.oidcClientId)}`,
    `--oidc-audience ${shellArgument(gateway.oidcAudience)}`,
    shellArgument(gateway.endpoint),
  ].join(" ");
}

export function gatewayStatusColor(status: string): "green" | "grey" {
  return status.trim().toLocaleLowerCase() === "unknown" ? "grey" : "green";
}

/**
 * Preview data for the connection-oriented UI. The current Gateway API does
 * not yet return the OIDC or console fields required by this view model.
 */
export const previewGateway: GatewayConnection = {
  clusterName: "Local cluster",
  consoleUrl:
    "https://openshell-dashboard-openshell.apps.rosa.gkrumbac.9bpp.p3.openshiftapps.com",
  endpoint:
    "https://openshell-gw-openshell-gateway-test.apps.rosa.hcmais01ue1.s9m2.p3.openshiftapps.com:443",
  id: "openshell-gateway-test",
  name: "openshell-gateway-test",
  oidcAudience: "openshell-cli",
  oidcClientId: "openshell-cli",
  oidcIssuer:
    "https://keycloak-openshell-keycloak.apps.rosa.hcmais01ue1.s9m2.p3.openshiftapps.com/realms/openshell",
  status: "Ready",
};

export const previewGateways: readonly GatewayConnection[] = [previewGateway];

export function getPreviewGateway(gatewayId: string) {
  const gateway = previewGateways.find(({ id }) => id === gatewayId);
  if (gateway) {
    return gateway;
  }

  return { ...previewGateway, id: gatewayId, name: gatewayId };
}
