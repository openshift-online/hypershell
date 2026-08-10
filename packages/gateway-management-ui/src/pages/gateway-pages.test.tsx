import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { IntlProvider } from "react-intl";
import { afterEach, beforeEach, vi } from "vitest";

import { GatewayUiProvider } from "../gateway-ui-provider";
import type { GatewayConnection } from "../gateways/gateway-connections";
import { GatewayPage, GatewaysPage } from "./gateway-pages";

const previewGateway: GatewayConnection = {
  clusterName: "Hub cluster",
  consoleUrl: "https://console.example.test",
  createdAt: "2026-08-10T14:30:00Z",
  endpoint: "https://gateway.example.test:443",
  id: "openshell-gateway-test",
  name: "openshell-gateway-test",
  oidcAudience: "openshell-cli",
  oidcClientId: "openshell-cli",
  oidcIssuer: "https://issuer.example.test/realms/openshell",
  status: "Ready",
};
const previewGateways = [previewGateway] as const;

const {
  deleteGatewayMock,
  getGatewayPlacementMock,
  getGatewayPlacementsMock,
  listGatewaysMock,
  navigateMock,
  renameGatewayMock,
} = vi.hoisted(() => ({
  deleteGatewayMock: vi.fn(),
  getGatewayPlacementMock: vi.fn(),
  getGatewayPlacementsMock: vi.fn(),
  listGatewaysMock: vi.fn(),
  navigateMock: vi.fn(),
  renameGatewayMock: vi.fn(),
}));

const gatewayOperations = {
  findGatewayPlacements: vi.fn(),
  getGateway: vi.fn(),
  getGatewayPlacement: getGatewayPlacementMock,
  getGatewayPlacements: getGatewayPlacementsMock,
  listGateways: listGatewaysMock,
  provisionGateway: vi.fn(),
  removeGateway: deleteGatewayMock,
  renameGateway: renameGatewayMock,
};

const navigation = {
  collectionHref: "/",
  createHref: "/gateways/new",
  detailHref: (gatewayId: string) => `/gateways/${gatewayId}`,
  navigate: navigateMock,
};

function gatewayResponse(id: string, name: string) {
  return {
    clusterId: "",
    createdAt: "2026-08-10T14:30:00Z",
    databaseId: "database-1",
    externalDns: "gateway.example.com",
    id,
    name,
    namespace: "openshell",
    phase: "",
    releaseId: "release-1",
    status: "Ready",
  };
}

function renderPage(Page: () => React.ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  return render(
    <IntlProvider locale="en">
      <QueryClientProvider client={queryClient}>
        <GatewayUiProvider gateways={gatewayOperations} navigation={navigation}>
          <Page />
        </GatewayUiProvider>
      </QueryClientProvider>
    </IntlProvider>,
  );
}

describe("gateway shell pages", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  beforeEach(() => {
    vi.clearAllMocks();
    deleteGatewayMock.mockResolvedValue(undefined);
    getGatewayPlacementMock.mockResolvedValue({
      id: "cluster-east",
      name: "Cluster East",
      provider: "AWS",
      region: "us-east-1",
      status: "Ready",
    });
    getGatewayPlacementsMock.mockResolvedValue([
      {
        id: "cluster-east",
        name: "Cluster East",
        provider: "AWS",
        region: "us-east-1",
        status: "Ready",
      },
    ]);
    navigateMock.mockResolvedValue(undefined);
  });

  it("renders gateway details with the shared connection actions", async () => {
    const user = userEvent.setup();
    renderPage(() => (
      <GatewayPage
        gatewayId="gateway-1"
        gateway={{
          clusterId: "",
          consoleUrl: "https://console.example.test",
          databaseId: "database-1",
          externalDns: "gateway.example.com",
          id: "gateway-1",
          name: "Team gateway",
          namespace: "openshell",
          oidcAudience: "openshell-cli",
          oidcClientId: "openshell-cli",
          oidcIssuer: "https://issuer.example.test/realms/openshell",
          phase: "",
          releaseId: "release-1",
          status: "Ready",
        }}
      />
    ));

    expect(
      screen.getByRole("heading", {
        level: 1,
        name: "Team gateway",
      }),
    ).toBeTruthy();
    expect(
      screen.getByDisplayValue("https://gateway.example.com:443"),
    ).toBeTruthy();
    expect(screen.getByText("release-1")).toBeTruthy();
    expect(screen.getByText("Cluster", { exact: true })).toBeTruthy();
    expect(screen.getByText("Hub cluster")).toBeTruthy();
    expect(
      screen.getByRole("link", {
        name: "Open console for Team gateway in a new tab",
      }),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Connect with the CLI" })).toBe(
      null,
    );
    expect(screen.getByText("CLI connection", { exact: true })).toBeTruthy();
    expect(
      screen.getByDisplayValue(/openshell gateway add --name 'Team gateway'/u),
    ).toBeTruthy();

    renameGatewayMock.mockResolvedValue(
      gatewayResponse("gateway-1", "Renamed team gateway"),
    );
    await user.click(screen.getByRole("button", { name: "Actions" }));
    await user.click(screen.getByRole("menuitem", { name: "Rename gateway" }));
    const renameDialog = screen.getByRole("dialog", {
      name: "Rename Team gateway",
    });
    const nameInput = within(renameDialog).getByRole("textbox", {
      name: "Gateway name",
    });
    await user.clear(nameInput);
    await user.type(nameInput, " Renamed team gateway ");
    await user.click(
      within(renameDialog).getByRole("button", { name: "Rename gateway" }),
    );
    await waitFor(() => {
      expect(renameGatewayMock).toHaveBeenCalledWith(
        "gateway-1",
        "Renamed team gateway",
      );
    });
    expect(
      await screen.findByText("Gateway renamed to Renamed team gateway"),
    ).toBeTruthy();
    expect(
      screen.queryByRole("dialog", { name: "Rename Team gateway" }),
    ).toBeNull();

    await user.click(screen.getByRole("button", { name: "Actions" }));
    await user.click(screen.getByRole("menuitem", { name: "Delete gateway" }));
    let dialog = screen.getByRole("dialog", {
      name: "Delete Team gateway?",
    });
    expect(
      within(dialog).getByText(
        "Deleting Team gateway will permanently remove the gateway. This action cannot be undone.",
      ),
    ).toBeTruthy();
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(deleteGatewayMock).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: "Actions" }));
    await user.click(screen.getByRole("menuitem", { name: "Delete gateway" }));
    dialog = screen.getByRole("dialog", { name: "Delete Team gateway?" });
    await user.click(
      within(dialog).getByRole("button", { name: "Delete gateway" }),
    );
    await waitFor(() => {
      expect(deleteGatewayMock).toHaveBeenCalledWith("gateway-1");
    });
    expect(
      screen.queryByRole("dialog", { name: "Delete Team gateway?" }),
    ).toBeNull();
  });

  it("renders a connection command without OIDC flags when OIDC is not configured", () => {
    renderPage(() => (
      <GatewayPage
        gateway={gatewayResponse("gateway-1", "Team gateway")}
        gatewayId="gateway-1"
      />
    ));

    expect(
      screen.getByDisplayValue("https://gateway.example.com:443"),
    ).toBeTruthy();
    expect(
      screen.queryByRole("link", {
        name: "Open console for Team gateway in a new tab",
      }),
    ).toBeNull();
    expect(
      screen.getByDisplayValue(
        "openshell gateway add --name 'Team gateway' https://gateway.example.com:443",
      ),
    ).toBeTruthy();
  });

  it("polls gateway details until its lifecycle reaches a terminal state", async () => {
    vi.useFakeTimers();
    gatewayOperations.getGateway
      .mockResolvedValueOnce({
        ...gatewayResponse("gateway-1", "Team gateway"),
        phase: "Provisioning",
        status: "",
      })
      .mockResolvedValue({
        ...gatewayResponse("gateway-1", "Team gateway"),
        phase: "Running",
        status: "",
      });

    const view = renderPage(() => <GatewayPage gatewayId="gateway-1" />);
    await act(async () => vi.advanceTimersByTimeAsync(0));

    expect(screen.getAllByText("Provisioning").length).toBeGreaterThan(0);
    expect(gatewayOperations.getGateway).toHaveBeenCalledOnce();

    await act(async () => vi.advanceTimersByTimeAsync(5_000));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await vi.advanceTimersByTimeAsync(1);
    });

    expect(gatewayOperations.getGateway).toHaveBeenCalledTimes(2);
    expect(screen.getAllByText("Running").length).toBeGreaterThan(0);

    await act(async () => vi.advanceTimersByTimeAsync(10_000));

    expect(gatewayOperations.getGateway).toHaveBeenCalledTimes(2);
    view.unmount();
  });

  it("resolves the managed-cluster name on gateway details", async () => {
    renderPage(() => (
      <GatewayPage
        gateway={{
          ...gatewayResponse("gateway-1", "Team gateway"),
          clusterId: "cluster-east",
        }}
        gatewayId="gateway-1"
      />
    ));

    expect(await screen.findByText("Cluster East")).toBeTruthy();
    expect(screen.queryByText("cluster-east")).toBeNull();
    expect(getGatewayPlacementMock).toHaveBeenCalledWith(
      "cluster-east",
      expect.any(AbortSignal),
    );
  });

  it("retries managed-cluster name resolution on gateway details", async () => {
    const user = userEvent.setup();
    getGatewayPlacementMock
      .mockRejectedValueOnce(new Error("unavailable"))
      .mockResolvedValue({
        id: "cluster-east",
        name: "Cluster East",
        provider: "AWS",
        region: "us-east-1",
        status: "Ready",
      });
    renderPage(() => (
      <GatewayPage
        gateway={{
          ...gatewayResponse("gateway-1", "Team gateway"),
          clusterId: "cluster-east",
        }}
        gatewayId="gateway-1"
      />
    ));

    await user.click(await screen.findByRole("button", { name: "Retry" }));

    expect(await screen.findByText("Cluster East")).toBeTruthy();
    expect(getGatewayPlacementMock).toHaveBeenCalledTimes(2);
  });

  it("shows an empty state when there are no gateways", () => {
    renderPage(() => <GatewaysPage gateways={[]} />);

    expect(
      screen.getByRole("heading", {
        level: 2,
        name: "No gateways",
      }),
    ).toBeTruthy();
  });

  it("shows and dismisses a deletion receipt after detail navigation", async () => {
    const user = userEvent.setup();
    renderPage(() => (
      <GatewaysPage deletedGatewayName="Team gateway" gateways={[]} />
    ));

    expect(
      await screen.findByText("Gateway Team gateway deleted"),
    ).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Close" }));
    expect(screen.queryByText("Gateway Team gateway deleted")).toBeNull();
  });

  it("loads the gateway list from the API", async () => {
    listGatewaysMock.mockResolvedValue({
      items: [
        gatewayResponse("openshell-gateway-test", "openshell-gateway-test"),
      ],
      page: 1,
      size: 20,
      total: 1,
    });

    renderPage(GatewaysPage);

    expect(screen.getByLabelText("Loading gateways")).toBeTruthy();
    expect(
      await screen.findByRole("link", { name: "openshell-gateway-test" }),
    ).toBeTruthy();
  });

  it("polls the current gateway page once until its lifecycles are terminal", async () => {
    vi.useFakeTimers();
    listGatewaysMock
      .mockResolvedValueOnce({
        items: [
          {
            ...gatewayResponse("gateway-1", "Team gateway"),
            phase: "Provisioning",
            status: "",
          },
        ],
        page: 1,
        size: 20,
        total: 1,
      })
      .mockResolvedValue({
        items: [
          {
            ...gatewayResponse("gateway-1", "Team gateway"),
            phase: "Running",
            status: "",
          },
        ],
        page: 1,
        size: 20,
        total: 1,
      });

    const view = renderPage(GatewaysPage);
    await act(async () => vi.advanceTimersByTimeAsync(0));

    expect(screen.getByText("Provisioning")).toBeTruthy();
    expect(listGatewaysMock).toHaveBeenCalledOnce();

    await act(async () => vi.advanceTimersByTimeAsync(5_000));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await vi.advanceTimersByTimeAsync(1);
    });

    expect(listGatewaysMock).toHaveBeenCalledTimes(2);
    expect(screen.getByText("Running")).toBeTruthy();

    await act(async () => vi.advanceTimersByTimeAsync(10_000));

    expect(listGatewaysMock).toHaveBeenCalledTimes(2);
    view.unmount();
  });

  it("normalizes search before invoking the gateway entry port", async () => {
    listGatewaysMock.mockResolvedValue({
      items: [gatewayResponse("gateway-1", "Team gateway")],
      page: 1,
      size: 20,
      total: 1,
    });

    renderPage(() => (
      <GatewaysPage
        collectionState={{
          page: 1,
          search: "  Team gateway  ",
          size: 20,
          sortDirection: "asc",
          sortField: "name",
        }}
      />
    ));

    await screen.findByRole("link", { name: "Team gateway" });
    expect(listGatewaysMock).toHaveBeenCalledWith(
      {
        page: 1,
        search: "Team gateway",
        size: 20,
        sortDirection: "asc",
        sortField: "name",
      },
      expect.any(AbortSignal),
    );
  });

  it("requests one new page when pagination changes", async () => {
    const user = userEvent.setup();
    listGatewaysMock.mockImplementation(
      (request: { page: number; size: number }) =>
        Promise.resolve({
          items: [
            gatewayResponse(
              `gateway-${String(request.page)}`,
              `Gateway page ${String(request.page)}`,
            ),
          ],
          page: request.page,
          size: request.size,
          total: 21,
        }),
    );
    renderPage(GatewaysPage);

    await screen.findByRole("link", { name: "Gateway page 1" });
    await user.click(screen.getByRole("button", { name: "Go to next page" }));
    await screen.findByRole("link", { name: "Gateway page 2" });

    expect(listGatewaysMock).toHaveBeenCalledTimes(2);
    expect(listGatewaysMock.mock.calls[1]?.[0]).toMatchObject({ page: 2 });
  });

  it("requests a selected page size from the first page", async () => {
    const user = userEvent.setup();
    listGatewaysMock.mockImplementation(
      (request: { page: number; size: number }) =>
        Promise.resolve({
          items: [
            gatewayResponse(
              `gateway-${String(request.size)}`,
              `Gateway page size ${String(request.size)}`,
            ),
          ],
          page: request.page,
          size: request.size,
          total: 51,
        }),
    );
    const { container } = renderPage(GatewaysPage);

    await screen.findByRole("link", { name: "Gateway page size 20" });
    const pageSizeToggle = container.querySelector(
      "#gateways-pagination-top-toggle",
    );
    expect(pageSizeToggle).toBeInstanceOf(HTMLButtonElement);
    await user.click(pageSizeToggle as HTMLButtonElement);
    await user.click(screen.getByRole("menuitem", { name: "50 per page" }));

    await waitFor(() => {
      expect(listGatewaysMock).toHaveBeenLastCalledWith(
        expect.objectContaining({ page: 1, size: 50 }),
        expect.any(AbortSignal),
      );
    });
    expect(
      await screen.findByRole("link", { name: "Gateway page size 50" }),
    ).toBeTruthy();
    await user.click(pageSizeToggle as HTMLButtonElement);
    expect(
      screen
        .getByRole("menuitem", { name: "50 per page" })
        .classList.contains("pf-m-selected"),
    ).toBe(true);
  });

  it("refreshes the gateway list", async () => {
    const user = userEvent.setup();
    listGatewaysMock.mockResolvedValue({
      items: [
        gatewayResponse("openshell-gateway-test", "openshell-gateway-test"),
      ],
      page: 1,
      size: 20,
      total: 1,
    });
    renderPage(GatewaysPage);

    await screen.findByRole("link", { name: "openshell-gateway-test" });
    const filter = screen.getByRole("textbox", {
      name: "Filter by name, cluster, status, or endpoint",
    });
    fireEvent.change(filter, { target: { value: "openshell" } });
    await waitFor(() => {
      expect(listGatewaysMock).toHaveBeenCalledTimes(2);
    });
    await user.click(screen.getByRole("button", { name: "Refresh gateways" }));

    await waitFor(() => {
      expect(listGatewaysMock).toHaveBeenCalledTimes(3);
    });
    expect(filter.getAttribute("value")).toBe("openshell");
  });

  it("debounces rapid gateway filters into one API request", async () => {
    const user = userEvent.setup();
    listGatewaysMock.mockResolvedValue({
      items: [gatewayResponse("gateway-1", "Team gateway")],
      page: 1,
      size: 20,
      total: 1,
    });
    renderPage(GatewaysPage);
    await screen.findByRole("link", { name: "Team gateway" });
    listGatewaysMock.mockClear();

    await user.type(
      screen.getByRole("textbox", {
        name: "Filter by name, cluster, status, or endpoint",
      }),
      "east",
    );

    expect(listGatewaysMock).not.toHaveBeenCalled();
    await waitFor(() => {
      expect(listGatewaysMock).toHaveBeenCalledWith(
        expect.objectContaining({ search: "east" }),
        expect.any(AbortSignal),
      );
    });
    expect(listGatewaysMock).toHaveBeenCalledOnce();
  });

  it("shows gateway API failures", async () => {
    listGatewaysMock.mockRejectedValue(new Error("unavailable"));

    renderPage(GatewaysPage);

    expect(
      await screen.findByText("Gateways could not be loaded"),
    ).toBeTruthy();
  });

  it("delegates authoritative sort and filter state to its host", async () => {
    const user = userEvent.setup();
    const onCollectionStateChange = vi.fn();
    renderPage(() => (
      <GatewaysPage
        collectionState={{
          page: 1,
          search: "",
          size: 20,
          sortDirection: "asc",
          sortField: "name",
        }}
        gateways={[
          {
            clusterName: "West cluster",
            consoleUrl: "https://console.example.test/zulu",
            endpoint: "https://zulu.example.test:443",
            id: "zulu",
            name: "Zulu gateway",
            oidcAudience: "openshell-cli",
            oidcClientId: "openshell-cli",
            oidcIssuer: "https://issuer.example.test",
            status: "Ready",
          },
          {
            clusterName: "East cluster",
            consoleUrl: "https://console.example.test/alpha",
            endpoint: "https://alpha.example.test:443",
            id: "alpha",
            name: "Alpha gateway",
            oidcAudience: "openshell-cli",
            oidcClientId: "openshell-cli",
            oidcIssuer: "https://issuer.example.test",
            status: "Pending",
          },
        ]}
        onCollectionStateChange={onCollectionStateChange}
      />
    ));

    const table = screen.getByRole("grid", { name: "OpenShell Gateways" });
    await user.click(
      within(table).getByRole("button", { name: "Gateway name" }),
    );
    expect(onCollectionStateChange).toHaveBeenCalledWith(
      {
        page: 1,
        search: "",
        size: 20,
        sortDirection: "desc",
        sortField: "name",
      },
      "sort",
    );

    fireEvent.change(
      screen.getByRole("textbox", {
        name: "Filter by name, cluster, status, or endpoint",
      }),
      { target: { value: "East cluster" } },
    );
    expect(onCollectionStateChange).toHaveBeenLastCalledWith(
      {
        page: 1,
        search: "East cluster",
        size: 20,
        sortDirection: "asc",
        sortField: "name",
      },
      "filter",
    );
  });

  it("links the gateway list to provisioning", () => {
    renderPage(() => <GatewaysPage gateways={previewGateways} />);

    expect(screen.getByRole("columnheader", { name: "Cluster" })).toBeTruthy();
    expect(screen.getByRole("columnheader", { name: "Created" })).toBeTruthy();
    expect(screen.getByText("Hub cluster")).toBeTruthy();
    expect(screen.getByText("Aug 10, 2026")).toBeTruthy();
    expect(
      screen
        .getByRole("link", { name: "Provision gateway" })
        .getAttribute("href"),
    ).toBe("/gateways/new");
  });

  it("sorts the gateway list by creation date", async () => {
    const user = userEvent.setup();
    const onCollectionStateChange = vi.fn();
    const collectionState = {
      page: 1,
      search: "",
      size: 20,
      sortDirection: "asc" as const,
      sortField: "name" as const,
    };
    renderPage(() => (
      <GatewaysPage
        collectionState={collectionState}
        gateways={previewGateways}
        onCollectionStateChange={onCollectionStateChange}
      />
    ));

    await user.click(screen.getByRole("button", { name: "Created" }));

    expect(onCollectionStateChange).toHaveBeenCalledWith(
      {
        ...collectionState,
        page: 1,
        sortDirection: "asc",
        sortField: "created",
      },
      "sort",
    );
  });

  it("renders Running with a semantic icon and no label chip", () => {
    renderPage(() => (
      <GatewaysPage gateways={[{ ...previewGateway, status: "Running" }]} />
    ));

    const statusCell = screen.getByText("Running").closest("td");
    expect(statusCell?.querySelector(".pf-v6-c-label")).toBeNull();
    expect(
      statusCell?.querySelector(".pf-v6-c-icon__content.pf-m-success"),
    ).toBeTruthy();
  });

  it("resolves managed-cluster names without displaying their identifiers", async () => {
    renderPage(() => (
      <GatewaysPage
        gateways={[
          {
            ...previewGateway,
            clusterId: "cluster-east",
            clusterName: "",
          },
        ]}
      />
    ));

    expect(await screen.findByText("Cluster East")).toBeTruthy();
    expect(screen.queryByText("cluster-east")).toBeNull();
    expect(getGatewayPlacementsMock).toHaveBeenCalledWith(
      ["cluster-east"],
      expect.any(AbortSignal),
    );
    expect(getGatewayPlacementMock).not.toHaveBeenCalled();
  });

  it("shows an unavailable state instead of a cluster identifier", async () => {
    getGatewayPlacementsMock.mockRejectedValue(new Error("unavailable"));
    renderPage(() => (
      <GatewaysPage
        gateways={[
          {
            ...previewGateway,
            clusterId: "cluster-east",
            clusterName: "",
          },
        ]}
      />
    ));

    expect(await screen.findByText("Not available")).toBeTruthy();
    expect(screen.queryByText("cluster-east")).toBeNull();
  });

  it("resolves a cold page of distinct cluster names with one batch request", async () => {
    const gateways = Array.from({ length: 20 }, (_, index) => ({
      ...gatewayResponse(
        `gateway-${String(index)}`,
        `Gateway ${String(index)}`,
      ),
      clusterId: `cluster-${String(index)}`,
    }));
    const clusterIds = gateways.map(({ clusterId }) => clusterId).sort();
    listGatewaysMock.mockResolvedValue({
      items: gateways,
      page: 1,
      size: 20,
      total: 20,
    });
    getGatewayPlacementsMock.mockResolvedValue(
      clusterIds.map((id) => ({
        id,
        name: `Name for ${id}`,
        provider: "AWS",
      })),
    );

    renderPage(() => <GatewaysPage />);

    expect(await screen.findByText("Name for cluster-0")).toBeTruthy();
    expect(listGatewaysMock).toHaveBeenCalledOnce();
    expect(getGatewayPlacementsMock).toHaveBeenCalledOnce();
    expect(getGatewayPlacementsMock).toHaveBeenCalledWith(
      clusterIds,
      expect.any(AbortSignal),
    );
    expect(getGatewayPlacementMock).not.toHaveBeenCalled();
  });

  it("deduplicates repeated cluster identifiers before batch resolution", async () => {
    renderPage(() => (
      <GatewaysPage
        gateways={[
          { ...previewGateway, clusterId: "cluster-east", clusterName: "" },
          {
            ...previewGateway,
            clusterId: "cluster-east",
            clusterName: "",
            id: "gateway-2",
            name: "gateway-2",
          },
        ]}
      />
    ));

    expect(await screen.findAllByText("Cluster East")).toHaveLength(2);
    expect(getGatewayPlacementsMock).toHaveBeenCalledOnce();
    expect(getGatewayPlacementsMock).toHaveBeenCalledWith(
      ["cluster-east"],
      expect.any(AbortSignal),
    );
  });

  it("provides console and CLI actions from the gateway row menu", async () => {
    const user = userEvent.setup();
    const writeText = vi.fn();
    Object.defineProperty(window.navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
    renderPage(() => <GatewaysPage gateways={previewGateways} />);

    expect(screen.queryByRole("link", { name: "View details" })).toBeNull();
    await user.click(
      screen.getByRole("button", {
        name: "Actions for openshell-gateway-test",
      }),
    );

    expect(
      screen
        .getByRole("menuitem", { name: "Open gateway console" })
        .getAttribute("href"),
    ).toBe(previewGateway.consoleUrl);
    await user.click(
      screen.getByRole("menuitem", {
        name: "Copy CLI connection command",
      }),
    );
    expect(writeText).toHaveBeenCalledWith(
      expect.stringContaining("openshell gateway add"),
    );
    expect(
      await screen.findByText(
        "CLI connection command for openshell-gateway-test copied",
      ),
    ).toBeTruthy();

    renameGatewayMock.mockResolvedValue(
      gatewayResponse("openshell-gateway-test", "Renamed gateway"),
    );
    await user.click(
      screen.getByRole("button", {
        name: "Actions for openshell-gateway-test",
      }),
    );
    await user.click(screen.getByRole("menuitem", { name: "Rename gateway" }));
    const renameDialog = screen.getByRole("dialog", {
      name: "Rename openshell-gateway-test",
    });
    const nameInput = within(renameDialog).getByRole("textbox", {
      name: "Gateway name",
    });
    await user.clear(nameInput);
    await user.type(nameInput, "Renamed gateway");
    await user.click(
      within(renameDialog).getByRole("button", { name: "Rename gateway" }),
    );
    await waitFor(() => {
      expect(renameGatewayMock).toHaveBeenCalledWith(
        "openshell-gateway-test",
        "Renamed gateway",
      );
    });
    expect(
      await screen.findByText("Gateway renamed to Renamed gateway"),
    ).toBeTruthy();

    await user.click(
      screen.getByRole("button", {
        name: "Actions for openshell-gateway-test",
      }),
    );
    await user.click(
      screen.getByRole("menuitem", {
        name: "Delete gateway",
      }),
    );
    const dialog = screen.getByRole("dialog", {
      name: "Delete openshell-gateway-test?",
    });
    await user.click(
      within(dialog).getByRole("button", { name: "Delete gateway" }),
    );
    await waitFor(() => {
      expect(deleteGatewayMock).toHaveBeenCalledWith("openshell-gateway-test");
    });
    expect(
      await screen.findByText("Gateway openshell-gateway-test deleted"),
    ).toBeTruthy();
  });

  it("keeps the confirmation open when gateway deletion fails", async () => {
    const user = userEvent.setup();
    deleteGatewayMock.mockRejectedValue(new Error("unavailable"));
    renderPage(() => <GatewaysPage gateways={previewGateways} />);

    await user.click(
      screen.getByRole("button", {
        name: "Actions for openshell-gateway-test",
      }),
    );
    await user.click(screen.getByRole("menuitem", { name: "Delete gateway" }));
    const dialog = screen.getByRole("dialog", {
      name: "Delete openshell-gateway-test?",
    });
    await user.click(
      within(dialog).getByRole("button", { name: "Delete gateway" }),
    );

    expect(
      await within(dialog).findByText("Gateway could not be deleted"),
    ).toBeTruthy();
    expect(
      within(dialog).getByText("No changes were made. Try again."),
    ).toBeTruthy();
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(
      screen.queryByRole("dialog", {
        name: "Delete openshell-gateway-test?",
      }),
    ).toBeNull();
  });

  it("validates and recovers from a failed gateway rename", async () => {
    const user = userEvent.setup();
    renameGatewayMock.mockRejectedValue(new Error("conflict"));
    renderPage(() => <GatewaysPage gateways={previewGateways} />);

    await user.click(
      screen.getByRole("button", {
        name: "Actions for openshell-gateway-test",
      }),
    );
    await user.click(screen.getByRole("menuitem", { name: "Rename gateway" }));
    const dialog = screen.getByRole("dialog", {
      name: "Rename openshell-gateway-test",
    });
    const nameInput = within(dialog).getByRole("textbox", {
      name: "Gateway name",
    });
    await user.clear(nameInput);
    await user.click(
      within(dialog).getByRole("button", { name: "Rename gateway" }),
    );
    expect(
      await within(dialog).findByText("This field is required."),
    ).toBeTruthy();
    expect(renameGatewayMock).not.toHaveBeenCalled();

    await user.type(nameInput, "Existing gateway");
    await user.click(
      within(dialog).getByRole("button", { name: "Rename gateway" }),
    );
    expect(
      await within(dialog).findByText("Gateway could not be renamed"),
    ).toBeTruthy();
    expect(
      within(dialog).getByText(
        "No changes were made. Choose a different name or try again.",
      ),
    ).toBeTruthy();

    await user.clear(nameInput);
    await user.type(nameInput, "Another gateway");
    expect(
      within(dialog).queryByText("Gateway could not be renamed"),
    ).toBeNull();
  });

  it("reports and dismisses a failed CLI command copy", async () => {
    const user = userEvent.setup();
    Object.defineProperty(window.navigator, "clipboard", {
      configurable: true,
      value: { writeText: vi.fn().mockRejectedValue(new Error("denied")) },
    });
    renderPage(() => <GatewaysPage gateways={previewGateways} />);

    await user.click(
      screen.getByRole("button", {
        name: "Actions for openshell-gateway-test",
      }),
    );
    await user.click(
      screen.getByRole("menuitem", {
        name: "Copy CLI connection command",
      }),
    );

    expect(
      await screen.findByText(
        "CLI connection command for openshell-gateway-test could not be copied",
      ),
    ).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Close" }));
    expect(
      screen.queryByText(
        "CLI connection command for openshell-gateway-test could not be copied",
      ),
    ).toBeNull();
  });
});
