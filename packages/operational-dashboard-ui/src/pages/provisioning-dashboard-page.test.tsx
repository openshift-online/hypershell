import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { IntlProvider } from "react-intl";

import { createDashboardOperations } from "../application/dashboard-operations";
import type { OperationalMetric } from "../application/dashboard-types";
import { DashboardUiProvider } from "../dashboard-ui-provider";
import { messages } from "../messages";
import { ProvisioningDashboardPage } from "./provisioning-dashboard-page";

const messageCatalog = Object.fromEntries(
  Object.values(messages).map((message) => [
    message.id,
    message.defaultMessage,
  ]),
);

function renderProvisioningDashboard(
  metrics: readonly OperationalMetric[] | undefined,
  options: { hasInitialLoadError?: boolean; timeRange?: 7 | 30 } = {},
) {
  const operationalMetrics = metrics ?? [];
  const dashboard = createDashboardOperations({
    controlPlane: {
      getOperationalMetrics: () =>
        Promise.resolve({
          lastSuccessfulRefresh: new Date(),
          metrics: operationalMetrics,
        }),
      getReliabilityMetrics: () =>
        Promise.resolve({
          lastSuccessfulRefresh: new Date(),
          metrics: [],
        }),
      getAdoptionMetrics: () =>
        Promise.resolve({
          lastSuccessfulRefresh: new Date(),
          metrics: [],
        }),
    },
  });

  render(
    <IntlProvider locale="en" messages={messageCatalog}>
      <DashboardUiProvider
        dashboard={dashboard}
        isAdmin
        navigation={{
          adoptionHref: "/dashboard/adoption",
          collectionHref: "/",
          dashboardHref: "/dashboard",
          inventoryHref: "/dashboard/inventory",
          provisioningHref: "/dashboard/provisioning",
          reliabilityHref: "/dashboard/reliability",
          usageHref: "/dashboard/usage",
          navigate: () => undefined,
        }}
      >
        <ProvisioningDashboardPage
          hasInitialLoadError={options.hasInitialLoadError ?? false}
          metrics={
            metrics === undefined
              ? undefined
              : {
                  lastSuccessfulRefresh: new Date(),
                  metrics: operationalMetrics,
                }
          }
          onRefresh={() => undefined}
          onTimeRangeChange={() => undefined}
          timeRange={options.timeRange ?? 7}
        />
      </DashboardUiProvider>
    </IntlProvider>,
  );
}

function getSuccessRateChangeValue() {
  const row = screen.getByText("Success rate change").closest("div");
  expect(row).not.toBeNull();
  return within(row as HTMLElement);
}

function getSummaryValue(label: string) {
  const row = screen.getByText(label).closest("div");
  expect(row).not.toBeNull();
  return within(row as HTMLElement);
}

describe("ProvisioningDashboardPage", () => {
  it("does not invent a success-rate change when trend data is unavailable", () => {
    renderProvisioningDashboard([
      {
        id: "provision-reliability",
        provisionOutcomes: {
          failureCount24h: "0",
          successCount24h: "1",
          successRatePercent: "100",
        },
        value: "100",
      },
    ]);

    const successfulValue = getSummaryValue("Successful").getByText("1");
    expect(screen.getByText("Successful").tagName).not.toBe("STRONG");
    expect(successfulValue.tagName).toBe("STRONG");
    expect(getSuccessRateChangeValue().getByText("-")).toBeTruthy();
    expect(screen.queryByText("+3%")).toBeNull();
  });

  it("derives the success-rate change from the API trend", () => {
    renderProvisioningDashboard([
      {
        id: "provision-reliability",
        provisionOutcomes: {
          failureCount24h: "1",
          successCount24h: "54",
          successRatePercent: "98.2",
        },
        successRateTrend: {
          points: [
            { label: "2026-10-06", value: 90 },
            { label: "2026-10-13", value: 99 },
          ],
        },
        value: "98.2",
      },
    ]);

    expect(getSuccessRateChangeValue().getByText("+10%")).toBeTruthy();
  });

  it("derives memory and CPU utilization and trend direction from API metrics", () => {
    renderProvisioningDashboard([
      {
        id: "memory",
        total: "100",
        trend: {
          points: [
            { label: "2026-10-06", value: 4 },
            { label: "2026-10-13", value: 5 },
          ],
        },
        unit: "GiB",
        value: "5",
      },
      {
        id: "cpu",
        total: "100",
        trend: {
          points: [
            { label: "2026-10-06", value: 2 },
            { label: "2026-10-13", value: 1 },
          ],
        },
        unit: "cores",
        value: "1",
      },
    ]);

    const memoryRow = screen.getByText("Memory used").closest("div");
    const cpuRow = screen.getByText("CPU used").closest("div");
    expect(memoryRow).not.toBeNull();
    expect(cpuRow).not.toBeNull();
    expect(within(memoryRow as HTMLElement).getByText("5%")).toBeTruthy();
    expect(within(cpuRow as HTMLElement).getByText("1%")).toBeTruthy();
    expect(
      memoryRow?.querySelector(
        ".hypershell-dashboard-summary-list__indicator--negative",
      ),
    ).toBeTruthy();
    expect(
      cpuRow?.querySelector(
        ".hypershell-dashboard-summary-list__indicator--positive",
      ),
    ).toBeTruthy();
  });

  it("shows an unavailable state when provision duration data is missing", () => {
    renderProvisioningDashboard([
      {
        id: "provision-time",
        value: "0",
      },
    ]);

    expect(screen.getAllByText("Metric unavailable").length).toBeGreaterThan(0);
    expect(
      screen.getAllByText("This information is not currently available.")
        .length,
    ).toBeGreaterThan(0);
  });

  it("shows an unavailable state when the provision time metric is absent", () => {
    renderProvisioningDashboard([]);

    expect(screen.getAllByText("Metric unavailable").length).toBeGreaterThan(0);
    expect(
      screen.getAllByText("This information is not currently available.")
        .length,
    ).toBeGreaterThan(0);
  });

  it("uses the selected range for memory, CPU, and reliability captions", () => {
    renderProvisioningDashboard(
      [
        {
          id: "memory",
          total: "100",
          trend: {
            points: [
              { label: "2026-10-01", value: 4 },
              { label: "2026-10-02", value: 5 },
            ],
          },
          unit: "GiB",
          value: "5",
        },
        {
          id: "cpu",
          total: "100",
          trend: {
            points: [
              { label: "2026-10-01", value: 2 },
              { label: "2026-10-02", value: 1 },
            ],
          },
          unit: "cores",
          value: "1",
        },
        {
          id: "provision-reliability",
          provisionOutcomes: {
            failureCount24h: "1",
            successCount24h: "54",
            successRatePercent: "98.2",
          },
          successRateTrend: {
            points: [
              { label: "2026-10-01", value: 90 },
              { label: "2026-10-02", value: 99 },
            ],
          },
          value: "98.2",
        },
      ],
      { timeRange: 30 },
    );

    expect(screen.getAllByText("Last 30 days")).toHaveLength(3);
  });

  // Verifies: WEB-OPS-17.03
  it("does not render a historical trend in the pods widget", () => {
    renderProvisioningDashboard([
      {
        id: "pods",
        podPhases: {
          failed: 0,
          pending: 1,
          running: 8,
          succeeded: 1,
          unknown: 0,
        },
        total: "100",
        trend: {
          points: [
            { label: "2026-10-01", value: 8 },
            { label: "2026-10-02", value: 10 },
          ],
        },
        unit: "pods",
        value: "10",
      },
    ]);

    expect(screen.getByText("of 100 pods")).toBeTruthy();
    expect(
      document.querySelector(".hypershell-dashboard-sparkline-chart__caption"),
    ).toBeNull();
  });

  it("shows a danger recovery state when the initial metric load fails", () => {
    renderProvisioningDashboard(undefined, { hasInitialLoadError: true });

    expect(
      screen.getByText("Operational dashboard metrics are unavailable"),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "An unexpected error occurred while loading dashboard metrics.",
      ),
    ).toBeTruthy();
  });
});
