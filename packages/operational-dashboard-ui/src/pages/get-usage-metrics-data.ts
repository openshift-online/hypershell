import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useRef } from "react";

import type {
  DashboardTimeRange,
  UsageDashboardMetrics,
} from "../application/dashboard-types";
import {
  usageDashboardMetricsQueryKey,
  usageDashboardRefreshMilliseconds,
} from "../dashboard/dashboard-data";
import { mergeUsageDashboardMetrics } from "../dashboard/dashboard-metric-sources";
import { useDashboardUi } from "../dashboard-ui-provider";

export interface UseGetUsageMetricsDataOptions {
  enabled?: boolean;
  timeRange?: DashboardTimeRange;
}

export function useGetUsageMetricsData({
  enabled = true,
  timeRange = 7,
}: UseGetUsageMetricsDataOptions = {}) {
  const { dashboard, isAdmin } = useDashboardUi();
  const successfulMetricsByTimeRangeRef = useRef<
    Partial<Record<DashboardTimeRange, UsageDashboardMetrics>>
  >({});

  return useQuery({
    enabled,
    placeholderData: keepPreviousData,
    queryFn: async ({ signal }) => {
      const next = await dashboard.getUsageMetrics(signal, timeRange, {
        isAdmin,
      });
      const merged = mergeUsageDashboardMetrics(
        successfulMetricsByTimeRangeRef.current[timeRange],
        next,
      );

      if (merged.metrics.length === 0) {
        throw new Error("Usage dashboard metrics unavailable");
      }

      successfulMetricsByTimeRangeRef.current[timeRange] = merged;
      return merged;
    },
    queryKey: usageDashboardMetricsQueryKey(timeRange),
    refetchInterval: usageDashboardRefreshMilliseconds,
    staleTime: usageDashboardRefreshMilliseconds,
  });
}
