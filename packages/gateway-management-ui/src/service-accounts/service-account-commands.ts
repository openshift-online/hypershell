import type { OpenShellGatewayServiceAccountConnection } from "../application/gateway-types";
import { shellArgument } from "../gateways/gateway-connections";

const secretEnvironmentVariable = "OPENSHELL_OIDC_CLIENT_SECRET";

function hasRequiredConnectionValues(
  connection: OpenShellGatewayServiceAccountConnection,
): boolean {
  return [
    connection.audience,
    connection.clientId,
    connection.gatewayEndpoint,
    connection.gatewayName,
    connection.issuer,
    connection.tokenEndpoint,
  ].every((value) => Boolean(value?.trim()));
}

function secretPrompt(): string[] {
  return [
    `if [ -z "\${${secretEnvironmentVariable}:-}" ]; then`,
    `  read -rsp 'OpenShell OIDC client secret: ' ${secretEnvironmentVariable}`,
    "  printf '\\n'",
    `  export ${secretEnvironmentVariable}`,
    "fi",
  ];
}

export function serviceAccountGatewayAlias(
  gatewayName: string,
  serviceAccountName: string,
): string {
  return `${gatewayName}-${serviceAccountName}`;
}

export function buildOpenShellServiceAccountScript(
  serviceAccountName: string,
  connection: OpenShellGatewayServiceAccountConnection,
): string | undefined {
  if (!hasRequiredConnectionValues(connection) || !serviceAccountName.trim()) {
    return undefined;
  }
  const alias = serviceAccountGatewayAlias(
    connection.gatewayName,
    serviceAccountName,
  );
  return [
    ...secretPrompt(),
    "",
    [
      "openshell gateway add",
      `--name ${shellArgument(alias)}`,
      `--oidc-issuer ${shellArgument(connection.issuer)}`,
      `--oidc-client-id ${shellArgument(connection.clientId)}`,
      `--oidc-audience ${shellArgument(connection.audience)}`,
      shellArgument(connection.gatewayEndpoint ?? ""),
    ].join(" \\\n  "),
    "",
    `openshell -g ${shellArgument(alias)} whoami --output json`,
  ].join("\n");
}

export function buildClientCredentialsScript(
  connection: OpenShellGatewayServiceAccountConnection,
): string | undefined {
  if (!hasRequiredConnectionValues(connection)) {
    return undefined;
  }
  return [
    ...secretPrompt(),
    "",
    "ACCESS_TOKEN=$(printf '%s' \"$OPENSHELL_OIDC_CLIENT_SECRET\" | \\",
    `  curl --fail --silent --show-error --request POST ${shellArgument(connection.tokenEndpoint)} \\`,
    "    --header 'Content-Type: application/x-www-form-urlencoded' \\",
    "    --data-urlencode 'grant_type=client_credentials' \\",
    `    --data-urlencode ${shellArgument(`client_id=${connection.clientId}`)} \\`,
    "    --data-urlencode client_secret@- | \\",
    `  python3 -c ${shellArgument('import json,sys; print(json.load(sys.stdin)["access_token"])')})`,
  ].join("\n");
}
