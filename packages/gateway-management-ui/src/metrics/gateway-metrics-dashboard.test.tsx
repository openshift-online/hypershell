import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { afterEach, describe, expect, it, vi } from "vitest";

import { GatewayMetricsDashboard } from "./gateway-metrics-dashboard";
import * as gatewayMetricsData from "./gateway-metrics-data";

function createTestQueryClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
}

function renderDashboard(queryClient = createTestQueryClient()) {
  return render(
    <IntlProvider locale="en">
      <QueryClientProvider client={queryClient}>
        <GatewayMetricsDashboard />
      </QueryClientProvider>
    </IntlProvider>,
  );
}

describe("GatewayMetricsDashboard", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("shows a loading spinner while metrics are fetching", () => {
    vi.spyOn(gatewayMetricsData, "fetchGatewayMetrics").mockReturnValue(
      new Promise(() => undefined),
    );

    const { container } = renderDashboard();

    expect(screen.getByLabelText("Loading metrics")).toBeTruthy();
    expect(container.querySelector(".pf-v6-c-spinner")).toBeTruthy();
  });

  it("shows recovery guidance when metrics fail to load", async () => {
    vi.spyOn(gatewayMetricsData, "fetchGatewayMetrics").mockRejectedValue(
      new Error("Metrics unavailable"),
    );

    renderDashboard();

    expect(await screen.findByText("Metrics could not be loaded")).toBeTruthy();
    expect(
      screen.getByText(
        "Check that Prometheus is running and reachable, then refresh.",
      ),
    ).toBeTruthy();
  });

  it("renders all five phase cards when metrics load", async () => {
    vi.spyOn(gatewayMetricsData, "fetchGatewayMetrics").mockResolvedValue({
      Pending: 1,
      Running: 5,
      Provisioning: 2,
      Degraded: 1,
      Failed: 0,
    });

    renderDashboard();

    expect(await screen.findByText("Gateway metrics")).toBeTruthy();
    expect(screen.getByText("Pending")).toBeTruthy();
    expect(screen.getByText("Running")).toBeTruthy();
    expect(screen.getByText("Provisioning")).toBeTruthy();
    expect(screen.getByText("Degraded")).toBeTruthy();
    expect(screen.getByText("Failed")).toBeTruthy();
    expect(screen.getByText("5")).toBeTruthy();
    expect(screen.getByText("2")).toBeTruthy();
    expect(screen.getAllByText("1")).toHaveLength(2);
    expect(screen.getByText("0")).toBeTruthy();
    expect(screen.getByText("5 gateways")).toBeTruthy();
    expect(screen.getAllByText("1 gateway")).toHaveLength(2);
    expect(screen.getByText("0 gateways")).toBeTruthy();
  });
});
