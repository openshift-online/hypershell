import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { IntlProvider } from "react-intl";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { englishMessages } from "../../i18n/catalog";

const { getSessionMock } = vi.hoisted(() => ({
  getSessionMock: vi.fn(),
}));

vi.mock("../../composition/session-composition", () => ({
  sessionGateway: { getSession: getSessionMock },
}));

import { RequireDashboardAdmin } from "./require-dashboard-admin";

function renderGuard() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  return render(
    <IntlProvider locale="en" messages={englishMessages}>
      <QueryClientProvider client={queryClient}>
        <RequireDashboardAdmin>
          <div data-testid="dashboard-content" />
        </RequireDashboardAdmin>
      </QueryClientProvider>
    </IntlProvider>,
  );
}

describe("RequireDashboardAdmin", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders children when auth is disabled (no-auth mode)", async () => {
    getSessionMock.mockResolvedValue({
      authenticated: false,
      authEnabled: false,
      roles: [],
    });

    renderGuard();

    expect(await screen.findByTestId("dashboard-content")).toBeTruthy();
  });

  it("shows access denied when auth is enabled but the session is unauthenticated", async () => {
    getSessionMock.mockResolvedValue({
      authenticated: false,
      authEnabled: true,
      roles: [],
    });

    renderGuard();

    expect(
      await screen.findByRole("heading", { name: "Access denied" }),
    ).toBeTruthy();
    expect(screen.queryByTestId("dashboard-content")).toBeNull();
  });

  it("renders children for hypershell-admins", async () => {
    getSessionMock.mockResolvedValue({
      authenticated: true,
      authEnabled: true,
      roles: ["hypershell-admins"],
    });

    renderGuard();

    expect(await screen.findByTestId("dashboard-content")).toBeTruthy();
  });

  it("renders children for platform:admin", async () => {
    getSessionMock.mockResolvedValue({
      authenticated: true,
      authEnabled: true,
      roles: ["platform:admin"],
    });

    renderGuard();

    expect(await screen.findByTestId("dashboard-content")).toBeTruthy();
  });

  it("shows access denied for authenticated non-admin users", async () => {
    getSessionMock.mockResolvedValue({
      authenticated: true,
      authEnabled: true,
      roles: ["hypershell-users"],
    });

    renderGuard();

    expect(
      await screen.findByRole("heading", { name: "Access denied" }),
    ).toBeTruthy();
    expect(screen.queryByTestId("dashboard-content")).toBeNull();
  });
});
