import { createDashboardOperations } from "@openshift-online/hypershell-operational-dashboard-ui";

import { createDashboardControlPlaneAdapter } from "../adapters/api/dashboard-control-plane";

export const dashboardOperations = createDashboardOperations({
  controlPlane: createDashboardControlPlaneAdapter(),
});
