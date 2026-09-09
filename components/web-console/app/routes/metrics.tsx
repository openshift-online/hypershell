import { GatewayMetricsDashboard } from "@openshift-online/hypershell-gateway-management-ui";

import { RequireDashboardAdmin } from "../features/dashboard/require-dashboard-admin";
import { createPageMeta } from "../lib/page-meta";

export const meta = createPageMeta(
  "app.nav.metrics",
  "app.page.metrics.description",
);

export default function MetricsRoute() {
  return (
    <RequireDashboardAdmin>
      <GatewayMetricsDashboard />
    </RequireDashboardAdmin>
  );
}
