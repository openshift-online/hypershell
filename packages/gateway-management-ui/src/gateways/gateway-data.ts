import type {
  GatewayListRequest,
  GatewayRecord,
} from "../application/gateway-types";
import type { GatewayConnection } from "./gateway-connections";

export const gatewayListQueryRoot = ["gateways", "list"] as const;
export const gatewayPlacementQueryRoot = ["gateways", "placements"] as const;

export function gatewayListQueryKey(request: GatewayListRequest) {
  return [
    ...gatewayListQueryRoot,
    request.page,
    request.size,
    request.search,
    request.sortField,
    request.sortDirection,
  ] as const;
}

export function gatewayQueryKey(gatewayId: string) {
  return ["gateways", "detail", gatewayId] as const;
}

export function gatewayPlacementQueryKey(search: string) {
  return [...gatewayPlacementQueryRoot, search.trim()] as const;
}

export function gatewayPlacementDetailQueryKey(clusterId: string) {
  return ["gateways", "placement", clusterId] as const;
}

type GatewayApiPayload = Omit<
  GatewayRecord,
  "externalDns" | "phase" | "status"
> &
  Partial<Pick<GatewayRecord, "externalDns" | "phase" | "status">>;

function gatewayEndpoint(gateway: GatewayApiPayload): string | undefined {
  const externalDns = gateway.externalDns?.trim() ?? "";
  if (!externalDns) {
    return undefined;
  }
  if (/^https?:\/\//iu.test(externalDns)) {
    return externalDns;
  }

  return `https://${externalDns}${externalDns.includes(":") ? "" : ":443"}`;
}

export function toGatewayConnection(
  gateway: GatewayApiPayload,
  hubClusterName = "Hub cluster",
): GatewayConnection {
  const status = [gateway.status, gateway.phase].find(
    (value) => value !== undefined && value.trim().length > 0,
  );

  return {
    ...(gateway.clusterId.trim()
      ? { clusterId: gateway.clusterId.trim() }
      : {}),
    clusterName: gateway.clusterId.trim() ? "" : hubClusterName,
    ...(gateway.consoleUrl ? { consoleUrl: gateway.consoleUrl } : {}),
    endpoint: gatewayEndpoint(gateway),
    id: gateway.id,
    name: gateway.name,
    ...(gateway.oidcAudience ? { oidcAudience: gateway.oidcAudience } : {}),
    ...(gateway.oidcClientId ? { oidcClientId: gateway.oidcClientId } : {}),
    ...(gateway.oidcIssuer ? { oidcIssuer: gateway.oidcIssuer } : {}),
    status: status ?? "Unknown",
  };
}
