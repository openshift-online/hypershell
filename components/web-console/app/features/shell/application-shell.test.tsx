import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { IntlProvider } from "react-intl";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { beforeEach, vi } from "vitest";

import { englishMessages } from "../../i18n/catalog";
import { ApplicationShell } from "./application-shell";

const { listSectors } = vi.hoisted(() => ({
  listSectors: vi.fn(),
}));

vi.mock("../../lib/api.client", () => ({
  apiClient: {
    fleets: {
      list: listSectors,
    },
  },
}));

function RouteContent() {
  const { pathname } = useLocation();

  return <h1>{pathname}</h1>;
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
              <Route index element={<RouteContent />} />
              <Route path="*" element={<RouteContent />} />
            </Route>
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>
    </IntlProvider>,
  );
}

async function openNavigation() {
  const user = userEvent.setup();
  const toggle = screen.getByRole("button", {
    name: "Toggle primary navigation",
  });
  if (toggle.getAttribute("aria-expanded") === "false") {
    await user.click(toggle);
  }

  return screen.getByRole("navigation", { name: "Primary navigation" });
}

describe("ApplicationShell", () => {
  beforeEach(() => {
    listSectors.mockReset();
    listSectors.mockResolvedValue({
      items: [
        { id: "sector-a", name: "Alpha sector" },
        { id: "sector-b", name: "Beta sector" },
      ],
      kind: "FleetList",
      page: 1,
      size: 100,
      total: 2,
    });
  });

  it("provides stable global navigation and a bypass link", async () => {
    renderShell();

    expect(
      screen
        .getByRole("link", { name: "Skip to content" })
        .getAttribute("href"),
    ).toBe("#main-content");
    expect(screen.getByRole("link", { name: "HyperShell" })).toBeTruthy();
    expect(screen.getByText("Development preview")).toBeTruthy();

    const navigation = await openNavigation();
    expect(
      within(navigation)
        .getByRole("link", { name: "Overview" })
        .getAttribute("aria-current"),
    ).toBe("page");
    expect(
      within(navigation).getByRole("link", { name: "Sectors" }),
    ).toBeTruthy();
    expect(screen.queryByText("Selected sector")).toBeNull();
    expect(screen.queryByRole("button", { name: /Select sector/ })).toBeNull();
  });

  it("keeps the sectors collection global", () => {
    renderShell("/fleets");

    expect(screen.queryByRole("button", { name: /Select sector/ })).toBeNull();
  });

  it("identifies the selected sector and gateway on a deep link", async () => {
    renderShell("/fleets/sector-a/gateways/gateway-b");

    const navigation = await openNavigation();
    expect(within(navigation).getByText("Selected sector")).toBeTruthy();
    expect(
      within(navigation)
        .getByRole("link", { name: "Gateways" })
        .getAttribute("aria-current"),
    ).toBe("page");

    const breadcrumb = screen.getByRole("navigation", { name: "Breadcrumb" });
    expect(within(breadcrumb).getByText("Sector sector-a")).toBeTruthy();
    expect(within(breadcrumb).getByText("Gateway gateway-b")).toBeTruthy();
    expect(
      await screen.findByRole("button", {
        name: "Select sector, currently Alpha sector",
      }),
    ).toBeTruthy();
  });

  it("switches sector context and leaves a gateway detail safely", async () => {
    const user = userEvent.setup();
    renderShell("/fleets/sector-a/gateways/gateway-b");

    await user.click(
      await screen.findByRole("button", {
        name: "Select sector, currently Alpha sector",
      }),
    );
    await user.click(screen.getByRole("option", { name: "Beta sector" }));

    expect(
      screen.getByRole("heading", {
        level: 1,
        name: "/fleets/sector-b/gateways",
      }),
    ).toBeTruthy();
  });

  it.each([
    ["/fleets/sector-a", "Sector sector-a"],
    ["/fleets/sector-a/settings", "Settings"],
    ["/fleets/sector-a/clients", "Clients"],
    ["/fleets/sector-a/keys", "Keys"],
  ])("renders contextual breadcrumbs for %s", (path, label) => {
    renderShell(path);

    const breadcrumb = screen.getByRole("navigation", { name: "Breadcrumb" });
    expect(within(breadcrumb).getByText(label)).toBeTruthy();
  });

  it("moves focus to the new page heading after client-side navigation", async () => {
    const user = userEvent.setup();
    renderShell();
    const navigation = await openNavigation();

    await user.click(within(navigation).getByRole("link", { name: "Sectors" }));

    expect(document.activeElement).toBe(
      screen.getByRole("heading", { level: 1, name: "/fleets" }),
    );
  });
});
