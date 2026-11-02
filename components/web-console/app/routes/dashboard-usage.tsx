import { UsageDashboardPage } from "@openshift-online/hypershell-operational-dashboard-ui";
import { RequireAuthenticatedDashboard } from "../features/dashboard/require-dashboard-admin";
import { createPageMeta } from "../lib/page-meta";

export const meta = createPageMeta(
  "app.dashboard.usage.title",
  "app.dashboard.usage.description",
);

export default function DashboardUsageRoute() {
  return (
    <RequireAuthenticatedDashboard>
      <UsageDashboardPage />
    </RequireAuthenticatedDashboard>
  );
}
