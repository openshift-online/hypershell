/** Query-key root for every gateway's effective policies (WEB-GLIS-26). */
export const gatewayEffectivePoliciesQueryRoot = [
  "gateways",
  "effective-policies",
] as const;

export function gatewayEffectivePoliciesQueryKey(gatewayId: string) {
  return [...gatewayEffectivePoliciesQueryRoot, gatewayId] as const;
}

/**
 * Policies change only when an administrator saves or a rollout advances, so
 * the tab treats data as fresh for 30 seconds and refetches on focus or
 * through the refresh button (WEB-GLIS-21, WEB-GLIS-24).
 */
export const gatewayEffectivePoliciesStaleMilliseconds = 30_000;
