import { render, screen } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type * as DashboardUi from "@openshift-online/hypershell-operational-dashboard-ui";

const { getSessionMock, useGetMetricsDataMock } = vi.hoisted(() => ({
  getSessionMock: vi.fn(),
  useGetMetricsDataMock: vi.fn(),
}));

vi.mock("../features/shell/use-session", () => ({
  useSession: getSessionMock,
}));

vi.mock("@openshift-online/hypershell-operational-dashboard-ui", async () => {
  const actual = await vi.importActual<typeof DashboardUi>(
    "@openshift-online/hypershell-operational-dashboard-ui",
  );

  return {
    ...actual,
    PlatformInventoryDashboardPage: () => (
      <div data-testid="platform-inventory-page" />
    ),
    useGetMetricsData: useGetMetricsDataMock,
  };
});

import DashboardInventoryRoute from "./dashboard-inventory";

const metricsQuery = {
  data: { lastSuccessfulRefresh: new Date(), metrics: [] },
  isFetching: false,
  isPending: false,
  refetch: vi.fn(),
};

const OPERATIONAL_DASHBOARD = "Operational dashboard";

function renderRoute() {
  const router = createMemoryRouter(
    [
      { element: <DashboardInventoryRoute />, path: "/dashboard/inventory" },
      {
        element: <h1>{OPERATIONAL_DASHBOARD}</h1>,
        path: "/dashboard",
      },
    ],
    { initialEntries: ["/dashboard/inventory"] },
  );
  render(<RouterProvider router={router} />);
}

describe("DashboardInventoryRoute", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useGetMetricsDataMock.mockReturnValue(metricsQuery);
  });

  it("renders for a platform administrator", async () => {
    getSessionMock.mockReturnValue({
      data: {
        authenticated: true,
        authEnabled: true,
        roles: ["platform:admin"],
      },
      isLoading: false,
    });

    renderRoute();

    expect(await screen.findByTestId("platform-inventory-page")).toBeTruthy();
  });

  // Verifies: PLAT-INV-06.02
  it("redirects authenticated non-admin users to the operational dashboard", async () => {
    getSessionMock.mockReturnValue({
      data: {
        authenticated: true,
        authEnabled: true,
        roles: ["hypershell-users"],
      },
      isLoading: false,
    });

    renderRoute();

    expect(
      await screen.findByRole("heading", { name: "Operational dashboard" }),
    ).toBeTruthy();
    expect(screen.queryByTestId("platform-inventory-page")).toBeNull();
  });
});
