import {
  ProvisioningDashboardPage,
  useGetMetricsData,
  type DashboardTimeRange,
} from "@openshift-online/hypershell-operational-dashboard-ui";
import { useState } from "react";
import { Navigate } from "react-router";

import { createPageMeta } from "../lib/page-meta";
import { hasDashboardAdminRole } from "../lib/session-roles";
import { useSession } from "../features/shell/use-session";

export const meta = createPageMeta(
  "app.dashboard.title",
  "app.dashboard.description",
);

export default function DashboardProvisioningRoute() {
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
    <ProvisioningDashboardPage
      isLoading={metricsQuery.isPending}
      isRefreshing={metricsQuery.isFetching}
      hasInitialLoadError={metricsQuery.isError}
      metrics={metricsQuery.data}
      onRefresh={() => {
        void metricsQuery.refetch();
      }}
      onTimeRangeChange={setTimeRange}
      timeRange={timeRange}
    />
  );
}
