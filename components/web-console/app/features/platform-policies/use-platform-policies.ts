import { gatewayEffectivePoliciesQueryRoot } from "@openshift-online/hypershell-gateway-management-ui";
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";

import { platformPolicyFailureKind } from "../../application/platform-policies/platform-policy-operations";
import type {
  PlatformPolicy,
  PlatformPolicyFindingsRequest,
  PlatformPolicySettings,
} from "../../application/platform-policies/platform-policy-types";
import { platformPolicyOperations } from "../../composition/platform-policy-composition";
import {
  platformPolicyQueryKeys,
  platformPolicyQueryRoot,
  platformPolicyStaleMilliseconds,
} from "./platform-policy-query-keys";
import {
  writeFailureFor,
  type PlatformPolicyDialog,
  type PlatformPolicyWriteFailure,
} from "./platform-policy-summary";

/** Every platform policy (WEB-PADM-02). */
export function usePlatformPolicies() {
  return useQuery({
    queryFn: ({ signal }) => platformPolicyOperations.listPolicies(signal),
    queryKey: platformPolicyQueryKeys.list(),
    staleTime: platformPolicyStaleMilliseconds,
  });
}

/** The global enforcement toggle and fleet defaults (WEB-PADM-03). */
export function usePlatformPolicySettings() {
  return useQuery({
    queryFn: ({ signal }) => platformPolicyOperations.getSettings(signal),
    queryKey: platformPolicyQueryKeys.settings(),
    staleTime: platformPolicyStaleMilliseconds,
  });
}

/** One policy with its rollout (WEB-PADM-04). */
export function usePlatformPolicy(policyId: string) {
  return useQuery({
    enabled: policyId.length > 0,
    queryFn: ({ signal }) =>
      platformPolicyOperations.getPolicy(policyId, signal),
    queryKey: platformPolicyQueryKeys.detail(policyId),
    retry: (failureCount, error) =>
      platformPolicyFailureKind(error) !== "not-found" &&
      platformPolicyFailureKind(error) !== "denied" &&
      failureCount < 2,
    staleTime: platformPolicyStaleMilliseconds,
  });
}

/** One page of a policy's audit findings with filters (WEB-PADM-04). */
export function usePlatformPolicyFindings(
  policyId: string,
  request: PlatformPolicyFindingsRequest,
) {
  return useQuery({
    enabled: policyId.length > 0,
    placeholderData: keepPreviousData,
    queryFn: ({ signal }) =>
      platformPolicyOperations.listFindings(policyId, request, signal),
    queryKey: platformPolicyQueryKeys.findings(policyId, request),
    staleTime: platformPolicyStaleMilliseconds,
  });
}

/**
 * Policy writes from the list and detail views. Each write is attempted once:
 * a `403` is a denied operation and a `409` a conflict, never retried
 * (WEB-PADM-01.02). A successful write invalidates the policy list, details,
 * settings, findings, and every gateway's effective policies so the views
 * refresh without a reload (WEB-PADM-11).
 */
export function usePlatformPolicyWrites({
  globalMode,
  onSettled,
}: {
  globalMode: PlatformPolicySettings["mode"];
  onSettled?: (
    dialog: PlatformPolicyDialog,
    failure: PlatformPolicyWriteFailure | undefined,
  ) => void;
}) {
  const queryClient = useQueryClient();
  const mutation = useMutation({
    mutationFn: async (
      dialog: PlatformPolicyDialog,
    ): Promise<PlatformPolicy | PlatformPolicySettings | undefined> => {
      if (dialog.kind === "global-mode") {
        platformPolicyOperations.confirmModeChange(globalMode, dialog.to);
        return platformPolicyOperations.setGlobalMode(dialog.to);
      }
      if (dialog.kind === "policy-mode") {
        return platformPolicyOperations.updatePolicy(dialog.policyId, {
          enforcementMode: dialog.to,
        });
      }
      await platformPolicyOperations.deletePolicy(dialog.policyId);
      return undefined;
    },
    onError: (error, dialog) => {
      onSettled?.(dialog, writeFailureFor(platformPolicyFailureKind(error)));
    },
    onSuccess: async (_result, dialog) => {
      if (dialog.kind === "delete") {
        queryClient.removeQueries({
          queryKey: platformPolicyQueryKeys.detail(dialog.policyId),
        });
      }
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: platformPolicyQueryRoot }),
        queryClient.invalidateQueries({
          queryKey: gatewayEffectivePoliciesQueryRoot,
        }),
      ]);
      onSettled?.(dialog, undefined);
    },
    retry: false,
  });
  return {
    isWriting: mutation.isPending,
    write: (dialog: PlatformPolicyDialog) => {
      mutation.mutate(dialog);
    },
  };
}
