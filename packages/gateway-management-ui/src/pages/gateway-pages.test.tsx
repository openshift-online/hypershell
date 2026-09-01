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
  phase: "Running",
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
  createOpenShellGatewayServiceAccount: vi.fn(),
  deleteOpenShellGatewayServiceAccount: vi.fn(),
  findGatewayPlacements: vi.fn(),
  findGatewayProfiles: vi.fn(),
  getGateway: vi.fn(),
  getGatewayPlacement: getGatewayPlacementMock,
  getGatewayPlacements: getGatewayPlacementsMock,
  getOpenShellGatewayServiceAccount: vi.fn(),
  listGateways: listGatewaysMock,
  listOpenShellGatewayServiceAccounts: vi.fn(),
  provisionGateway: vi.fn(),
  removeGateway: deleteGatewayMock,
  renameGateway: renameGatewayMock,
  revokeOpenShellGatewayServiceAccount: vi.fn(),
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
    phase: "Running",
    releaseId: "release-1",
    status: "Ready",
  };
}

const serviceAccountCreateResult = {
  credential: {
    accessTokenLifetimeSeconds: 300,
    audience: "gateway-client",
    clientId: "service-client",
    clientSecret: "one-time-client-secret",
    gatewayEndpoint: "gateway.example.test:443",
    gatewayName: "Team gateway",
    issuer: "https://issuer.example.test/realms/openshell",
    tokenEndpoint:
      "https://issuer.example.test/realms/openshell/protocol/openid-connect/token",
  },
  serviceAccount: {
    clientId: "service-client",
    createdAt: "2026-08-21T12:00:00Z",
    createdByUserId: "user-1",
    expiresAt: "2026-11-19T12:00:00Z",
    gatewayId: "gateway-1",
    id: "account-1",
    name: "release-bot",
    role: "openshell-user" as const,
    status: "ready" as const,
    subject: "service-subject",
    updatedAt: "2026-08-21T12:00:00Z",
  },
};

function mockServiceAccountCollection() {
  gatewayOperations.listOpenShellGatewayServiceAccounts.mockResolvedValue({
    capabilities: {
      allowedRoles: ["openshell-user"],
      canCreate: true,
      canManageAll: false,
      expirationPolicy: {
        defaultSeconds: 7_776_000,
        maximumSeconds: 31_536_000,
        minimumSeconds: 3_600,
      },
    },
    items: [],
    page: 1,
    size: 20,
    total: 0,
  });
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
          phase: "Running",
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
      screen.getByRole("link", {
        name: "Open console for Team gateway in a new tab",
      }),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Connect with the CLI" })).toBe(
      null,
    );
    // Connection is the default tab and leads with the login command.
    expect(
      screen.getByRole("tab", { name: "Connection", selected: true }),
    ).toBeTruthy();
    expect(
      screen.getByText(/openshell gateway add/, { selector: "code" }),
    ).toBeTruthy();

    // Operational configuration lives behind the Details tab.
    await user.click(screen.getByRole("tab", { name: "Details" }));
    expect(
      screen.getByDisplayValue("https://gateway.example.com:443"),
    ).toBeTruthy();
    expect(screen.getByText("release-1")).toBeTruthy();
    expect(screen.getByText("Cluster", { exact: true })).toBeTruthy();
    expect(screen.getByText("Hub cluster")).toBeTruthy();
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
      screen.queryByRole("link", {
        name: "Open console for Team gateway in a new tab",
      }),
    ).toBeNull();
    const cliCmd = screen.getByText(/openshell gateway add/, {
      selector: "code",
    });
    expect(cliCmd).toBeTruthy();
    expect(cliCmd.textContent).not.toContain("--oidc-");
  });

  it("walks through gateway connection with provider and sandbox commands", () => {
    renderPage(() => (
      <GatewayPage
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
          phase: "Running",
          releaseId: "release-1",
          status: "Ready",
        }}
        gatewayId="gateway-1"
      />
    ));

    // The preamble is consolidated into a single "One-time setup" block, with
    // the re-runnable sandbox command in its own block.
    expect(
      screen.getByRole("heading", { level: 2, name: "One-time setup" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("heading", { level: 2, name: "Create a sandbox" }),
    ).toBeTruthy();

    // The setup block carries login, provider, and inference selection together.
    const setupCommand = screen.getByText(/openshell gateway add/, {
      selector: "code",
    });
    expect(setupCommand.textContent).toContain("--from-gcloud-adc");
    expect(setupCommand.textContent).toContain("openshell inference set");
    expect(
      screen.getByText(/openshell sandbox create/, { selector: "code" }),
    ).toBeTruthy();
  });

  it("reports gateway login as unavailable without an endpoint", () => {
    renderPage(() => (
      <GatewayPage
        gateway={{
          ...gatewayResponse("gateway-1", "Team gateway"),
          externalDns: "",
        }}
        gatewayId="gateway-1"
      />
    ));

    expect(
      screen.getByRole("status", {
        name: "This gateway is still provisioning. Its connection command becomes available once the gateway is running.",
      }),
    ).toBeTruthy();
    expect(screen.queryByDisplayValue(/openshell gateway add/u)).toBeNull();
  });

  it("withholds the connection command while the gateway is provisioning", () => {
    // The route address may already be published while the gateway is still
    // provisioning; the command must not surface until phase is Running.
    renderPage(() => (
      <GatewayPage
        gateway={{
          ...gatewayResponse("gateway-1", "Team gateway"),
          phase: "Provisioning",
          status: "",
        }}
        gatewayId="gateway-1"
      />
    ));

    expect(
      screen.getByRole("status", {
        name: "This gateway is still provisioning. Its connection command becomes available once the gateway is running.",
      }),
    ).toBeTruthy();
    expect(
      screen.queryByText(/openshell gateway add/u, { selector: "code" }),
    ).toBeNull();
  });

  it("withholds the console button until the gateway is ready to connect", () => {
    // The console button gates on the same readiness signal as the connection
    // command, so while the gateway is still provisioning neither the enabled
    // link nor the disabled placeholder button is rendered.
    renderPage(() => (
      <GatewayPage
        gateway={{
          ...gatewayResponse("gateway-1", "Team gateway"),
          consoleUrl: "https://console.example.test",
          phase: "Provisioning",
          status: "",
        }}
        gatewayId="gateway-1"
      />
    ));

    expect(
      screen.queryByRole("link", {
        name: "Open console for Team gateway in a new tab",
      }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", {
        name: "Open console for Team gateway in a new tab",
      }),
    ).toBeNull();
  });

  it("shows a disabled console button while the console is still provisioning", () => {
    // Once the gateway is ready to connect the button appears, but until the
    // control plane publishes the console address it stays disabled (with a
    // "Provisioning console..." tooltip) rather than hidden, so its eventual
    // availability is discoverable.
    renderPage(() => (
      <GatewayPage
        gateway={{
          ...gatewayResponse("gateway-1", "Team gateway"),
          consoleUrl: undefined,
          phase: "Running",
          status: "Ready",
        }}
        gatewayId="gateway-1"
      />
    ));

    const consoleButton = screen.getByRole("button", {
      name: "Open console for Team gateway in a new tab",
    });
    expect(consoleButton.getAttribute("aria-disabled")).toBe("true");
    // It is a disabled button, not an actionable link, so it has no destination.
    expect(
      screen.queryByRole("link", {
        name: "Open console for Team gateway in a new tab",
      }),
    ).toBeNull();
  });

  it("encodes service-account and detail tabs through its host", async () => {
    const user = userEvent.setup();
    const onTabChange = vi.fn();
    renderPage(() => (
      <GatewayPage
        activeTab="connection"
        gateway={gatewayResponse("gateway-1", "Team gateway")}
        gatewayId="gateway-1"
        onTabChange={onTabChange}
      />
    ));

    await user.click(screen.getByRole("tab", { name: "Service accounts" }));
    expect(onTabChange).toHaveBeenCalledWith("service-accounts");

    await user.click(
      screen.getByRole("button", { name: "Create or manage service accounts" }),
    );
    expect(onTabChange).toHaveBeenLastCalledWith("service-accounts");

    await user.click(screen.getByRole("tab", { name: "Details" }));
    expect(onTabChange).toHaveBeenCalledWith("details");
  });

  it("blocks a tab switch that would discard an unacknowledged secret", async () => {
    mockServiceAccountCollection();
    gatewayOperations.createOpenShellGatewayServiceAccount.mockResolvedValue(
      serviceAccountCreateResult,
    );
    const user = userEvent.setup();
    renderPage(() => (
      <GatewayPage
        gateway={gatewayResponse("gateway-1", "Team gateway")}
        gatewayId="gateway-1"
      />
    ));

    await user.click(screen.getByRole("tab", { name: "Service accounts" }));
    await user.click(
      await screen.findByRole("button", { name: "Create service account" }),
    );
    await user.type(
      screen.getByRole("textbox", { name: "Service account name" }),
      "release-bot",
    );
    await user.click(
      screen.getByRole("button", { name: "Create service account" }),
    );
    const secret =
      await screen.findByLabelText<HTMLInputElement>("Client secret");
    expect(secret.value).toBe("one-time-client-secret");

    // Switching tabs would unmount the dialog and discard the secret, so the
    // loss confirmation must intercept it instead of losing it silently.
    await user.click(
      screen.getByRole("tab", { hidden: true, name: "Connection" }),
    );
    expect(
      screen.getByRole("dialog", {
        name: "Leave without saving the client secret?",
      }),
    ).toBeTruthy();

    // Returning keeps the secret available and stays on the tab.
    await user.click(screen.getByRole("button", { name: "Return to setup" }));
    expect(
      screen.getByRole("dialog", { name: "Set up release-bot" }),
    ).toBeTruthy();
    expect(screen.getByLabelText<HTMLInputElement>("Client secret").value).toBe(
      "one-time-client-secret",
    );
  });

  it("discards the one-time secret only after confirming the tab switch", async () => {
    mockServiceAccountCollection();
    gatewayOperations.createOpenShellGatewayServiceAccount.mockResolvedValue(
      serviceAccountCreateResult,
    );
    const user = userEvent.setup();
    renderPage(() => (
      <GatewayPage
        gateway={gatewayResponse("gateway-1", "Team gateway")}
        gatewayId="gateway-1"
      />
    ));

    await user.click(screen.getByRole("tab", { name: "Service accounts" }));
    await user.click(
      await screen.findByRole("button", { name: "Create service account" }),
    );
    await user.type(
      screen.getByRole("textbox", { name: "Service account name" }),
      "release-bot",
    );
    await user.click(
      screen.getByRole("button", { name: "Create service account" }),
    );
    await screen.findByLabelText<HTMLInputElement>("Client secret");

    await user.click(
      screen.getByRole("tab", { hidden: true, name: "Connection" }),
    );
    await user.click(screen.getByRole("button", { name: "Leave setup" }));

    expect(screen.queryByLabelText("Client secret")).toBeNull();
    expect(
      screen.queryByRole("dialog", { name: "Set up release-bot" }),
    ).toBeNull();
    expect(
      screen.getByRole("button", { name: "Create or manage service accounts" }),
    ).toBeTruthy();
  });

  it("blocks a tab switch while a create request is still pending", async () => {
    mockServiceAccountCollection();
    let resolveCreate: (
      value: typeof serviceAccountCreateResult,
    ) => void = () => undefined;
    gatewayOperations.createOpenShellGatewayServiceAccount.mockReturnValue(
      new Promise<typeof serviceAccountCreateResult>((resolve) => {
        resolveCreate = resolve;
      }),
    );
    const user = userEvent.setup();
    renderPage(() => (
      <GatewayPage
        gateway={gatewayResponse("gateway-1", "Team gateway")}
        gatewayId="gateway-1"
      />
    ));

    await user.click(screen.getByRole("tab", { name: "Service accounts" }));
    await user.click(
      await screen.findByRole("button", { name: "Create service account" }),
    );
    await user.type(
      screen.getByRole("textbox", { name: "Service account name" }),
      "release-bot",
    );
    await user.click(
      screen.getByRole("button", { name: "Create service account" }),
    );

    // The request has not resolved, so navigating away could discard the
    // eventual one-time secret; the guard intercepts the switch.
    await user.click(
      screen.getByRole("tab", { hidden: true, name: "Connection" }),
    );
    expect(
      screen.getByRole("dialog", {
        name: "Leave without saving the client secret?",
      }),
    ).toBeTruthy();

    resolveCreate(serviceAccountCreateResult);
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
        consoleUrl: "https://console.example.com",
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

  it("enables the console button once the console address arrives without a refresh", async () => {
    vi.useFakeTimers();
    // Console polling is bounded by the gateway's createdAt, so pin the clock
    // just inside the console-ready window to exercise the still-provisioning
    // path (a gateway created moments ago whose console is about to arrive).
    vi.setSystemTime(new Date("2026-08-10T14:31:00Z"));
    // A routed gateway reaches Running before its console pod can serve, so the
    // first fetch has no console URL and the button renders disabled. The control
    // plane publishes it a moment later; the page must keep polling and enable
    // the button on its own.
    gatewayOperations.getGateway
      .mockResolvedValueOnce({
        ...gatewayResponse("gateway-1", "Team gateway"),
        consoleUrl: undefined,
        phase: "Running",
        status: "Ready",
      })
      .mockResolvedValue({
        ...gatewayResponse("gateway-1", "Team gateway"),
        consoleUrl: "https://console.example.com",
        phase: "Running",
        status: "Ready",
      });

    const view = renderPage(() => <GatewayPage gatewayId="gateway-1" />);
    await act(async () => vi.advanceTimersByTimeAsync(0));

    expect(gatewayOperations.getGateway).toHaveBeenCalledOnce();
    // Disabled placeholder button, not yet an actionable link.
    expect(
      screen
        .getByRole("button", {
          name: "Open console for Team gateway in a new tab",
        })
        .getAttribute("aria-disabled"),
    ).toBe("true");
    expect(
      screen.queryByRole("link", {
        name: "Open console for Team gateway in a new tab",
      }),
    ).toBeNull();

    await act(async () => vi.advanceTimersByTimeAsync(5_000));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await vi.advanceTimersByTimeAsync(1);
    });

    expect(gatewayOperations.getGateway).toHaveBeenCalledTimes(2);
    // The console URL is published, so the button becomes an actionable link.
    expect(
      screen.getByRole("link", {
        name: "Open console for Team gateway in a new tab",
      }),
    ).toBeTruthy();

    // The console URL is published, so polling stops.
    await act(async () => vi.advanceTimersByTimeAsync(10_000));
    expect(gatewayOperations.getGateway).toHaveBeenCalledTimes(2);
    view.unmount();
  });

  it("resolves the managed-cluster name on gateway details", async () => {
    const user = userEvent.setup();
    renderPage(() => (
      <GatewayPage
        gateway={{
          ...gatewayResponse("gateway-1", "Team gateway"),
          clusterId: "cluster-east",
        }}
        gatewayId="gateway-1"
      />
    ));

    await user.click(screen.getByRole("tab", { name: "Details" }));
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

    await user.click(screen.getByRole("tab", { name: "Details" }));
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
            consoleUrl: "https://console.example.com",
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

    await user.click(within(table).getByRole("button", { name: "Created by" }));
    expect(onCollectionStateChange).toHaveBeenLastCalledWith(
      {
        page: 1,
        search: "",
        size: 20,
        sortDirection: "asc",
        sortField: "owner",
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

  it("shows the active sandbox count with a not-available fallback when unset", () => {
    renderPage(() => (
      <GatewaysPage
        gateways={[
          {
            ...previewGateway,
            activeSandboxCount: 3,
            id: "gw-busy",
            name: "gw-busy",
          },
          // activeSandboxCount deliberately omitted: an unset count must render
          // the not-available fallback rather than a misleading zero.
          { ...previewGateway, id: "gw-idle", name: "gw-idle" },
        ]}
      />
    ));

    expect(
      screen.getByRole("columnheader", { name: "Active sandboxes" }),
    ).toBeTruthy();

    const busyRow = screen.getByRole("row", { name: /gw-busy/u });
    expect(within(busyRow).getByText("3")).toBeTruthy();

    const idleRow = screen.getByRole("row", { name: /gw-idle/u });
    const sandboxCell = idleRow.querySelector(
      "[data-label='Active sandboxes']",
    );
    expect(sandboxCell?.textContent).toBe("Not available");
  });

  it("shows the gateway owner with a not-available fallback when unset", () => {
    renderPage(() => (
      <GatewaysPage
        gateways={[
          {
            ...previewGateway,
            createdBy: "alice@example.com",
            id: "gw-owned",
            name: "gw-owned",
          },
          { ...previewGateway, id: "gw-unowned", name: "gw-unowned" },
        ]}
      />
    ));

    expect(
      screen.getByRole("columnheader", { name: "Created by" }),
    ).toBeTruthy();

    const ownedRow = screen.getByRole("row", { name: /gw-owned/u });
    expect(within(ownedRow).getByText("alice@example.com")).toBeTruthy();

    const unownedRow = screen.getByRole("row", { name: /gw-unowned/u });
    const ownerCell = unownedRow.querySelector("[data-label='Created by']");
    expect(ownerCell?.textContent).toBe("Not available");
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

    expect(await screen.findAllByText("Not available")).toBeTruthy();
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

    const consoleMenuItem = screen.getByRole("menuitem", {
      name: "Open gateway console",
    });
    expect(consoleMenuItem.getAttribute("href")).toBe(
      previewGateway.consoleUrl,
    );
    expect(consoleMenuItem.getAttribute("target")).toBe("_blank");
    expect(
      consoleMenuItem.querySelector(".pf-v6-c-menu__item-external-icon svg"),
    ).toBeTruthy();
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

  it("warns about active sandboxes without blocking deletion", async () => {
    const user = userEvent.setup();
    renderPage(() => (
      <GatewaysPage gateways={[{ ...previewGateway, activeSandboxCount: 3 }]} />
    ));

    await user.click(
      screen.getByRole("button", {
        name: "Actions for openshell-gateway-test",
      }),
    );
    await user.click(screen.getByRole("menuitem", { name: "Delete gateway" }));
    const dialog = screen.getByRole("dialog", {
      name: "Delete openshell-gateway-test?",
    });
    expect(
      within(dialog).getByText(
        "This gateway has 3 active sandboxes that will be disrupted by deletion.",
      ),
    ).toBeTruthy();

    // The warning is advisory: the delete button is still actionable.
    await user.click(
      within(dialog).getByRole("button", { name: "Delete gateway" }),
    );
    await waitFor(() => {
      expect(deleteGatewayMock).toHaveBeenCalledWith("openshell-gateway-test");
    });
  });

  it("omits the sandbox warning when no sandboxes are active", async () => {
    const user = userEvent.setup();
    renderPage(() => (
      <GatewaysPage gateways={[{ ...previewGateway, activeSandboxCount: 0 }]} />
    ));

    await user.click(
      screen.getByRole("button", {
        name: "Actions for openshell-gateway-test",
      }),
    );
    await user.click(screen.getByRole("menuitem", { name: "Delete gateway" }));
    const dialog = screen.getByRole("dialog", {
      name: "Delete openshell-gateway-test?",
    });
    expect(within(dialog).queryByText(/active sandbox/u)).toBeNull();
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
