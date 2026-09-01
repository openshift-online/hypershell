import type { GatewayProfileListRequest } from "../application/gateway-profile-types";

export const gatewayProfileListQueryRoot = [
  "gateway-profiles",
  "list",
] as const;
export const gatewayProfileSearchQueryRoot = [
  "gateway-profiles",
  "search",
] as const;
export const gatewayProfileSearchDebounceMilliseconds = 250;

export function gatewayProfileListQueryKey(request: GatewayProfileListRequest) {
  return [
    ...gatewayProfileListQueryRoot,
    request.page,
    request.size,
    request.search,
    request.sortField,
    request.sortDirection,
  ] as const;
}

export function gatewayProfileQueryKey(gatewayProfileId: string) {
  return ["gateway-profiles", "detail", gatewayProfileId] as const;
}

export function gatewayProfileSearchQueryKey(search: string) {
  return [...gatewayProfileSearchQueryRoot, search.trim()] as const;
}
