import { ReliabilityDashboardPage } from "@openshift-online/hypershell-operational-dashboard-ui";

import { RequireDashboardAdmin } from "../features/dashboard/require-dashboard-admin";
import { createPageMeta } from "../lib/page-meta";

export const meta = createPageMeta(
  "app.nav.dashboardReliability",
  "app.page.dashboardReliability.description",
);

export default function DashboardReliabilityRoute() {
  return (
    <RequireDashboardAdmin>
      <ReliabilityDashboardPage />
    </RequireDashboardAdmin>
  );
}
