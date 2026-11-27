import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { IntlProvider } from "react-intl";
import { describe, expect, it, vi } from "vitest";

import type {
  CostDashboardMetrics,
  CostDashboardView,
} from "../application/cost-dashboard-types";
import { createDashboardOperations } from "../application/dashboard-operations";
import type { DashboardInvocationContext } from "../application/dashboard-types";
import { costMessages } from "../cost-messages";
import { DashboardUiProvider } from "../dashboard-ui-provider";
import {
  mockCostDashboardMetrics,
  mockCostDashboardNotConfiguredMetrics,
  mockMyCostDashboardMetrics,
} from "../fixtures/mock-cost-dashboard-metrics";
import { messages } from "../messages";
import { CostDashboardPage } from "./cost-dashboard-page";
import { mergeCostDashboardMetrics } from "./get-cost-metrics-data";

const catalog = Object.fromEntries(
  [...Object.values(messages), ...Object.values(costMessages)].map(
    (message) => [message.id, message.defaultMessage],
  ),
);

function renderPage({
  isAdmin,
  load,
  metrics,
}: {
  isAdmin: boolean;
  load?: (context: DashboardInvocationContext) => Promise<CostDashboardMetrics>;
  metrics?: CostDashboardMetrics;
}) {
  const unused = () => Promise.reject(new Error("not used"));
  const dashboard = createDashboardOperations({
    controlPlane: {
      getAdoptionMetrics: unused,
      getCostMetrics: load,
      getOperationalMetrics: unused,
      getReliabilityMetrics: unused,
    },
  });
  render(
    <IntlProvider locale="en" messages={catalog}>
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <DashboardUiProvider
          dashboard={dashboard}
          isAdmin={isAdmin}
          navigation={{
            adoptionHref: "/dashboard/adoption",
            collectionHref: "/",
            costHref: "/dashboard/cost",
            dashboardHref: "/dashboard",
            inventoryHref: "/dashboard/inventory",
            navigate: () => undefined,
            provisioningHref: "/dashboard/provisioning",
            reliabilityHref: "/dashboard/reliability",
            usageHref: "/dashboard/usage",
          }}
        >
          <CostDashboardPage metrics={metrics} />
        </DashboardUiProvider>
      </QueryClientProvider>
    </IntlProvider>,
  );
}

describe("CostDashboardPage", () => {
  // mockup parity
  // Verifies: WEB-COST-08
  it("renders the platform widgets from the approved mockup for admins", () => {
    renderPage({ isAdmin: true, metrics: mockCostDashboardMetrics });

    expect(
      screen.getByRole("heading", { name: "Cost dashboard" }),
    ).toBeTruthy();
    expect(screen.getByText("$18,420 / month")).toBeTruthy();
    for (const title of [
      "Cost summary",
      "By environment",
      "By cluster",
      "By resource class",
    ]) {
      expect(screen.getAllByText(title).length).toBeGreaterThan(0);
    }
    expect(screen.getByText("$46.05", { selector: "h3" })).toBeTruthy();
    expect(screen.getByText("12% of estimated monthly cost")).toBeTruthy();
    // WEB-COST-02: platform widgets carry the System label.
    expect(
      document.querySelectorAll(".hypershell-dashboard-widget--system"),
    ).toHaveLength(8);
    // WEB-COST-07: the model is explained in the page.
    expect(
      screen.getByRole("heading", { name: "About these estimates" }),
    ).toBeTruthy();
  });

  // Verifies: WEB-COST-02.01
  it("loads only My cost for non-admins and shows no view switch", async () => {
    const views: (CostDashboardView | undefined)[] = [];
    renderPage({
      isAdmin: false,
      load: (context) => {
        views.push(context.costView);
        return Promise.resolve(mockMyCostDashboardMetrics);
      },
    });

    expect(await screen.findByText("$740.40 / month")).toBeTruthy();
    expect(views).toEqual(["mine"]);
    expect(screen.queryByRole("button", { name: "Platform" })).toBeNull();
    expect(
      document.querySelector(".hypershell-dashboard-widget--system"),
    ).toBeNull();
    // Two gateways are charged to the user; the third names its billing owner.
    expect(
      screen.getAllByText("Charged to me", { selector: "span" }),
    ).toHaveLength(2);
    expect(screen.getByText("dana")).toBeTruthy();
  });

  it("switches an admin between the platform and My cost views", async () => {
    const load = vi.fn((context: DashboardInvocationContext) =>
      Promise.resolve(
        context.costView === "mine"
          ? mockMyCostDashboardMetrics
          : mockCostDashboardMetrics,
      ),
    );
    renderPage({ isAdmin: true, load });

    expect(await screen.findByText("$18,420 / month")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "My cost" }));
    expect(await screen.findByText("$740.40 / month")).toBeTruthy();
    await waitFor(() => {
      expect(load.mock.calls.map(([context]) => context.costView)).toEqual([
        "platform",
        "mine",
      ]);
    });
  });

  // Verifies: WEB-COST-03.01
  it("shows the not-configured state in every widget and never $0", () => {
    renderPage({
      isAdmin: true,
      metrics: mockCostDashboardNotConfiguredMetrics,
    });

    expect(screen.getAllByText("Cost model not configured")).toHaveLength(8);
    expect(screen.queryByText(/\$0/u)).toBeNull();
  });

  // Verifies: WEB-COST-09.03
  it("shows an empty state for a user with no gateways", () => {
    renderPage({
      isAdmin: false,
      metrics: {
        ...mockMyCostDashboardMetrics,
        empty: true,
        mine: undefined,
      },
    });
    expect(screen.getByText("You have no gateways yet")).toBeTruthy();
    expect(screen.queryByText(/\$0/u)).toBeNull();
  });

  // Verifies: WEB-COST-06
  it("shows missing sources as unavailable, not zero", () => {
    renderPage({
      isAdmin: true,
      metrics: {
        ...mockCostDashboardMetrics,
        platform: { breakdown: mockCostDashboardMetrics.platform?.breakdown },
      },
    });
    expect(screen.getAllByText("Unavailable").length).toBeGreaterThan(0);
    expect(screen.queryByText("$0.00 / month")).toBeNull();
  });

  it("shows the partial coverage notice", () => {
    renderPage({
      isAdmin: true,
      metrics: { ...mockCostDashboardMetrics, coverageStart: "2026-11-20" },
    });
    expect(
      screen.getByText("Partial data for the selected range"),
    ).toBeTruthy();
  });
});

describe("mergeCostDashboardMetrics", () => {
  it("keeps the last good value of a source that failed on refresh", () => {
    const merged = mergeCostDashboardMetrics(mockCostDashboardMetrics, {
      ...mockCostDashboardMetrics,
      failedSources: ["cost-summary"],
      platform: { breakdown: mockCostDashboardMetrics.platform?.breakdown },
    });
    expect(merged.platform?.summary).toEqual(
      mockCostDashboardMetrics.platform?.summary,
    );
  });
});
