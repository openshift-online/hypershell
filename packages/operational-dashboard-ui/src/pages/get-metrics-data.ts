import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useRef } from "react";

import {
  operationalDashboardMetricsQueryKey,
  operationalDashboardRefreshMilliseconds,
} from "../dashboard/dashboard-data";
import { mergeOperationalDashboardMetrics } from "../dashboard/dashboard-metric-sources";
import { useDashboardUi } from "../dashboard-ui-provider";
import type {
  DashboardTimeRange,
  OperationalDashboardMetrics,
} from "../application/dashboard-types";

export interface UseGetMetricsDataOptions {
  enabled?: boolean;
  timeRange?: DashboardTimeRange;
}

export function useGetMetricsData({
  enabled = true,
  timeRange = 7,
}: UseGetMetricsDataOptions = {}) {
  const { dashboard, isAdmin } = useDashboardUi();
  const mergedMetricsRef = useRef<OperationalDashboardMetrics | undefined>(
    undefined,
  );

  return useQuery({
    enabled,
    placeholderData: keepPreviousData,
    queryFn: async ({ signal }) => {
      const next = await dashboard.getOperationalMetrics(signal, timeRange, {
        isAdmin,
      });
      const merged = mergeOperationalDashboardMetrics(
        mergedMetricsRef.current,
        next,
      );
      mergedMetricsRef.current = merged;
      return merged;
    },
    queryKey: [...operationalDashboardMetricsQueryKey(), timeRange],
    refetchInterval: operationalDashboardRefreshMilliseconds,
    staleTime: operationalDashboardRefreshMilliseconds,
  });
}
