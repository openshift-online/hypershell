import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  useGatewayProfileUi,
  useGatewayUi,
} from "@openshift-online/hypershell-gateway-management-ui";
import { IntlProvider } from "react-intl";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { beforeEach, vi } from "vitest";

import { englishMessages } from "../../i18n/catalog";
import { ApplicationShell } from "./application-shell";

const { getGatewayMock, getGatewayProfileMock } = vi.hoisted(() => ({
  getGatewayMock: vi.fn(),
  getGatewayProfileMock: vi.fn(),
}));
const navigateToGatewayLabel = "Navigate to gateway";
const navigateToGatewayProfileLabel = "Navigate to gateway profile";

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

vi.mock("../../composition/gateway-profile-composition", () => ({
  gatewayProfileOperations: {
    createGatewayProfile: vi.fn(),
    getGatewayProfile: getGatewayProfileMock,
    listGatewayProfiles: vi.fn(),
    removeGatewayProfile: vi.fn(),
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
  const { navigation: profileNavigation } = useGatewayProfileUi();

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
      <button
        onClick={() => {
          void profileNavigation.navigate(
            profileNavigation.detailHref("profile-b"),
          );
        }}
        type="button"
      >
        {navigateToGatewayProfileLabel}
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
    getGatewayProfileMock.mockResolvedValue({
      id: "profile-b",
      name: "Small profile",
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

  it("uses the profile name for a gateway profile deep link", async () => {
    renderShell("/gateway-profiles/profile-b");

    const breadcrumb = screen.getByRole("navigation", { name: "Breadcrumb" });
    expect(
      within(breadcrumb)
        .getByRole("link", { name: "Gateway profiles" })
        .getAttribute("href"),
    ).toBe("/gateway-profiles");
    expect(await within(breadcrumb).findByText("Small profile")).toBeTruthy();
  });

  it("identifies the gateway profile creation route", () => {
    renderShell("/gateway-profiles/new");

    const breadcrumb = screen.getByRole("navigation", { name: "Breadcrumb" });
    expect(within(breadcrumb).getByText("Gateway profiles")).toBeTruthy();
    expect(within(breadcrumb).getByText("Create gateway profile")).toBeTruthy();
  });

  it("provides host navigation to the gateway profile package", async () => {
    const user = userEvent.setup();
    renderShell();

    await user.click(
      screen.getByRole("button", { name: navigateToGatewayProfileLabel }),
    );

    expect(
      screen.getByRole("heading", {
        level: 1,
        name: "/gateway-profiles/profile-b",
      }),
    ).toBeTruthy();
  });
});
