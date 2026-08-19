import type {
  GatewayListRequest,
  GatewayRecord,
} from "../application/gateway-types";
import { normalizeGatewayPlacementClusterIds } from "../application/gateway-placement";
import type { GatewayConnection } from "./gateway-connections";

export const gatewayListQueryRoot = ["gateways", "list"] as const;
export const gatewayPlacementQueryRoot = ["gateways", "placements"] as const;
export const gatewayPlacementStaleMilliseconds = 60_000;
export const gatewaySearchDebounceMilliseconds = 250;
export const gatewayStatusPollMilliseconds = 5_000;

const gatewayPollingStates = new Set([
  "pending",
  "provisioning",
  "reconciling",
  "updating",
  "degraded",
]);
const gatewayFailedLifecycleStates = new Set(["error", "failed"]);

export function gatewayNeedsStatusPolling(
  gateway: Pick<GatewayRecord, "phase" | "status">,
): boolean {
  const states = [gateway.phase, gateway.status]
    .map((value) => value?.trim().toLocaleLowerCase() ?? "")
    .filter(Boolean);

  return (
    states.length === 0 ||
    states.some((value) => gatewayPollingStates.has(value))
  );
}

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
  return [...gatewayPlacementQueryRoot, "search", search.trim()] as const;
}

export function gatewayPlacementDetailQueryKey(clusterId: string) {
  return [...gatewayPlacementQueryRoot, "detail", clusterId] as const;
}

export function gatewayPlacementBatchQueryKey(clusterIds: readonly string[]) {
  const normalizedClusterIds = normalizeGatewayPlacementClusterIds(clusterIds);
  return [
    ...gatewayPlacementQueryRoot,
    "batch",
    ...normalizedClusterIds,
  ] as const;
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
  hubClusterName: string,
): GatewayConnection {
  const clusterId = gateway.clusterId.trim();
  const phase = gateway.phase?.trim() ?? "";
  const healthStatus = gateway.status?.trim() ?? "";
  const normalizedPhase = phase.toLocaleLowerCase();
  const status =
    phase &&
    (gatewayPollingStates.has(normalizedPhase) ||
      gatewayFailedLifecycleStates.has(normalizedPhase))
      ? phase
      : healthStatus || phase;

  return {
    ...(clusterId ? { clusterId } : {}),
    clusterName: clusterId ? "" : hubClusterName,
    ...(gateway.consoleUrl ? { consoleUrl: gateway.consoleUrl } : {}),
    ...(gateway.createdAt ? { createdAt: gateway.createdAt } : {}),
    endpoint: gatewayEndpoint(gateway),
    id: gateway.id,
    name: gateway.name,
    ...(gateway.oidcAudience ? { oidcAudience: gateway.oidcAudience } : {}),
    ...(gateway.oidcClientId ? { oidcClientId: gateway.oidcClientId } : {}),
    ...(gateway.oidcIssuer ? { oidcIssuer: gateway.oidcIssuer } : {}),
    ...(phase ? { phase } : {}),
    status: status || "Unknown",
  };
}
