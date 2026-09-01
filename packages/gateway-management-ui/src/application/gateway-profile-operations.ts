import type {
  GatewayProfileAction,
  GatewayProfileProbe,
  GatewayProfileProbeOutcome,
  GatewayProfileProbePublisher,
} from "./gateway-profile-probes";
import {
  GatewayProfileOperationError,
  type GatewayProfileControlPlane,
  type GatewayProfileInvocationContext,
  type GatewayProfileOperations,
  type GatewayWorkflowRuntime,
} from "./gateway-profile-types";

export interface GatewayProfileOperationDependencies {
  controlPlane: GatewayProfileControlPlane;
  probes: GatewayProfileProbePublisher;
  runtime: GatewayWorkflowRuntime;
}

function isCancelled(error: unknown): boolean {
  return (
    typeof error === "object" &&
    error !== null &&
    "name" in error &&
    error.name === "AbortError"
  );
}

function failureKind(error: unknown) {
  if (isCancelled(error)) {
    return "cancelled" as const;
  }
  return error instanceof GatewayProfileOperationError ? error.kind : "unknown";
}

function failureOutcome(error: unknown): GatewayProfileProbeOutcome {
  const kind = failureKind(error);
  if (kind === "cancelled") {
    return "cancelled";
  }
  if (kind === "conflict") {
    return "conflicted";
  }
  if (kind === "denied") {
    return "denied";
  }
  return "failed";
}

function probe(
  action: GatewayProfileAction,
  correlationId: string,
  traceId: string,
  failure: ReturnType<typeof failureKind> | null,
  name: GatewayProfileProbe["name"],
  occurredAt: string,
  outcome: GatewayProfileProbeOutcome,
  operationId?: string,
  parentInvocationId?: string,
): GatewayProfileProbe {
  return Object.freeze({
    context: Object.freeze({
      correlationId,
      traceId,
      ...(operationId === undefined ? {} : { operationId }),
      ...(parentInvocationId === undefined ? {} : { parentInvocationId }),
    }),
    fields: Object.freeze({ action, failureKind: failure, outcome }),
    name,
    occurredAt,
    schemaVersion: 1,
  });
}

export function createGatewayProfileOperations({
  controlPlane,
  probes,
  runtime,
}: GatewayProfileOperationDependencies): GatewayProfileOperations {
  async function execute<T>(
    action: GatewayProfileAction,
    signal: AbortSignal | undefined,
    task: (context: GatewayProfileInvocationContext) => Promise<T>,
  ): Promise<T> {
    const correlationId = runtime.createCorrelationId();
    const traceId = runtime.createTraceId();
    const context: GatewayProfileInvocationContext = {
      correlationId,
      ...(signal === undefined ? {} : { signal }),
    };
    probes.publish(
      probe(
        action,
        correlationId,
        traceId,
        null,
        "gatewayProfile.workflow.started",
        runtime.now(),
        "started",
      ),
    );
    probes.publish(
      probe(
        action,
        correlationId,
        traceId,
        null,
        "gatewayProfile.dependency.attempted",
        runtime.now(),
        "started",
        undefined,
        correlationId,
      ),
    );

    try {
      const result = await task(context);
      probes.publish(
        probe(
          action,
          correlationId,
          traceId,
          null,
          "gatewayProfile.dependency.completed",
          runtime.now(),
          "succeeded",
          undefined,
          correlationId,
        ),
      );
      probes.publish(
        probe(
          action,
          correlationId,
          traceId,
          null,
          "gatewayProfile.workflow.completed",
          runtime.now(),
          "succeeded",
        ),
      );
      return result;
    } catch (error) {
      const kind = failureKind(error);
      const outcome = failureOutcome(error);
      const operationId =
        error instanceof GatewayProfileOperationError
          ? error.operationId
          : undefined;
      probes.publish(
        probe(
          action,
          correlationId,
          traceId,
          kind,
          "gatewayProfile.dependency.completed",
          runtime.now(),
          outcome,
          operationId,
          correlationId,
        ),
      );
      probes.publish(
        probe(
          action,
          correlationId,
          traceId,
          kind,
          "gatewayProfile.workflow.completed",
          runtime.now(),
          outcome,
          operationId,
        ),
      );
      throw error;
    }
  }

  return {
    createGatewayProfile: (input, signal) =>
      execute("create", signal, (context) =>
        controlPlane.createGatewayProfile(input, context),
      ),
    getGatewayProfile: (gatewayProfileId, signal) =>
      execute("get", signal, (context) =>
        controlPlane.getGatewayProfile(gatewayProfileId, context),
      ),
    listGatewayProfiles: (request, signal) =>
      execute("list", signal, (context) =>
        controlPlane.listGatewayProfiles(request, context),
      ),
    removeGatewayProfile: (gatewayProfileId, signal) =>
      execute("remove", signal, (context) =>
        controlPlane.removeGatewayProfile(gatewayProfileId, context),
      ),
  };
}
