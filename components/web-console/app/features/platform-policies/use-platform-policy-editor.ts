import { gatewayEffectivePoliciesQueryRoot } from "@openshift-online/hypershell-gateway-management-ui";
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useEffect, useState } from "react";

import { platformPolicyFailureKind } from "../../application/platform-policies/platform-policy-operations";
import {
  PlatformPolicyOperationError,
  type PlatformPolicy,
  type PlatformPolicyEvaluationInput,
  type PlatformPolicyOperations,
} from "../../application/platform-policies/platform-policy-types";
import type { PlatformPolicySaveFailure } from "./platform-policy-form-page";
import {
  emptyPlatformPolicyFormValues,
  emptyPlatformPolicyTestValues,
  platformPolicyFormValues,
  toPlatformPolicyDraft,
  toPlatformPolicyTestRequest,
  type PlatformPolicyFormValues,
  type PlatformPolicyTestValues,
} from "./platform-policy-form-model";
import {
  platformPolicyQueryKeys,
  platformPolicyQueryRoot,
  platformPolicyStaleMilliseconds,
} from "./platform-policy-query-keys";

/** Live validation waits for a pause in typing before calling /evaluate. */
export const platformPolicyValidationDelayMilliseconds = 400;

function useDebounced<Value>(value: Value, delay: number): Value {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebounced(value);
    }, delay);
    return () => {
      clearTimeout(timer);
    };
  }, [value, delay]);
  return debounced;
}

export function toPlatformPolicySaveFailure(
  error: unknown,
): PlatformPolicySaveFailure {
  if (error instanceof PlatformPolicyOperationError) {
    return {
      kind: error.kind,
      ...(error.reason === undefined ? {} : { reason: error.reason }),
    };
  }
  return { kind: platformPolicyFailureKind(error) };
}

export interface PlatformPolicyEditorOptions {
  onSaved: (policy: PlatformPolicy) => void;
  operations: PlatformPolicyOperations;
  /** The policy to edit; absent when creating. */
  policyId?: string;
}

/**
 * Container hook for the platform policy editor (WEB-PADM-05..11): the draft,
 * the debounced live `/evaluate` dry run (same semantics as enforcement), the
 * edit-mode policy load, and the create/update write. A `403` write is a
 * denied operation and is never retried; a successful write invalidates the
 * admin list/detail/settings queries and every gateway's effective policies.
 */
export function usePlatformPolicyEditor({
  onSaved,
  operations,
  policyId,
}: PlatformPolicyEditorOptions) {
  const queryClient = useQueryClient();
  const isEditing = policyId !== undefined;
  const policyQuery = useQuery({
    enabled: isEditing,
    queryFn: ({ signal }) => operations.getPolicy(policyId ?? "", signal),
    queryKey: platformPolicyQueryKeys.detail(policyId ?? ""),
    retry: false,
    staleTime: platformPolicyStaleMilliseconds,
  });
  const settingsQuery = useQuery({
    queryFn: ({ signal }) => operations.getSettings(signal),
    queryKey: platformPolicyQueryKeys.settings(),
    retry: false,
    staleTime: platformPolicyStaleMilliseconds,
  });
  const [editedValues, setEditedValues] = useState<PlatformPolicyFormValues>();
  const [testValues, setTestValues] = useState<PlatformPolicyTestValues>(
    emptyPlatformPolicyTestValues,
  );

  const values =
    editedValues ??
    (policyQuery.data
      ? platformPolicyFormValues(policyQuery.data)
      : emptyPlatformPolicyFormValues);

  const { request } = toPlatformPolicyTestRequest(testValues);
  const input: PlatformPolicyEvaluationInput = {
    cedar: values.cedar,
    documentationUrl: values.documentationUrl.trim(),
    name: values.name.trim(),
    request,
    userMessage: values.userMessage.trim(),
  };
  const fingerprint = JSON.stringify(input);
  const debouncedFingerprint = useDebounced(
    fingerprint,
    platformPolicyValidationDelayMilliseconds,
  );
  const hasCedar = values.cedar.trim().length > 0;
  const debouncedInput = JSON.parse(
    debouncedFingerprint,
  ) as PlatformPolicyEvaluationInput;
  const evaluationQuery = useQuery({
    enabled: hasCedar && debouncedInput.cedar.trim().length > 0,
    placeholderData: keepPreviousData,
    queryFn: ({ signal }) => operations.evaluatePolicy(debouncedInput, signal),
    queryKey: platformPolicyQueryKeys.evaluation(debouncedFingerprint),
    retry: false,
    staleTime: platformPolicyStaleMilliseconds,
  });

  const save = useMutation({
    mutationFn: async (draftValues: PlatformPolicyFormValues) => {
      const draft = toPlatformPolicyDraft(draftValues);
      if (policyId !== undefined) {
        // The name is the stable identifier and cannot change on update.
        return operations.updatePolicy(policyId, {
          cedar: draft.cedar,
          description: draft.description,
          documentationUrl: draft.documentationUrl,
          target: draft.target,
          userMessage: draft.userMessage,
        });
      }
      return operations.createPolicy(draft);
    },
    onSuccess: async (policy) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: platformPolicyQueryRoot }),
        queryClient.invalidateQueries({
          queryKey: gatewayEffectivePoliciesQueryRoot,
        }),
      ]);
      onSaved(policy);
    },
    // A denied write is a decision, not a transient failure (WEB-PADM-01.02).
    retry: false,
  });

  return {
    load: isEditing
      ? {
          isError: policyQuery.isError && !policyQuery.data,
          isPending: policyQuery.isPending,
          refetch: () => {
            void policyQuery.refetch();
          },
        }
      : undefined,
    view: {
      evaluation: hasCedar ? evaluationQuery.data : undefined,
      evaluationFailed: hasCedar && evaluationQuery.isError,
      isEditing,
      isEvaluating:
        hasCedar &&
        (fingerprint !== debouncedFingerprint || evaluationQuery.isFetching),
      isSaving: save.isPending,
      onSave: () => {
        save.mutate(values);
      },
      onTestValuesChange: setTestValues,
      onValuesChange: (next: PlatformPolicyFormValues) => {
        if (save.isError) {
          save.reset();
        }
        setEditedValues(next);
      },
      ...(save.isError
        ? { saveFailure: toPlatformPolicySaveFailure(save.error) }
        : {}),
      ...(settingsQuery.data?.schemaVersion
        ? { schemaVersion: settingsQuery.data.schemaVersion }
        : {}),
      testValues,
      values,
    },
  };
}
