import type {
  DashboardControlPlane,
  DashboardInvocationContext,
  DashboardOperations,
  DashboardWorkflowRuntime,
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

const workflowAction: DashboardWorkflowAction = "get-operational-metrics";

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

export function createDashboardOperations({
  controlPlane,
  probes = noopDashboardProbePublisher,
  runtime = defaultRuntime,
}: DashboardOperationDependencies): DashboardOperations {
  return {
    getOperationalMetrics: async (signal) => {
      const correlationId = runtime.createCorrelationId();
      const context: DashboardInvocationContext = {
        correlationId,
        ...(signal === undefined ? {} : { signal }),
      };
      const occurredAt = new Date().toISOString();

      probes.publish(
        workflowProbe(
          workflowAction,
          correlationId,
          "dashboard.workflow.started",
          occurredAt,
          "started",
        ),
      );

      try {
        const metrics = await controlPlane.getOperationalMetrics(context);
        probes.publish(
          workflowProbe(
            workflowAction,
            correlationId,
            "dashboard.workflow.completed",
            new Date().toISOString(),
            "succeeded",
          ),
        );
        return metrics;
      } catch (error) {
        probes.publish(
          workflowProbe(
            workflowAction,
            correlationId,
            "dashboard.workflow.completed",
            new Date().toISOString(),
            isCancelled(error) ? "cancelled" : "failed",
          ),
        );
        throw error;
      }
    },
  };
}
