import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { IntlProvider } from "react-intl";
import { createMemoryRouter, RouterProvider } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type * as DashboardUi from "@openshift-online/hypershell-operational-dashboard-ui";

const { getSessionMock } = vi.hoisted(() => ({
  getSessionMock: vi.fn(),
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
    CostDashboardPage: () => <div data-testid="cost-dashboard-page" />,
  };
});

import DashboardCostRoute from "./dashboard-cost";
import { englishMessages } from "../i18n/catalog";

function renderRoute() {
  const router = createMemoryRouter(
    [{ element: <DashboardCostRoute />, path: "/dashboard/cost" }],
    { initialEntries: ["/dashboard/cost"] },
  );
  render(
    <IntlProvider locale="en" messages={englishMessages}>
      <QueryClientProvider client={new QueryClient()}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </IntlProvider>,
  );
}

describe("DashboardCostRoute", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders for an authenticated platform administrator", async () => {
    getSessionMock.mockReturnValue({
      data: {
        authenticated: true,
        authEnabled: true,
        roles: ["platform:admin"],
      },
      isLoading: false,
    });

    renderRoute();

    expect(await screen.findByTestId("cost-dashboard-page")).toBeTruthy();
  });

  it("renders for an authenticated non-administrator", async () => {
    getSessionMock.mockReturnValue({
      data: {
        authenticated: true,
        authEnabled: true,
        roles: ["hypershell-users"],
      },
      isLoading: false,
    });

    renderRoute();

    expect(await screen.findByTestId("cost-dashboard-page")).toBeTruthy();
  });

  it("denies an unauthenticated session", async () => {
    getSessionMock.mockReturnValue({
      data: { authenticated: false, authEnabled: true, roles: [] },
      isLoading: false,
    });

    renderRoute();

    expect(
      await screen.findByRole("heading", { name: "Access denied" }),
    ).toBeTruthy();
    expect(screen.queryByTestId("cost-dashboard-page")).toBeNull();
  });
});
