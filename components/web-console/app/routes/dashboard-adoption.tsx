import { AdoptionDashboardPage } from "@openshift-online/hypershell-operational-dashboard-ui";

import { RequireDashboardAdmin } from "../features/dashboard/require-dashboard-admin";
import { createPageMeta } from "../lib/page-meta";

export const meta = createPageMeta(
  "app.nav.dashboardAdoption",
  "app.page.dashboardAdoption.description",
);

export default function DashboardAdoptionRoute() {
  return (
    <RequireDashboardAdmin>
      <AdoptionDashboardPage />
    </RequireDashboardAdmin>
  );
}
