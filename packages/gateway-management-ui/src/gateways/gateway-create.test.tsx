import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IntlProvider } from "react-intl";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { GatewayUiProvider } from "../gateway-ui-provider";
import { GatewayCreatePage } from "./gateway-create";

const { provisionGateway, navigate } = vi.hoisted(() => ({
  provisionGateway: vi.fn(),
  navigate: vi.fn(),
}));

const availablePlacement = {
  awsPublic: true,
  awsVpn: true,
  ibmPublic: true,
  ibmVpn: false,
  localKind: false,
} as const;

const operations = {
  changeGatewayAccessRole: vi.fn(),
  createOpenShellGatewayServiceAccount: vi.fn(),
  grantGatewayAccess: vi.fn(),
  listGatewayAccess: vi.fn(),
  revokeGatewayAccess: vi.fn(),
  searchGatewayDirectory: vi.fn(),
  deleteOpenShellGatewayServiceAccount: vi.fn(),
  findGatewayPlacements: vi.fn(),
  getGateway: vi.fn(),
  getGatewayPlacement: vi.fn(),
  getGatewayPlacementAvailability: vi.fn(),
  getGatewayPlacements: vi.fn(),
  getOpenShellGatewayServiceAccount: vi.fn(),
  listGateways: vi.fn(),
  listOpenShellGatewayServiceAccounts: vi.fn(),
  provisionGateway,
  removeGateway: vi.fn(),
  renameGateway: vi.fn(),
  revokeOpenShellGatewayServiceAccount: vi.fn(),
};

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <IntlProvider locale="en">
      <QueryClientProvider client={client}>
        <GatewayUiProvider
          gateways={operations}
          navigation={{
            collectionHref: "/",
            createHref: "/gateways/new",
            detailHref: (id: string) => `/gateways/${id}`,
            navigate,
          }}
        >
          <GatewayCreatePage />
        </GatewayUiProvider>
      </QueryClientProvider>
    </IntlProvider>,
  );
}

async function enterNameAndSelectPublic(
  user: ReturnType<typeof userEvent.setup>,
) {
  await user.type(
    screen.getByRole("textbox", { name: "Gateway name" }),
    "team-gateway",
  );
  await user.click(screen.getByRole("radio", { name: "Public" }));
}

function radio(name: string) {
  return screen.getByRole<HTMLInputElement>("radio", { name });
}

describe("GatewayCreatePage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    operations.getGatewayPlacementAvailability.mockResolvedValue(
      availablePlacement,
    );
    provisionGateway.mockResolvedValue({
      id: "gateway-1",
      name: "team-gateway",
    });
  });

  it("defaults public placement to IBM Cloud", async () => {
    const user = userEvent.setup();
    renderPage();
    await enterNameAndSelectPublic(user);

    expect(radio("IBM Cloud").checked).toBe(true);
    expect(radio("Amazon Web Services").checked).toBe(false);
    await user.click(screen.getByRole("button", { name: "Provision gateway" }));

    await waitFor(() => {
      expect(provisionGateway).toHaveBeenCalledWith({
        name: "team-gateway",
        placement: { network: "public", provider: "ibm" },
      });
    });
  });

  it("forces AWS for VPN and disables IBM Cloud", async () => {
    const user = userEvent.setup();
    renderPage();
    await user.type(
      screen.getByRole("textbox", { name: "Gateway name" }),
      "team-gateway",
    );
    await user.click(screen.getByRole("radio", { name: "VPN" }));

    expect(radio("Amazon Web Services").checked).toBe(true);
    expect(radio("IBM Cloud").disabled).toBe(true);
    await user.click(screen.getByRole("button", { name: "Provision gateway" }));

    await waitFor(() => {
      expect(provisionGateway).toHaveBeenCalledWith({
        name: "team-gateway",
        placement: { network: "vpn", provider: "aws" },
      });
    });
  });

  it("uses accessible radio groups with mutually exclusive choices", async () => {
    const user = userEvent.setup();
    renderPage();

    expect(
      screen.getByRole("radiogroup", { name: "Network access" }),
    ).toBeTruthy();
    await user.click(screen.getByRole("radio", { name: "Public" }));
    expect(radio("Public").checked).toBe(true);
    await user.click(screen.getByRole("radio", { name: "VPN" }));
    expect(radio("VPN").checked).toBe(true);
    expect(radio("Public").checked).toBe(false);
  });

  it("shows required placement and name errors without disabling submit", async () => {
    const user = userEvent.setup();
    renderPage();
    const submit = screen.getByRole("button", { name: "Provision gateway" });
    await user.click(submit);

    expect((submit as HTMLButtonElement).disabled).toBe(false);
    expect(screen.getAllByText("This field is required.")).toHaveLength(2);
    expect(provisionGateway).not.toHaveBeenCalled();
  });

  it("shows progress only while provisioning is pending", async () => {
    const user = userEvent.setup();
    let resolveProvision:
      ((gateway: { id: string; name: string }) => void) | undefined;
    provisionGateway.mockReturnValue(
      new Promise((resolve) => {
        resolveProvision = resolve;
      }),
    );
    renderPage();
    await enterNameAndSelectPublic(user);
    const submit = screen.getByRole("button", { name: "Provision gateway" });
    await user.click(submit);

    await waitFor(() => {
      expect((submit as HTMLButtonElement).disabled).toBe(true);
    });
    expect(submit.classList.contains("pf-m-progress")).toBe(true);
    resolveProvision?.({ id: "gateway-1", name: "team-gateway" });
    await waitFor(() => {
      expect(navigate).toHaveBeenCalledWith("/gateways/gateway-1");
    });
  });

  it("offers and defaults local-kind when it is the only available placement", async () => {
    operations.getGatewayPlacementAvailability.mockResolvedValue({
      awsPublic: false,
      awsVpn: false,
      ibmPublic: false,
      ibmVpn: false,
      localKind: true,
    });
    const user = userEvent.setup();
    renderPage();

    expect(
      (
        await screen.findByRole<HTMLInputElement>("radio", {
          name: "Use local-kind",
        })
      ).checked,
    ).toBe(true);
    expect(radio("Public").disabled).toBe(true);
    expect(radio("VPN").disabled).toBe(true);
    await user.type(
      screen.getByRole("textbox", { name: "Gateway name" }),
      "team-gateway",
    );
    await user.click(screen.getByRole("button", { name: "Provision gateway" }));
    await waitFor(() => {
      expect(provisionGateway).toHaveBeenCalledWith({
        name: "team-gateway",
        placement: { mode: "local-kind" },
      });
    });
  });

  it("hides local-kind when managed placement is also available", () => {
    operations.getGatewayPlacementAvailability.mockResolvedValue({
      ...availablePlacement,
      localKind: true,
    });
    renderPage();

    expect(screen.queryByRole("radio", { name: "Use local-kind" })).toBeNull();
    expect(radio("Public").checked).toBe(false);
    expect(radio("VPN").checked).toBe(false);
  });

  it("fails closed and retries when placement availability cannot be loaded", async () => {
    const user = userEvent.setup();
    operations.getGatewayPlacementAvailability
      .mockRejectedValueOnce(new Error("unavailable"))
      .mockResolvedValue(availablePlacement);
    renderPage();

    expect(
      await screen.findByText(
        "Placement availability could not be loaded. Cloud placement is disabled until availability can be verified.",
      ),
    ).toBeTruthy();
    expect(radio("Public").disabled).toBe(true);
    expect(radio("VPN").disabled).toBe(true);

    await user.click(screen.getByRole("button", { name: "Retry" }));

    await waitFor(() => {
      expect(radio("Public").disabled).toBe(false);
      expect(radio("VPN").disabled).toBe(false);
    });
    expect(operations.getGatewayPlacementAvailability).toHaveBeenCalledTimes(2);
  });

  it("uses network-specific availability and explains the backend reason", async () => {
    operations.getGatewayPlacementAvailability.mockResolvedValue({
      awsPublic: true,
      awsVpn: false,
      awsReason: "no-eligible-cluster",
      ibmPublic: false,
      ibmVpn: false,
      ibmReason: "no-eligible-cluster",
      localKind: false,
    });
    const user = userEvent.setup();
    renderPage();
    const vpn = radio("VPN");
    expect(vpn.disabled).toBe(true);
    expect(vpn.getAttribute("aria-describedby")).toBe(
      "placement-vpn-description",
    );
    expect(
      (
        await screen.findByText(
          "VPN is unavailable: no eligible managed cluster is connected.",
        )
      ).id,
    ).toBe("placement-vpn-description");
    await user.click(screen.getByRole("radio", { name: "Public" }));

    expect(radio("IBM Cloud").disabled).toBe(true);
    expect(radio("IBM Cloud").getAttribute("aria-describedby")).toBe(
      "provider-ibm-description",
    );
    expect(
      screen.getByText(
        "IBM Cloud is unavailable: no eligible managed cluster is connected.",
      ),
    ).toBeTruthy();
  });

  it("explains when neither managed nor local placement is available", async () => {
    operations.getGatewayPlacementAvailability.mockResolvedValue({
      awsPublic: false,
      awsVpn: false,
      ibmPublic: false,
      ibmVpn: false,
      localKind: false,
    });
    renderPage();

    expect(
      await screen.findByText("No managed placement is currently available."),
    ).toBeTruthy();
    expect(screen.queryByRole("radio", { name: "Use local-kind" })).toBeNull();
  });
});
