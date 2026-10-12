import type { GatewayAccessListRequest } from "../application/gateway-types";

export const accessSearchDebounceMilliseconds = 300;

export function accessListQueryKey(
  gatewayId: string,
  request: GatewayAccessListRequest,
) {
  return [
    "gateways",
    "detail",
    gatewayId,
    "access",
    "list",
    request.page,
    request.size,
    request.search,
    request.role ?? "all",
    request.sort,
    request.order,
  ] as const;
}

export function accessListQueryRoot(gatewayId: string) {
  return ["gateways", "detail", gatewayId, "access", "list"] as const;
}

export function accessDirectoryQueryKey(gatewayId: string, search: string) {
  return [
    "gateways",
    "detail",
    gatewayId,
    "access",
    "directory",
    search,
  ] as const;
}
