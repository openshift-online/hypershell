import type { PlatformPolicyFindingsRequest } from "../../application/platform-policies/platform-policy-types";

/** Query keys for the platform policy admin panel (WEB-PADM-11). */
export const platformPolicyQueryRoot = ["platform-policies"] as const;

export const platformPolicyQueryKeys = {
  access: () => [...platformPolicyQueryRoot, "access"] as const,
  detail: (policyId: string) =>
    [...platformPolicyQueryRoot, "detail", policyId] as const,
  evaluation: (fingerprint: string) =>
    [...platformPolicyQueryRoot, "evaluation", fingerprint] as const,
  findings: (policyId: string, request: PlatformPolicyFindingsRequest) =>
    [
      ...platformPolicyQueryRoot,
      "findings",
      policyId,
      request.gatewayId ?? "",
      request.outcome ?? "",
      request.page,
      request.size,
    ] as const,
  list: () => [...platformPolicyQueryRoot, "list"] as const,
  settings: () => [...platformPolicyQueryRoot, "settings"] as const,
};

/** Policies change only on administrator writes; refetch on focus. */
export const platformPolicyStaleMilliseconds = 30_000;
