import type {
  DashboardControlPlane,
  DashboardInvocationContext,
  OperationalDashboardMetrics,
} from "@openshift-online/hypershell-operational-dashboard-ui";

import {
  fetchGatewayMetrics,
  gatewayPhases,
} from "@openshift-online/hypershell-gateway-management-ui";

export function createDashboardControlPlaneAdapter(): DashboardControlPlane {
  return {
    async getOperationalMetrics(
      context: DashboardInvocationContext,
    ): Promise<OperationalDashboardMetrics> {
      const counts = await fetchGatewayMetrics(context.signal);

      return {
        lastSuccessfulRefresh: new Date(),
        metrics: gatewayPhases.map((phase) => ({
          id: `gateway-phase-${phase.toLowerCase()}`,
          value: String(counts[phase]),
        })),
      };
    },
  };
}
