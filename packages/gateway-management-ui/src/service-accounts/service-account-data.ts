import type {
  OpenShellGatewayServiceAccountListRequest,
  OpenShellGatewayServiceAccountRecord,
} from "../application/gateway-types";

export const serviceAccountSearchDebounceMilliseconds = 300;
export const serviceAccountStatusPollMilliseconds = 5_000;

const transitionalStatuses = new Set([
  "degraded",
  "deleting",
  "provisioning",
  "revoking",
]);

export function serviceAccountNeedsPolling(
  account: Pick<OpenShellGatewayServiceAccountRecord, "status">,
): boolean {
  return transitionalStatuses.has(account.status);
}

export function serviceAccountListQueryKey(
  gatewayId: string,
  request: OpenShellGatewayServiceAccountListRequest,
) {
  return [
    "gateways",
    "detail",
    gatewayId,
    "service-accounts",
    "list",
    request.page,
    request.size,
    request.search,
    request.status ?? "all",
    request.sort,
    request.order,
  ] as const;
}

export function serviceAccountListQueryRoot(gatewayId: string) {
  return ["gateways", "detail", gatewayId, "service-accounts", "list"] as const;
}

export function serviceAccountQueryKey(
  gatewayId: string,
  serviceAccountId: string,
) {
  return [
    "gateways",
    "detail",
    gatewayId,
    "service-accounts",
    "detail",
    serviceAccountId,
  ] as const;
}
