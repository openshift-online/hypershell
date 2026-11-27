import { CostDashboardPage } from "@openshift-online/hypershell-operational-dashboard-ui";
import { RequireAuthenticatedDashboard } from "../features/dashboard/require-dashboard-admin";
import { createPageMeta } from "../lib/page-meta";

export const meta = createPageMeta(
  "app.dashboard.cost.title",
  "app.dashboard.cost.description",
);

// WEB-COST-02: every signed-in user can open the cost dashboard. The page
// shows the platform view only to administrators; others see My cost.
export default function DashboardCostRoute() {
  return (
    <RequireAuthenticatedDashboard>
      <CostDashboardPage />
    </RequireAuthenticatedDashboard>
  );
}
