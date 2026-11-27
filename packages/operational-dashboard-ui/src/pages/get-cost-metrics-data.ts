import { useQuery } from "@tanstack/react-query";
import { useRef } from "react";

import type {
  CostDashboardMetrics,
  CostDashboardView,
} from "../application/cost-dashboard-types";
import type { DashboardTimeRange } from "../application/dashboard-types";
import {
  costDashboardMetricsQueryKey,
  costDashboardRefreshMilliseconds,
} from "../dashboard/dashboard-data";
import { useDashboardUi } from "../dashboard-ui-provider";

export interface UseGetCostMetricsDataOptions {
  enabled?: boolean;
  timeRange?: DashboardTimeRange;
  view: CostDashboardView;
}

/**
 * Merge a refresh into the last good payload: a source that failed this time
 * keeps its previous value (stale-value preservation, as in WEB-USE-04).
 */
export function mergeCostDashboardMetrics(
  previous: CostDashboardMetrics | undefined,
  next: CostDashboardMetrics,
): CostDashboardMetrics {
  if (previous === undefined || (next.failedSources?.length ?? 0) === 0) {
    return next;
  }
  const failed = new Set(next.failedSources);
  return {
    ...next,
    platform: {
      summary: failed.has("cost-summary")
        ? previous.platform?.summary
        : next.platform?.summary,
      breakdown: failed.has("cost-breakdown")
        ? previous.platform?.breakdown
        : next.platform?.breakdown,
      topGateways: failed.has("cost-gateways")
        ? previous.platform?.topGateways
        : next.platform?.topGateways,
    },
    mine: {
      summary: failed.has("my-cost-summary")
        ? previous.mine?.summary
        : next.mine?.summary,
      gateways: failed.has("my-cost-gateways")
        ? previous.mine?.gateways
        : next.mine?.gateways,
      workspaces: failed.has("my-cost-gateways")
        ? previous.mine?.workspaces
        : next.mine?.workspaces,
    },
  };
}

export function useGetCostMetricsData({
  enabled = true,
  timeRange = 7,
  view,
}: UseGetCostMetricsDataOptions) {
  const { dashboard, isAdmin } = useDashboardUi();
  const lastGoodRef = useRef<Partial<Record<string, CostDashboardMetrics>>>({});

  return useQuery<
    CostDashboardMetrics,
    Error,
    CostDashboardMetrics,
    ReturnType<typeof costDashboardMetricsQueryKey>
  >({
    enabled,
    placeholderData: (previous: CostDashboardMetrics | undefined) =>
      previous?.view === view ? previous : undefined,
    queryFn: async ({ signal }) => {
      const key = `${view}:${String(timeRange)}`;
      if (!dashboard.getCostMetrics) {
        throw new Error("Dashboard operations do not provide cost metrics");
      }
      const next = await dashboard.getCostMetrics(signal, timeRange, {
        costView: view,
        isAdmin,
      });
      const merged = mergeCostDashboardMetrics(lastGoodRef.current[key], next);
      lastGoodRef.current[key] = merged;
      return merged;
    },
    queryKey: costDashboardMetricsQueryKey(view, timeRange),
    refetchInterval: costDashboardRefreshMilliseconds,
    staleTime: costDashboardRefreshMilliseconds,
  });
}
