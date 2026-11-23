import {
  freezeProbe,
  type PlatformPolicyAction,
  type PlatformPolicyLoadedProbe,
  type PlatformPolicyModeChangeConfirmedProbe,
  type PlatformPolicyOutcome,
  type PlatformPolicyProbePublisher,
  type PlatformPolicySaveDeniedProbe,
  type PlatformPolicySavedProbe,
  type PlatformPolicyValidatedProbe,
  type PlatformPolicyWorkflowProbe,
  type PlatformPolicyWriteAction,
} from "./platform-policy-probes";
import {
  PlatformPolicyOperationError,
  type PlatformPolicyControlPlane,
  type PlatformPolicyFailureKind,
  type PlatformPolicyInvocationContext,
  type PlatformPolicyOperations,
  type PlatformPolicyRuntime,
} from "./platform-policy-types";

export interface PlatformPolicyOperationDependencies {
  controlPlane: PlatformPolicyControlPlane;
  probes: PlatformPolicyProbePublisher;
  runtime: PlatformPolicyRuntime;
}

function isCancelled(error: unknown): boolean {
  return (
    typeof error === "object" &&
    error !== null &&
    "name" in error &&
    error.name === "AbortError"
  );
}

export function platformPolicyFailureKind(
  error: unknown,
): PlatformPolicyFailureKind {
  if (isCancelled(error)) {
    return "cancelled";
  }
  return error instanceof PlatformPolicyOperationError ? error.kind : "unknown";
}

function failureOutcome(
  kind: PlatformPolicyFailureKind,
): PlatformPolicyOutcome {
  if (kind === "cancelled" || kind === "conflicted" || kind === "denied") {
    return kind;
  }
  return "failed";
}

interface Invocation {
  context: { correlationId: string; traceId: string };
  emit<
    TProbe extends
      | PlatformPolicyLoadedProbe
      | PlatformPolicySavedProbe
      | PlatformPolicySaveDeniedProbe
      | PlatformPolicyValidatedProbe,
  >(
    name: TProbe["name"],
    fields: TProbe["fields"],
  ): void;
}

/**
 * Creates the admin panel's driving port. Every use case publishes one started
 * and exactly one terminal workflow probe, a dependency attempt/outcome pair,
 * and its domain fact (UIS-OBS-01), with the invocation's correlation id on the
 * outbound request (UIS-OBS-08).
 */
export function createPlatformPolicyOperations({
  controlPlane,
  probes,
  runtime,
}: PlatformPolicyOperationDependencies): PlatformPolicyOperations {
  async function execute<T>(
    action: PlatformPolicyAction,
    signal: AbortSignal | undefined,
    task: (context: PlatformPolicyInvocationContext) => Promise<T>,
    onSuccess?: (result: T, invocation: Invocation) => void,
    writeAction?: PlatformPolicyWriteAction,
  ): Promise<T> {
    const correlationId = runtime.createCorrelationId();
    const traceId = runtime.createTraceId();
    const base = { correlationId, traceId };
    const workflow = (
      name: PlatformPolicyWorkflowProbe["name"],
      outcome: PlatformPolicyOutcome,
      failureKind: PlatformPolicyFailureKind | null,
      operationId?: string,
    ) => {
      const dependency =
        name === "platform-policy.dependency.attempted" ||
        name === "platform-policy.dependency.completed";
      probes.publish(
        freezeProbe<PlatformPolicyWorkflowProbe>(
          name,
          {
            ...base,
            ...(operationId === undefined ? {} : { operationId }),
            ...(dependency ? { parentInvocationId: correlationId } : {}),
          },
          runtime.now(),
          { action, failureKind, outcome },
        ),
      );
    };
    const invocation: Invocation = {
      context: base,
      emit(name, fields) {
        probes.publish(freezeProbe(name, base, runtime.now(), fields));
      },
    };
    workflow("platform-policy.workflow.started", "started", null);
    workflow("platform-policy.dependency.attempted", "started", null);
    try {
      const result = await task({
        correlationId,
        ...(signal === undefined ? {} : { signal }),
      });
      workflow("platform-policy.dependency.completed", "succeeded", null);
      onSuccess?.(result, invocation);
      if (writeAction) {
        invocation.emit<PlatformPolicySavedProbe>("platform-policy.saved", {
          action: writeAction,
        });
      }
      workflow("platform-policy.workflow.completed", "succeeded", null);
      return result;
    } catch (error) {
      const kind = platformPolicyFailureKind(error);
      const outcome = failureOutcome(kind);
      const operationId =
        error instanceof PlatformPolicyOperationError
          ? error.operationId
          : undefined;
      workflow(
        "platform-policy.dependency.completed",
        outcome,
        kind,
        operationId,
      );
      if (writeAction && (kind === "denied" || kind === "conflicted")) {
        invocation.emit<PlatformPolicySaveDeniedProbe>(
          "platform-policy.save.denied",
          { action: writeAction, failureKind: kind },
        );
      }
      workflow(
        "platform-policy.workflow.completed",
        outcome,
        kind,
        operationId,
      );
      throw error;
    }
  }

  const loaded =
    (action: PlatformPolicyLoadedProbe["fields"]["action"], count: number) =>
    (_result: unknown, invocation: Invocation) => {
      invocation.emit<PlatformPolicyLoadedProbe>("platform-policy.loaded", {
        action,
        itemCount: count,
      });
    };

  return {
    async checkAccess(signal) {
      try {
        await execute("check-access", signal, (context) =>
          controlPlane.getSettings(context),
        );
        return "authorized";
      } catch (error) {
        if (platformPolicyFailureKind(error) === "denied") {
          return "denied";
        }
        throw error;
      }
    },
    confirmModeChange(from, to) {
      probes.publish(
        freezeProbe<PlatformPolicyModeChangeConfirmedProbe>(
          "platform-policy.mode-change.confirmed",
          {
            correlationId: runtime.createCorrelationId(),
            traceId: runtime.createTraceId(),
          },
          runtime.now(),
          { from, to },
        ),
      );
    },
    createPolicy: (draft, signal) =>
      execute(
        "create",
        signal,
        (context) => controlPlane.createPolicy(draft, context),
        undefined,
        "create",
      ),
    deletePolicy: (policyId, signal) =>
      execute(
        "delete",
        signal,
        (context) => controlPlane.deletePolicy(policyId, context),
        undefined,
        "delete",
      ),
    evaluatePolicy: (input, signal) =>
      execute(
        "evaluate",
        signal,
        (context) => controlPlane.evaluatePolicy(input, context),
        (result, invocation) => {
          invocation.emit<PlatformPolicyValidatedProbe>(
            "platform-policy.validated",
            {
              diagnosticCount: result.diagnostics.length,
              forbidsDefaults: result.forbidsDefaults,
              requestAllowed: result.requestAllowed,
              valid: result.valid,
            },
          );
        },
      ),
    getPolicy: (policyId, signal) =>
      execute(
        "get",
        signal,
        (context) => controlPlane.getPolicy(policyId, context),
        loaded("get", 1),
      ),
    getSettings: (signal) =>
      execute(
        "get-settings",
        signal,
        (context) => controlPlane.getSettings(context),
        loaded("get-settings", 1),
      ),
    listFindings: (policyId, request, signal) =>
      execute(
        "list-findings",
        signal,
        (context) => controlPlane.listFindings(policyId, request, context),
        (result, invocation) => {
          loaded("list-findings", result.items.length)(result, invocation);
        },
      ),
    listPolicies: (signal) =>
      execute(
        "list",
        signal,
        (context) => controlPlane.listPolicies(context),
        (result, invocation) => {
          loaded("list", result.length)(result, invocation);
        },
      ),
    setGlobalMode: (mode, signal) =>
      execute(
        "set-global-mode",
        signal,
        (context) => controlPlane.updateSettings(mode, context),
        undefined,
        "set-global-mode",
      ),
    updatePolicy: (policyId, update, signal) =>
      execute(
        "update",
        signal,
        (context) => controlPlane.updatePolicy(policyId, update, context),
        undefined,
        "update",
      ),
  };
}
