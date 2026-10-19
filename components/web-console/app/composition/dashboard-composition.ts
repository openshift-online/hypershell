import { createDashboardOperations } from "@openshift-online/hypershell-operational-dashboard-ui";

import { createApiClient } from "../adapters/api/api.client";
import { createDashboardControlPlaneAdapter } from "../adapters/api/dashboard-control-plane";

export const dashboardOperations = createDashboardOperations({
  controlPlane: createDashboardControlPlaneAdapter((correlationId) =>
    createApiClient(correlationId),
  ),
});
