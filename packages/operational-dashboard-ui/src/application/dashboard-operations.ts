import type {
  DashboardOperationalMetricsOptions,
  DashboardControlPlane,
  DashboardFailedSourceId,
  DashboardInvocationContext,
  DashboardOperations,
  DashboardTimeRange,
  DashboardWorkflowRuntime,
  OperationalDashboardMetrics,
} from "./dashboard-types";
import type {
  DashboardProbe,
  DashboardProbePublisher,
  DashboardWorkflowAction,
} from "./dashboard-probes";
import { noopDashboardProbePublisher } from "./dashboard-probes";

export interface DashboardOperationDependencies {
  controlPlane: DashboardControlPlane;
  probes?: DashboardProbePublisher;
  runtime?: DashboardWorkflowRuntime;
}

const defaultRuntime: DashboardWorkflowRuntime = {
  createCorrelationId: () => crypto.randomUUID(),
};

function isCancelled(error: unknown): boolean {
  return (
    typeof error === "object" &&
    error !== null &&
    "name" in error &&
    error.name === "AbortError"
  );
}

function workflowProbe(
  action: DashboardWorkflowAction,
  correlationId: string,
  name: DashboardProbe["name"],
  occurredAt: string,
  outcome: DashboardProbe["fields"]["outcome"],
): DashboardProbe {
  return Object.freeze({
    context: Object.freeze({ correlationId }),
    fields: Object.freeze({ action, outcome }),
    name,
    occurredAt,
    schemaVersion: 1,
  });
}

function partialFailureProbe(
  action: DashboardWorkflowAction,
  correlationId: string,
  failedSources: readonly DashboardFailedSourceId[],
): DashboardProbe {
  return Object.freeze({
    context: Object.freeze({ correlationId }),
    fields: Object.freeze({
      action,
      failedSources,
      outcome: "failed",
    }),
    name: "dashboard.metrics.partial-failure",
    occurredAt: new Date().toISOString(),
    schemaVersion: 1,
  });
}

async function loadMetricsWithProbes(
  action: DashboardWorkflowAction,
  load: (
    context: DashboardInvocationContext,
  ) => Promise<OperationalDashboardMetrics>,
  probes: DashboardProbePublisher,
  runtime: DashboardWorkflowRuntime,
  signal?: AbortSignal,
  timeRange: DashboardTimeRange = 7,
  options: DashboardOperationalMetricsOptions = {},
): Promise<OperationalDashboardMetrics> {
  const correlationId = runtime.createCorrelationId();
  const context: DashboardInvocationContext = {
    correlationId,
    ...(options.isAdmin === undefined ? {} : { isAdmin: options.isAdmin }),
    timeRange,
    ...(signal === undefined ? {} : { signal }),
  };
  const occurredAt = new Date().toISOString();

  probes.publish(
    workflowProbe(
      action,
      correlationId,
      "dashboard.workflow.started",
      occurredAt,
      "started",
    ),
  );

  try {
    const metrics = await load(context);
    probes.publish(
      workflowProbe(
        action,
        correlationId,
        "dashboard.workflow.completed",
        new Date().toISOString(),
        "succeeded",
      ),
    );
    if (
      metrics.failedSources !== undefined &&
      metrics.failedSources.length > 0
    ) {
      probes.publish(
        partialFailureProbe(action, correlationId, metrics.failedSources),
      );
    }
    return metrics;
  } catch (error) {
    probes.publish(
      workflowProbe(
        action,
        correlationId,
        "dashboard.workflow.completed",
        new Date().toISOString(),
        isCancelled(error) ? "cancelled" : "failed",
      ),
    );
    throw error;
  }
}

export function createDashboardOperations({
  controlPlane,
  probes = noopDashboardProbePublisher,
  runtime = defaultRuntime,
}: DashboardOperationDependencies): DashboardOperations {
  return {
    getOperationalMetrics: (signal, timeRange = 7, options = {}) =>
      loadMetricsWithProbes(
        "get-operational-metrics",
        (context) => controlPlane.getOperationalMetrics(context),
        probes,
        runtime,
        signal,
        timeRange,
        options,
      ),
    getReliabilityMetrics: (signal) =>
      loadMetricsWithProbes(
        "get-reliability-metrics",
        (context) => controlPlane.getReliabilityMetrics(context),
        probes,
        runtime,
        signal,
      ),
  };
}
