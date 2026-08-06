import { previewGateway, type GatewayConnection } from "./gateway-connections";
import type { GatewayRecord } from "./gateway-types";

export function gatewayQueryKey(gatewayId: string) {
  return ["gateways", "detail", gatewayId] as const;
}

type GatewayApiPayload = Omit<
  GatewayRecord,
  "externalDns" | "phase" | "status"
> &
  Partial<Pick<GatewayRecord, "externalDns" | "phase" | "status">>;

function gatewayEndpoint(gateway: GatewayApiPayload): string {
  const externalDns = gateway.externalDns?.trim() ?? "";
  if (!externalDns) {
    return previewGateway.endpoint;
  }
  if (/^https?:\/\//iu.test(externalDns)) {
    return externalDns;
  }

  return `https://${externalDns}${externalDns.includes(":") ? "" : ":443"}`;
}

export function toGatewayConnection(
  gateway: GatewayApiPayload,
): GatewayConnection {
  const status = [gateway.status, gateway.phase].find(
    (value) => value !== undefined && value.trim().length > 0,
  );

  return {
    clusterName: gateway.clusterId.trim() || "Local cluster",
    consoleUrl: previewGateway.consoleUrl,
    endpoint: gatewayEndpoint(gateway),
    id: gateway.id,
    name: gateway.name,
    oidcAudience: previewGateway.oidcAudience,
    oidcClientId: previewGateway.oidcClientId,
    oidcIssuer: previewGateway.oidcIssuer,
    status: status ?? "Unknown",
  };
}
