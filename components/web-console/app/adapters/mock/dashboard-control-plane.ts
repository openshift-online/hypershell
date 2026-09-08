import type {
  DashboardControlPlane,
  DashboardInvocationContext,
} from "@openshift-online/hypershell-operational-dashboard-ui";
import { mockOperationalDashboardMetrics } from "@openshift-online/hypershell-operational-dashboard-ui/fixtures";

export function createMockDashboardControlPlane(): DashboardControlPlane {
  return {
    async getOperationalMetrics(context: DashboardInvocationContext) {
      context.signal?.throwIfAborted();

      await new Promise((resolve) => setTimeout(resolve, 2000)); // This is just for demos for now

      return {
        ...mockOperationalDashboardMetrics,
        lastSuccessfulRefresh: new Date(),
      };
    },
  };
}
