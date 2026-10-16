import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useRef } from "react";

import {
  adoptionDashboardMetricsQueryKey,
  operationalDashboardRefreshMilliseconds,
} from "../dashboard/dashboard-data";
import { mergeAdoptionDashboardMetrics } from "../dashboard/dashboard-metric-sources";
import { useDashboardUi } from "../dashboard-ui-provider";
import type {
  DashboardTimeRange,
  OperationalDashboardMetrics,
} from "../application/dashboard-types";

export interface UseGetAdoptionMetricsDataOptions {
  enabled?: boolean;
  timeRange?: DashboardTimeRange;
}

export function useGetAdoptionMetricsData({
  enabled = true,
  timeRange = 7,
}: UseGetAdoptionMetricsDataOptions = {}) {
  const { dashboard, isAdmin } = useDashboardUi();
  const mergedMetricsRef = useRef<OperationalDashboardMetrics | undefined>(
    undefined,
  );

  return useQuery({
    enabled,
    placeholderData: keepPreviousData,
    queryFn: async ({ signal }) => {
      const next = await dashboard.getAdoptionMetrics(signal, timeRange, {
        isAdmin,
      });
      const merged = mergeAdoptionDashboardMetrics(
        mergedMetricsRef.current,
        next,
      );
      mergedMetricsRef.current = merged;
      return merged;
    },
    queryKey: adoptionDashboardMetricsQueryKey(timeRange),
    refetchInterval: operationalDashboardRefreshMilliseconds,
    staleTime: operationalDashboardRefreshMilliseconds,
  });
}
