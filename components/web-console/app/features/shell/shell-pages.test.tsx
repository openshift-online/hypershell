import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { IntlProvider } from "react-intl";
import { MemoryRouter } from "react-router";
import { beforeEach, vi } from "vitest";

import { englishMessages } from "../../i18n/catalog";
import { previewGateways } from "../gateways/gateway-connections";
import type * as GatewayData from "../gateways/gateway-data";
import { GatewayPage, GatewaysPage } from "./shell-pages";

const { deleteGatewayMock, listGatewayConnectionsMock, renameGatewayMock } =
  vi.hoisted(() => ({
    deleteGatewayMock: vi.fn(),
    listGatewayConnectionsMock: vi.fn(),
    renameGatewayMock: vi.fn(),
  }));

vi.mock("../gateways/gateway-data", async (importOriginal) => ({
  ...(await importOriginal<typeof GatewayData>()),
  deleteGateway: deleteGatewayMock,
  listGatewayConnections: listGatewayConnectionsMock,
  renameGateway: renameGatewayMock,
}));

function gatewayResponse(id: string, name: string) {
  return {
    cluster_id: "",
    created_at: null,
    database_id: "database-1",
    external_dns: "gateway.example.com",
    fleet_id: "",
    href: `/api/hypershell/v1/gateways/${id}`,
    id,
    kind: "Gateway" as const,
    name,
    namespace: "openshell",
    phase: "",
    release_id: "release-1",
    service_type: "",
    status: "Ready",
    tls_mode: "",
    updated_at: null,
  };
}

function renderPage(
  Page: () => React.ReactNode,
  initialEntries: React.ComponentProps<
    typeof MemoryRouter
  >["initialEntries"] = ["/"],
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  return render(
    <IntlProvider locale="en" messages={englishMessages}>
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={initialEntries}>
          <Page />
        </MemoryRouter>
      </QueryClientProvider>
    </IntlProvider>,
  );
}

describe("gateway shell pages", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    deleteGatewayMock.mockResolvedValue(undefined);
  });

  it("renders gateway details with the shared connection actions", async () => {
    const user = userEvent.setup();
    renderPage(() => (
      <GatewayPage
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
    expect(
      screen.getByDisplayValue("https://gateway.example.com:443"),
    ).toBeTruthy();
    expect(screen.getByText("release-1")).toBeTruthy();
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
    renderPage(
      () => <GatewaysPage gateways={[]} />,
      [
        {
          pathname: "/",
          state: { deletedGatewayName: "Team gateway" },
        },
      ],
    );

    expect(
      await screen.findByText("Gateway Team gateway deleted"),
    ).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Close" }));
    expect(screen.queryByText("Gateway Team gateway deleted")).toBeNull();
  });

  it("loads the gateway list from the API", async () => {
    listGatewayConnectionsMock.mockResolvedValue(previewGateways);

    renderPage(GatewaysPage);

    expect(screen.getByLabelText("Loading gateways")).toBeTruthy();
    expect(
      await screen.findByRole("link", { name: "openshell-gateway-test" }),
    ).toBeTruthy();
  });

  it("refreshes the gateway list", async () => {
    const user = userEvent.setup();
    listGatewayConnectionsMock.mockResolvedValue(previewGateways);
    renderPage(GatewaysPage);

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

  it("shows gateway API failures", async () => {
    listGatewayConnectionsMock.mockRejectedValue(new Error("unavailable"));

    renderPage(GatewaysPage);

    expect(
      await screen.findByText("Gateways could not be loaded"),
    ).toBeTruthy();
  });

  it("sorts and filters the gateway list", async () => {
    const user = userEvent.setup();
    renderPage(() => (
      <GatewaysPage
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

    let table = screen.getByRole("grid", { name: "OpenShell Gateways" });
    expect(within(table).getAllByRole("row")[1]?.textContent).toContain(
      "Alpha gateway",
    );

    await user.click(
      within(table).getByRole("button", { name: "Gateway name" }),
    );
    table = screen.getByRole("grid", { name: "OpenShell Gateways" });
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

  it("links the gateway list to provisioning", () => {
    renderPage(() => <GatewaysPage gateways={previewGateways} />);

    expect(screen.getByRole("columnheader", { name: "Cluster" })).toBeTruthy();
    expect(screen.getByText("Local cluster")).toBeTruthy();
    expect(
      screen
        .getByRole("link", { name: "Provision gateway" })
        .getAttribute("href"),
    ).toBe("/gateways/new");
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
    ).toBe(previewGateways[0]?.consoleUrl);
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
