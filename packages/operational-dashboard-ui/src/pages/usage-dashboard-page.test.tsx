import { fireEvent, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { IntlProvider } from "react-intl";

import { createDashboardOperations } from "../application/dashboard-operations";
import type { UsageDashboardMetrics } from "../application/dashboard-types";
import { DashboardUiProvider } from "../dashboard-ui-provider";
import { messages } from "../messages";
import { UsageDashboardPage } from "./usage-dashboard-page";

const messageCatalog = Object.fromEntries(
  Object.values(messages).map((message) => [
    message.id,
    message.defaultMessage,
  ]),
);

const metrics: UsageDashboardMetrics = {
  lastSuccessfulRefresh: new Date("2026-10-15T14:00:00.000Z"),
  metrics: [
    { id: "gateway-request-volume", unit: "requests", value: "1240000" },
    { id: "gateway-error-rate", unit: "%", value: "1.25" },
    { id: "active-gateways", total: "97", unit: "gateways", value: "72" },
    { id: "unused-gateways", unit: "gateways", value: "25" },
    {
      id: "sandbox-utilization",
      total: "340",
      unit: "sandboxes",
      value: "214",
    },
    {
      id: "sandbox-lifetime",
      sandboxLifetime: {
        mean: "67320",
        p50: "29520",
        p95: "367200",
      },
      value: "67320",
    },
  ],
  timeRange: 7,
};

function renderPage({
  isAdmin = true,
  metricsOverride = metrics,
}: {
  isAdmin?: boolean;
  metricsOverride?: UsageDashboardMetrics;
} = {}) {
  const dashboard = createDashboardOperations({
    controlPlane: {
      getOperationalMetrics: () => Promise.resolve(metrics),
      getReliabilityMetrics: () => Promise.resolve(metrics),
      getAdoptionMetrics: () => Promise.resolve(metrics),
      getUsageMetrics: () => Promise.resolve(metrics),
    },
  });

  window.localStorage.clear();

  render(
    <IntlProvider locale="en" messages={messageCatalog}>
      <QueryClientProvider client={new QueryClient()}>
        <DashboardUiProvider
          dashboard={dashboard}
          isAdmin={isAdmin}
          navigation={{ collectionHref: "/", navigate: () => undefined }}
        >
          <UsageDashboardPage metrics={metricsOverride} />
        </DashboardUiProvider>
      </QueryClientProvider>
    </IntlProvider>,
  );
}

describe("UsageDashboardPage", () => {
  it("renders all six usage widgets", () => {
    renderPage();

    expect(
      screen.getByRole("heading", { name: "Usage dashboard" }),
    ).toBeTruthy();
    expect(screen.getByText("Gateway request volume")).toBeTruthy();
    expect(screen.getByText("Gateway error rate")).toBeTruthy();
    expect(screen.getByText("Active gateways")).toBeTruthy();
    expect(screen.getByText("Provisioned but unused")).toBeTruthy();
    expect(screen.getByText("Sandbox utilization")).toBeTruthy();
    expect(screen.getByText("Sandbox lifetime")).toBeTruthy();
  });

  it("adds the System header treatment to every widget for admins", () => {
    renderPage({ isAdmin: true });

    const systemWidgets = Array.from(
      document.querySelectorAll(".hypershell-usage-widget--system"),
    );

    expect(systemWidgets).toHaveLength(6);
    expect(
      systemWidgets.every((widget) =>
        widget.getAttribute("aria-label")?.endsWith("System"),
      ),
    ).toBe(true);
    expect(
      systemWidgets.every(
        (widget) =>
          (widget as HTMLElement).style.getPropertyValue(
            "--hypershell-system-label",
          ) === '"System"',
      ),
    ).toBe(true);
    expect(screen.getByText(/Metrics cover the entire fleet\./u)).toBeTruthy();
  });

  it("omits System labels and describes authorized scope for non-admins", () => {
    renderPage({ isAdmin: false });

    expect(
      document.querySelector(".hypershell-usage-widget--system"),
    ).toBeNull();
    expect(
      screen.getByText(/Metrics are scoped to the resources you can access\./u),
    ).toBeTruthy();
  });

  it("shows a partial-data notice without converting missing points to zero", () => {
    renderPage({
      metricsOverride: {
        ...metrics,
        metrics: [
          {
            id: "gateway-request-volume",
            trend: {
              isPartial: true,
              points: [
                { label: "2026-10-14", value: 100 },
                { label: "2026-10-15", value: null },
              ],
            },
            unit: "requests",
            value: "100",
          },
        ],
      },
    });

    expect(screen.getByText("Some usage data is incomplete")).toBeTruthy();
    expect(screen.getByText("Partial data")).toBeTruthy();
  });

  it("renders unavailable widgets when a metric source is omitted", () => {
    renderPage({
      metricsOverride: {
        ...metrics,
        metrics: [{ id: "sandbox-utilization", total: "10", value: "4" }],
      },
    });

    expect(
      screen.getAllByText("Metric unavailable").length,
    ).toBeGreaterThanOrEqual(5);
  });

  it("supports changing between 7-day and 30-day ranges", () => {
    const onTimeRangeChange = vi.fn();
    const dashboard = createDashboardOperations({
      controlPlane: {
        getOperationalMetrics: () => Promise.resolve(metrics),
        getReliabilityMetrics: () => Promise.resolve(metrics),
        getAdoptionMetrics: () => Promise.resolve(metrics),
        getUsageMetrics: () => Promise.resolve(metrics),
      },
    });

    render(
      <IntlProvider locale="en" messages={messageCatalog}>
        <QueryClientProvider client={new QueryClient()}>
          <DashboardUiProvider
            dashboard={dashboard}
            isAdmin
            navigation={{ collectionHref: "/", navigate: () => undefined }}
          >
            <UsageDashboardPage
              metrics={metrics}
              onTimeRangeChange={onTimeRangeChange}
              timeRange={7}
            />
          </DashboardUiProvider>
        </QueryClientProvider>
      </IntlProvider>,
    );

    fireEvent.change(screen.getByLabelText("Time range"), {
      target: { value: "30" },
    });

    expect(onTimeRangeChange).toHaveBeenCalledWith(30);
  });
});
