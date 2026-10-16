import {
  PlatformInventoryDashboardPage,
  useGetMetricsData,
  type DashboardTimeRange,
} from "@openshift-online/hypershell-operational-dashboard-ui";
import { useState } from "react";
import { Navigate } from "react-router";

import { useSession } from "../features/shell/use-session";
import { createPageMeta } from "../lib/page-meta";
import { hasDashboardAdminRole } from "../lib/session-roles";

export const meta = createPageMeta(
  "app.dashboard.inventory.title",
  "app.dashboard.inventory.description",
);

export default function DashboardInventoryRoute() {
  const { data: session, isLoading } = useSession();
  const [timeRange, setTimeRange] = useState<DashboardTimeRange>(7);
  const metricsQuery = useGetMetricsData({ timeRange });

  if (
    !isLoading &&
    session?.authenticated &&
    !hasDashboardAdminRole(session.roles)
  ) {
    return <Navigate replace to="/dashboard" />;
  }

  if (!isLoading && session?.authEnabled && !session.authenticated) {
    return <Navigate replace to="/dashboard" />;
  }

  return (
    <PlatformInventoryDashboardPage
      isLoading={metricsQuery.isPending}
      isRefreshing={metricsQuery.isFetching}
      metrics={metricsQuery.data}
      onRefresh={() => {
        void metricsQuery.refetch();
      }}
      onTimeRangeChange={setTimeRange}
      timeRange={timeRange}
    />
  );
}
