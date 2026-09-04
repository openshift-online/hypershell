import { OperationalDashboardPage } from "@openshift-online/hypershell-operational-dashboard-ui";

import { RequireDashboardAdmin } from "../features/dashboard/require-dashboard-admin";
import { createPageMeta } from "../lib/page-meta";

export const meta = createPageMeta(
  "app.nav.dashboard",
  "app.page.dashboard.description",
);

export default function DashboardRoute() {
  return (
    <RequireDashboardAdmin>
      <OperationalDashboardPage />
    </RequireDashboardAdmin>
  );
}
