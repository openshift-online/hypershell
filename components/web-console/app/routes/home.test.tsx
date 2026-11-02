import { render, screen } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type * as DashboardUi from "@openshift-online/hypershell-operational-dashboard-ui";
import { englishMessages } from "../i18n/catalog";

const { dashboardHostMock } = vi.hoisted(() => ({
  dashboardHostMock: vi.fn(),
}));

vi.mock("../lib/dashboard-host", () => ({
  isDashboardHost: dashboardHostMock,
}));

vi.mock("../features/homepage/homepage", () => ({
  HomepagePage: () => <div data-testid="homepage-page" />,
}));

vi.mock("../features/shell/use-session", () => ({
  useSession: () => ({
    data: { authenticated: true, authEnabled: true, roles: ["platform:admin"] },
    isLoading: false,
  }),
}));

vi.mock("@openshift-online/hypershell-operational-dashboard-ui", async () => {
  const actual = await vi.importActual<typeof DashboardUi>(
    "@openshift-online/hypershell-operational-dashboard-ui",
  );
  return {
    ...actual,
    OperationalDashboardPage: () => <div data-testid="dashboard-page" />,
  };
});

import HomeRoute from "./home";

function renderRoot() {
  render(
    <IntlProvider locale="en" messages={englishMessages}>
      <MemoryRouter initialEntries={["/"]}>
        <Routes>
          <Route element={<HomeRoute />} path="/" />
        </Routes>
      </MemoryRouter>
    </IntlProvider>,
  );
}

describe("HomeRoute", () => {
  beforeEach(() => {
    dashboardHostMock.mockReturnValue(false);
  });

  it("renders the homepage on the console host", () => {
    renderRoot();

    expect(screen.getByTestId("homepage-page")).toBeTruthy();
    expect(screen.queryByTestId("dashboard-page")).toBeNull();
  });

  it("keeps the operational dashboard on the dashboard host", () => {
    dashboardHostMock.mockReturnValue(true);
    renderRoot();

    expect(screen.getByTestId("dashboard-page")).toBeTruthy();
    expect(screen.queryByTestId("homepage-page")).toBeNull();
  });
});
