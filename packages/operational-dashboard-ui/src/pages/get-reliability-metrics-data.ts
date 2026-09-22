import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useRef } from "react";

import {
  reliabilityDashboardMetricsQueryKey,
  reliabilityDashboardRefreshMilliseconds,
} from "../dashboard/dashboard-data";
import { mergeReliabilityDashboardMetrics } from "../dashboard/dashboard-metric-sources";
import { useDashboardUi } from "../dashboard-ui-provider";
import type { OperationalDashboardMetrics } from "../application/dashboard-types";

export interface UseGetReliabilityMetricsDataOptions {
  enabled?: boolean;
}

export function useGetReliabilityMetricsData({
  enabled = true,
}: UseGetReliabilityMetricsDataOptions = {}) {
  const { dashboard } = useDashboardUi();
  const mergedMetricsRef = useRef<OperationalDashboardMetrics | undefined>(
    undefined,
  );

  return useQuery({
    enabled,
    placeholderData: keepPreviousData,
    queryFn: async ({ signal }) => {
      const next = await dashboard.getReliabilityMetrics(signal);
      const merged = mergeReliabilityDashboardMetrics(
        mergedMetricsRef.current,
        next,
      );
      mergedMetricsRef.current = merged;
      return merged;
    },
    queryKey: reliabilityDashboardMetricsQueryKey(),
    refetchInterval: reliabilityDashboardRefreshMilliseconds,
    staleTime: reliabilityDashboardRefreshMilliseconds,
  });
}
