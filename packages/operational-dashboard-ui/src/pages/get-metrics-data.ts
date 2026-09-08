import { useQuery } from "@tanstack/react-query";

import {
  operationalDashboardMetricsQueryKey,
  operationalDashboardRefreshMilliseconds,
} from "../dashboard/dashboard-data";
import { useDashboardUi } from "../dashboard-ui-provider";

export interface UseGetMetricsDataOptions {
  enabled?: boolean;
}

export function useGetMetricsData({
  enabled = true,
}: UseGetMetricsDataOptions = {}) {
  const { dashboard } = useDashboardUi();

  return useQuery({
    enabled,
    queryFn: async ({ signal }) => {
      return dashboard.getOperationalMetrics(signal);
    },
    queryKey: operationalDashboardMetricsQueryKey(),
    refetchInterval: operationalDashboardRefreshMilliseconds,
    staleTime: operationalDashboardRefreshMilliseconds,
  });
}
