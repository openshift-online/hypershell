import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import { IntlProvider } from "react-intl";

import { createDashboardOperations } from "../application/dashboard-operations";
import type { OperationalDashboardMetrics } from "../application/dashboard-types";
import { DashboardUiProvider } from "../dashboard-ui-provider";
import { mockOperationalDashboardMetrics } from "../fixtures/mock-operational-dashboard-metrics";
import { messages } from "../messages";
import { OperationalDashboardPage } from "./operational-dashboard-page";

const messageCatalog = Object.fromEntries(
  Object.values(messages).map((message) => [
    message.id,
    message.defaultMessage,
  ]),
);

const metrics: OperationalDashboardMetrics = {
  ...mockOperationalDashboardMetrics,
  lastSuccessfulRefresh: new Date("2026-10-27T14:00:00.000Z"),
};

function renderPage(isAdmin: boolean) {
  const dashboard = createDashboardOperations({
    controlPlane: {
      getOperationalMetrics: () => Promise.resolve(metrics),
      getReliabilityMetrics: () => Promise.resolve(metrics),
      getAdoptionMetrics: () => Promise.resolve(metrics),
    },
  });

  window.localStorage.clear();

  render(
    <IntlProvider locale="en" messages={messageCatalog}>
      <QueryClientProvider client={new QueryClient()}>
        <DashboardUiProvider
          dashboard={dashboard}
          isAdmin={isAdmin}
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
          <OperationalDashboardPage metrics={metrics} />
        </DashboardUiProvider>
      </QueryClientProvider>
    </IntlProvider>,
  );
}

describe("OperationalDashboardPage", () => {
  it("marks every Overview widget as System data for admins", () => {
    renderPage(true);

    const systemWidgets = Array.from(
      document.querySelectorAll(".hypershell-dashboard-widget--system"),
    );

    expect(systemWidgets).toHaveLength(7);
    expect(
      systemWidgets.every((widget) =>
        widget.getAttribute("aria-label")?.endsWith("System"),
      ),
    ).toBe(true);
    expect(screen.queryByText("Admin")).toBeNull();
  });

  it("omits System widget headers for non-admins", () => {
    renderPage(false);

    expect(
      document.querySelector(".hypershell-dashboard-widget--system"),
    ).toBeNull();
    expect(screen.queryByText("Admin")).toBeNull();
  });
});
