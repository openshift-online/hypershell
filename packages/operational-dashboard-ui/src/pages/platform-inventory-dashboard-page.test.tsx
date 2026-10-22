import { fireEvent, render, screen, within } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { useState } from "react";
import { describe, expect, it } from "vitest";

import type {
  OperationalDashboardMetrics,
  OperationalMetric,
} from "../application/dashboard-types";
import { createDashboardOperations } from "../application/dashboard-operations";
import { DashboardUiProvider } from "../dashboard-ui-provider";
import { messages } from "../messages";
import { PlatformInventoryDashboardPage } from "./platform-inventory-dashboard-page";

const messageCatalog = Object.fromEntries(
  Object.values(messages).map((message) => [
    message.id,
    message.defaultMessage,
  ]),
);

const managedClusterMetric: OperationalMetric = {
  createdLast7Days: "1",
  createdLast30Days: "2",
  id: "managed-clusters",
  inventoryProviders: { AWS: 4, "IBM Cloud": 3, Other: 1 },
  inventoryRegions: {
    "us-east (AWS)": 3,
    "us-central (IBM Cloud)": 2,
    "eu-west (IBM Cloud)": 2,
    "ap-south (Other)": 1,
  },
  inventoryStatus: { Failed: 1, Ready: 7 },
  value: "8",
};

const metrics: OperationalDashboardMetrics = {
  lastSuccessfulRefresh: new Date("2026-10-13T09:00:00.000Z"),
  metrics: [managedClusterMetric],
};

function renderPage(
  timeRange: 7 | 30 = 7,
  metricsOverride: OperationalDashboardMetrics = metrics,
) {
  const dashboard = createDashboardOperations({
    controlPlane: {
      getOperationalMetrics: () => Promise.resolve(metrics),
      getReliabilityMetrics: () =>
        Promise.resolve({ lastSuccessfulRefresh: new Date(), metrics: [] }),
      getAdoptionMetrics: () =>
        Promise.resolve({ lastSuccessfulRefresh: new Date(), metrics: [] }),
    },
  });

  window.localStorage.clear();
  function TimeRangeHarness() {
    const [selectedTimeRange, setSelectedTimeRange] = useState(timeRange);

    return (
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
          <PlatformInventoryDashboardPage
            isLoading={false}
            isRefreshing={false}
            metrics={metricsOverride}
            onRefresh={() => undefined}
            onTimeRangeChange={setSelectedTimeRange}
            timeRange={selectedTimeRange}
          />
        </DashboardUiProvider>
      </IntlProvider>
    );
  }

  render(<TimeRangeHarness />);
}

function getSummaryRow(label: string) {
  const row = screen.getByText(label).closest("div");
  expect(row).not.toBeNull();
  return row as HTMLElement;
}

function getSummaryValue(label: string) {
  return within(getSummaryRow(label));
}

describe("PlatformInventoryDashboardPage", () => {
  it("renders every summary number from the managed-cluster API metric", () => {
    renderPage(7);

    expect(getSummaryValue("Managed clusters").getByText("8")).toBeTruthy();
    expect(getSummaryValue("Recently added (7d)").getByText("1")).toBeTruthy();
    expect(getSummaryValue("Healthy clusters").getByText("7")).toBeTruthy();
    expect(
      getSummaryValue("Clusters needing attention").getByText("1"),
    ).toBeTruthy();
    expect(getSummaryValue("Inventory exceptions").getByText("1")).toBeTruthy();
    expect(
      getSummaryRow("Inventory exceptions").querySelector(
        ".hypershell-dashboard-summary-list__indicator--negative",
      ),
    ).toBeTruthy();
  });

  it("shows warning semantics for degraded and pending inventory buckets", () => {
    renderPage(7, {
      ...metrics,
      metrics: [
        {
          ...managedClusterMetric,
          inventoryStatus: { Degraded: 2, Pending: 3, Ready: 5 },
        },
      ],
    });

    expect(getSummaryValue("Inventory exceptions").getByText("5")).toBeTruthy();
    expect(
      getSummaryRow("Inventory exceptions").querySelector(
        ".hypershell-dashboard-summary-list__indicator--warning",
      ),
    ).toBeTruthy();
    expect(
      getSummaryRow("Inventory exceptions").querySelector(
        ".hypershell-dashboard-summary-list__indicator--negative",
      ),
    ).toBeNull();
  });

  it("omits the inventory exception row for healthy-only inventories", () => {
    renderPage(7, {
      ...metrics,
      metrics: [
        {
          ...managedClusterMetric,
          inventoryStatus: { Other: 1, Ready: 7 },
          value: "8",
        },
      ],
    });

    expect(screen.queryByText("Inventory exceptions")).toBeNull();
  });

  it("selects the 30-day API count when that time range is active", () => {
    renderPage(30);

    expect(getSummaryValue("Recently added (30d)").getByText("2")).toBeTruthy();
  });

  it("changes the recently-added count when the user selects 30 days", () => {
    renderPage(7);

    expect(getSummaryValue("Recently added (7d)").getByText("1")).toBeTruthy();

    fireEvent.change(screen.getByLabelText("Time range"), {
      target: { value: "30" },
    });

    expect(getSummaryValue("Recently added (30d)").getByText("2")).toBeTruthy();
  });

  it("does not synthesize inventory numbers when the metric is unavailable", () => {
    window.localStorage.clear();
    const dashboard = createDashboardOperations({
      controlPlane: {
        getOperationalMetrics: () =>
          Promise.resolve({
            failedSources: ["platform-inventory"],
            lastSuccessfulRefresh: new Date(),
            metrics: [],
          }),
        getReliabilityMetrics: () =>
          Promise.resolve({ lastSuccessfulRefresh: new Date(), metrics: [] }),
        getAdoptionMetrics: () =>
          Promise.resolve({ lastSuccessfulRefresh: new Date(), metrics: [] }),
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
          <PlatformInventoryDashboardPage
            isLoading={false}
            isRefreshing={false}
            metrics={{
              failedSources: ["platform-inventory"],
              lastSuccessfulRefresh: new Date(),
              metrics: [],
            }}
            onRefresh={() => undefined}
            onTimeRangeChange={() => undefined}
            timeRange={7}
          />
        </DashboardUiProvider>
      </IntlProvider>,
    );

    expect(screen.getAllByText("Metric unavailable").length).toBe(3);
    expect(screen.queryByText("Recently added (7d)")).toBeNull();
  });
});
