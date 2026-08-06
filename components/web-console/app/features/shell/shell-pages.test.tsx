import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { IntlProvider } from "react-intl";
import { MemoryRouter } from "react-router";
import { beforeEach, vi } from "vitest";

import { englishMessages } from "../../i18n/catalog";
import { previewGateways } from "../gateways/gateway-connections";
import type * as GatewayData from "../gateways/gateway-data";
import { AdminGatewayPage, AdminGatewaysPage } from "./shell-pages";

const { listGatewayConnectionsMock } = vi.hoisted(() => ({
  listGatewayConnectionsMock: vi.fn(),
}));

vi.mock("../gateways/gateway-data", async (importOriginal) => ({
  ...(await importOriginal<typeof GatewayData>()),
  listGatewayConnections: listGatewayConnectionsMock,
}));

function renderPage(Page: () => React.ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  return render(
    <IntlProvider locale="en" messages={englishMessages}>
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <Page />
        </MemoryRouter>
      </QueryClientProvider>
    </IntlProvider>,
  );
}

describe("admin shell pages", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders gateway details returned by the API", () => {
    renderPage(() => (
      <AdminGatewayPage
        gateway={{
          cluster_id: "",
          created_at: null,
          database_id: "database-1",
          external_dns: "gateway.example.com",
          fleet_id: "",
          href: "/api/hypershell/v1/gateways/gateway-1",
          id: "gateway-1",
          kind: "Gateway",
          name: "Team gateway",
          namespace: "openshell",
          phase: "",
          release_id: "release-1",
          service_type: "",
          status: "Ready",
          tls_mode: "",
          updated_at: null,
        }}
      />
    ));

    expect(
      screen.getByRole("heading", {
        level: 1,
        name: "Team gateway",
      }),
    ).toBeTruthy();
    expect(screen.getByText("https://gateway.example.com:443")).toBeTruthy();
    expect(screen.getByText("release-1")).toBeTruthy();
  });

  it("shows an empty state when there are no gateways to administer", () => {
    renderPage(() => <AdminGatewaysPage gateways={[]} />);

    expect(
      screen.getByRole("heading", {
        level: 2,
        name: "No gateways to administer",
      }),
    ).toBeTruthy();
  });

  it("loads the administrative gateway list from the API", async () => {
    listGatewayConnectionsMock.mockResolvedValue(previewGateways);

    renderPage(AdminGatewaysPage);

    expect(screen.getByLabelText("Loading gateways")).toBeTruthy();
    expect(
      await screen.findByRole("link", { name: "openshell-gateway-test" }),
    ).toBeTruthy();
  });

  it("refreshes the administrative gateway list", async () => {
    const user = userEvent.setup();
    listGatewayConnectionsMock.mockResolvedValue(previewGateways);
    renderPage(AdminGatewaysPage);

    await screen.findByRole("link", { name: "openshell-gateway-test" });
    const filter = screen.getByRole("textbox", {
      name: "Filter by name, cluster, status, or endpoint",
    });
    await user.type(filter, "openshell");
    await user.click(screen.getByRole("button", { name: "Refresh gateways" }));

    await waitFor(() => {
      expect(listGatewayConnectionsMock).toHaveBeenCalledTimes(2);
    });
    expect(filter.getAttribute("value")).toBe("openshell");
  });

  it("shows administrative gateway API failures", async () => {
    listGatewayConnectionsMock.mockRejectedValue(new Error("unavailable"));

    renderPage(AdminGatewaysPage);

    expect(
      await screen.findByText("Gateways could not be loaded"),
    ).toBeTruthy();
  });

  it("sorts and filters the administrative gateway list", async () => {
    const user = userEvent.setup();
    renderPage(() => (
      <AdminGatewaysPage
        gateways={[
          {
            clusterName: "West cluster",
            consoleUrl: "https://console.example/zulu",
            endpoint: "https://zulu.example:443",
            id: "zulu",
            name: "Zulu gateway",
            oidcAudience: "openshell-cli",
            oidcClientId: "openshell-cli",
            oidcIssuer: "https://issuer.example",
            status: "Ready",
          },
          {
            clusterName: "East cluster",
            consoleUrl: "https://console.example/alpha",
            endpoint: "https://alpha.example:443",
            id: "alpha",
            name: "Alpha gateway",
            oidcAudience: "openshell-cli",
            oidcClientId: "openshell-cli",
            oidcIssuer: "https://issuer.example",
            status: "Pending",
          },
        ]}
      />
    ));

    let table = screen.getByRole("grid", { name: "Gateways" });
    expect(within(table).getAllByRole("row")[1]?.textContent).toContain(
      "Alpha gateway",
    );

    await user.click(
      within(table).getByRole("button", { name: "Gateway name" }),
    );
    table = screen.getByRole("grid", { name: "Gateways" });
    expect(within(table).getAllByRole("row")[1]?.textContent).toContain(
      "Zulu gateway",
    );

    await user.type(
      screen.getByRole("textbox", {
        name: "Filter by name, cluster, status, or endpoint",
      }),
      "East cluster",
    );
    expect(screen.getByText("Alpha gateway")).toBeTruthy();
    expect(screen.queryByText("Zulu gateway")).toBeNull();
  });

  it("links gateway administration to provisioning", () => {
    renderPage(() => <AdminGatewaysPage gateways={previewGateways} />);

    expect(screen.getByRole("columnheader", { name: "Cluster" })).toBeTruthy();
    expect(screen.getByText("Local cluster")).toBeTruthy();
    expect(
      screen
        .getByRole("link", { name: "Provision gateway" })
        .getAttribute("href"),
    ).toBe("/admin/gateways/new");
  });
});
