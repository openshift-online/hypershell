import type {
  GatewayAction,
  GatewayProbe,
  GatewayProbeOutcome,
  GatewayProbePublisher,
} from "./gateway-probes";
import {
  GatewayOperationError,
  type GatewayControlPlane,
  type GatewayInvocationContext,
  type GatewayOperations,
  type GatewayWorkflowRuntime,
} from "./gateway-types";

export interface GatewayOperationDependencies {
  controlPlane: GatewayControlPlane;
  probes: GatewayProbePublisher;
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
  return error instanceof GatewayOperationError ? error.kind : "unknown";
}

function failureOutcome(error: unknown): GatewayProbeOutcome {
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
  action: GatewayAction,
  correlationId: string,
  traceId: string,
  failure: ReturnType<typeof failureKind> | null,
  name: GatewayProbe["name"],
  occurredAt: string,
  outcome: GatewayProbeOutcome,
  operationId?: string,
  parentInvocationId?: string,
): GatewayProbe {
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

export function createGatewayOperations({
  controlPlane,
  probes,
  runtime,
}: GatewayOperationDependencies): GatewayOperations {
  async function execute<T>(
    action: GatewayAction,
    signal: AbortSignal | undefined,
    task: (context: GatewayInvocationContext) => Promise<T>,
  ): Promise<T> {
    const correlationId = runtime.createCorrelationId();
    const traceId = runtime.createTraceId();
    const context: GatewayInvocationContext = {
      correlationId,
      ...(signal === undefined ? {} : { signal }),
    };
    probes.publish(
      probe(
        action,
        correlationId,
        traceId,
        null,
        "gateway.workflow.started",
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
        "gateway.dependency.attempted",
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
          "gateway.dependency.completed",
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
          "gateway.workflow.completed",
          runtime.now(),
          "succeeded",
        ),
      );
      return result;
    } catch (error) {
      const kind = failureKind(error);
      const outcome = failureOutcome(error);
      const operationId =
        error instanceof GatewayOperationError ? error.operationId : undefined;
      probes.publish(
        probe(
          action,
          correlationId,
          traceId,
          kind,
          "gateway.dependency.completed",
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
          "gateway.workflow.completed",
          runtime.now(),
          outcome,
          operationId,
        ),
      );
      throw error;
    }
  }

  return {
    findGatewayPlacements: (search, signal) =>
      execute("find-placements", signal, (context) =>
        controlPlane.findGatewayPlacements(search.trim(), context),
      ),
    getGatewayPlacement: (clusterId, signal) =>
      execute("get-placement", signal, (context) =>
        controlPlane.getGatewayPlacement(clusterId, context),
      ),
    getGatewayPlacements: (clusterIds, signal) =>
      execute("get-placements", signal, (context) =>
        controlPlane.getGatewayPlacements(clusterIds, context),
      ),
    getGateway: (gatewayId, signal) =>
      execute("get", signal, (context) =>
        controlPlane.getGateway(gatewayId, context),
      ),
    listGateways: (request, signal) =>
      execute("list", signal, (context) =>
        controlPlane.listGateways(request, context),
      ),
    provisionGateway: (input, signal) =>
      execute("provision", signal, (context) =>
        controlPlane.provisionGateway(input, context),
      ),
    removeGateway: (gatewayId, signal) =>
      execute("remove", signal, (context) =>
        controlPlane.removeGateway(gatewayId, context),
      ),
    renameGateway: (gatewayId, name, signal) =>
      execute("rename", signal, (context) =>
        controlPlane.renameGateway(gatewayId, name, context),
      ),
  };
}
