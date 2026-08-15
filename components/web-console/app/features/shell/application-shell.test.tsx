import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useGatewayUi } from "@openshift-online/hypershell-gateway-management-ui";
import { IntlProvider } from "react-intl";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { beforeEach, vi } from "vitest";

import { englishMessages } from "../../i18n/catalog";
import { ApplicationShell } from "./application-shell";

const { getGatewayMock } = vi.hoisted(() => ({
  getGatewayMock: vi.fn(),
}));
const navigateToGatewayLabel = "Navigate to gateway";

vi.mock("../../composition/gateway-composition", () => ({
  gatewayOperations: {
    findGatewayPlacements: vi.fn(),
    getGateway: getGatewayMock,
    getGatewayPlacement: vi.fn(),
    getGatewayPlacements: vi.fn(),
    listGateways: vi.fn(),
    provisionGateway: vi.fn(),
    removeGateway: vi.fn(),
    renameGateway: vi.fn(),
  },
}));

// The masthead identity menu reads the session; keep it unauthenticated here so
// shell assertions are unaffected. Menu behavior is covered in user-menu.test.
vi.mock("../../composition/session-composition", () => ({
  sessionGateway: {
    getSession: vi.fn().mockResolvedValue({ authenticated: false, roles: [] }),
  },
}));

function RouteContent() {
  const { pathname } = useLocation();
  const { navigation } = useGatewayUi();

  return (
    <>
      <h1>{pathname}</h1>
      <button
        onClick={() => {
          void navigation.navigate(navigation.detailHref("gateway-b"));
        }}
        type="button"
      >
        {navigateToGatewayLabel}
      </button>
    </>
  );
}

function renderShell(initialPath = "/") {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  return render(
    <IntlProvider locale="en" messages={englishMessages}>
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[initialPath]}>
          <Routes>
            <Route element={<ApplicationShell />}>
              <Route path="*" element={<RouteContent />} />
            </Route>
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>
    </IntlProvider>,
  );
}

describe("ApplicationShell", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getGatewayMock.mockResolvedValue({
      id: "gateway-b",
      name: "Friendly gateway",
    });
  });

  it("provides the focused HyperShell gateway shell", () => {
    const { container } = renderShell();

    expect(
      screen
        .getByRole("link", { name: "Skip to content" })
        .getAttribute("href"),
    ).toBe("#main-content");
    expect(screen.getByRole("link", { name: "HyperShell" })).toBeTruthy();
    expect(container.querySelector('img[src*="logo.png"]')).toBeTruthy();
    expect(screen.queryByText("Administration")).toBeNull();

    expect(
      screen.queryByRole("navigation", { name: "Primary navigation" }),
    ).toBeNull();
    expect(screen.queryByText("Connect with the CLI")).toBeNull();
  });

  it("uses the gateway name for a gateway deep link", async () => {
    renderShell("/gateways/gateway-b");

    const breadcrumb = screen.getByRole("navigation", { name: "Breadcrumb" });
    expect(
      within(breadcrumb)
        .getByRole("link", { name: "OpenShell Gateways" })
        .getAttribute("href"),
    ).toBe("/");
    expect(
      await within(breadcrumb).findByText("Friendly gateway"),
    ).toBeTruthy();
    expect(within(breadcrumb).queryByText("Gateway gateway-b")).toBeNull();
  });

  it("identifies the gateway provisioning route", () => {
    renderShell("/gateways/new");

    const breadcrumb = screen.getByRole("navigation", { name: "Breadcrumb" });
    expect(within(breadcrumb).getByText("OpenShell Gateways")).toBeTruthy();
    expect(within(breadcrumb).getByText("Provision gateway")).toBeTruthy();
  });

  it("provides host navigation to the gateway package", async () => {
    const user = userEvent.setup();
    renderShell();

    await user.click(
      screen.getByRole("button", { name: "Navigate to gateway" }),
    );

    expect(
      screen.getByRole("heading", {
        level: 1,
        name: "/gateways/gateway-b",
      }),
    ).toBeTruthy();
  });

  it("moves focus after shell navigation", async () => {
    const user = userEvent.setup();
    renderShell("/gateways/gateway-b");

    await user.click(screen.getByRole("link", { name: "HyperShell" }));

    expect(document.activeElement).toBe(
      screen.getByRole("heading", { level: 1, name: "/" }),
    );
  });
});
