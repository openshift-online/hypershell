import { createDashboardControlPlaneAdapter } from "../adapters/api/dashboard-control-plane";
import type { DashboardControlPlane } from "../adapters/api/dashboard-control-plane";

export interface DashboardOperations {
  controlPlane: DashboardControlPlane;
}

export const dashboardOperations: DashboardOperations = {
  controlPlane: createDashboardControlPlaneAdapter(),
};
