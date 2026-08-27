import {
  fetchGatewayMetrics,
  gatewayPhases,
  type GatewayPhaseCounts,
} from "@openshift-online/hypershell-gateway-management-ui";

export interface OperationalDashboardMetric {
  id: string;
  value: string;
}

export interface OperationalDashboardMetrics {
  lastSuccessfulRefresh: Date;
  metrics: OperationalDashboardMetric[];
}

export interface DashboardControlPlane {
  getOperationalMetrics(signal?: AbortSignal): Promise<OperationalDashboardMetrics>;
}

export function createDashboardControlPlaneAdapter(): DashboardControlPlane {
  return {
    async getOperationalMetrics(signal?: AbortSignal): Promise<OperationalDashboardMetrics> {
      const counts: GatewayPhaseCounts = await fetchGatewayMetrics(signal);

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
