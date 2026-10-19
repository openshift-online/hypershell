import { render, screen } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { createMemoryRouter, RouterProvider } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { getSessionMock } = vi.hoisted(() => ({
  getSessionMock: vi.fn(),
}));

vi.mock("../features/shell/use-session", () => ({
  useSession: getSessionMock,
}));

vi.mock("@openshift-online/hypershell-operational-dashboard-ui", () => ({
  AdoptionDashboardPage: () => <div data-testid="adoption-page" />,
}));

import DashboardAdoptionRoute from "./dashboard-adoption";
import { englishMessages } from "../i18n/catalog";

function renderRoute() {
  const router = createMemoryRouter(
    [{ element: <DashboardAdoptionRoute />, path: "/dashboard/adoption" }],
    { initialEntries: ["/dashboard/adoption"] },
  );
  render(
    <IntlProvider locale="en" messages={englishMessages}>
      <RouterProvider router={router} />
    </IntlProvider>,
  );
}

describe("DashboardAdoptionRoute", () => {
  beforeEach(() => {
    vi.clearAllMocks();
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

    expect(await screen.findByTestId("adoption-page")).toBeTruthy();
  });

  it("shows access denied for an authenticated non-admin", async () => {
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
      await screen.findByRole("heading", { name: "Access denied" }),
    ).toBeTruthy();
    expect(screen.queryByTestId("adoption-page")).toBeNull();
  });
});
