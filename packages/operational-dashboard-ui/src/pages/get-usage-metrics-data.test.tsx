import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { IntlProvider } from "react-intl";

import { createDashboardOperations } from "../application/dashboard-operations";
import type {
  DashboardInvocationContext,
  UsageDashboardMetrics,
} from "../application/dashboard-types";
import { DashboardUiProvider } from "../dashboard-ui-provider";
import {
  usageDashboardMetricsQueryKey,
  usageDashboardRefreshMilliseconds,
} from "../dashboard/dashboard-data";
import { useGetUsageMetricsData } from "./get-usage-metrics-data";

const metrics: UsageDashboardMetrics = {
  lastSuccessfulRefresh: new Date("2026-10-15T14:00:00.000Z"),
  metrics: [{ id: "gateway-request-volume", unit: "requests", value: "100" }],
};

describe("useGetUsageMetricsData", () => {
  it("passes scope, range, and abort signal through the operations port", async () => {
    const expectedMetrics = { ...metrics, timeRange: 30 as const };
    const getUsageMetrics = vi.fn((context: DashboardInvocationContext) =>
      Promise.resolve({ ...metrics, timeRange: context.timeRange }),
    );
    const dashboard = createDashboardOperations({
      controlPlane: {
        getOperationalMetrics: () => Promise.resolve(metrics),
        getReliabilityMetrics: () => Promise.resolve(metrics),
        getUsageMetrics,
      },
    });
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    const wrapper = ({ children }: { children: ReactNode }) => (
      <IntlProvider locale="en">
        <QueryClientProvider client={queryClient}>
          <DashboardUiProvider
            dashboard={dashboard}
            isAdmin
            navigation={{
              collectionHref: "/",
              navigate: () => {
                return undefined;
              },
            }}
          >
            {children}
          </DashboardUiProvider>
        </QueryClientProvider>
      </IntlProvider>
    );

    const { result } = renderHook(
      () => useGetUsageMetricsData({ timeRange: 30 }),
      { wrapper },
    );

    await waitFor(() => {
      expect(result.current.data).toEqual(expectedMetrics);
    });

    expect(getUsageMetrics).toHaveBeenCalledTimes(1);
    const context = getUsageMetrics.mock.calls[0]?.[0];

    expect(context).toMatchObject({
      isAdmin: true,
      timeRange: 30,
    });
    expect(context?.signal).toBeInstanceOf(AbortSignal);
  });

  it("uses a range-aware query key and the shared refresh cadence", () => {
    expect(usageDashboardMetricsQueryKey(30)).toEqual([
      "usage-dashboard",
      "metrics",
      30,
    ]);
    expect(usageDashboardRefreshMilliseconds).toBe(15 * 60 * 1000);
  });
});
