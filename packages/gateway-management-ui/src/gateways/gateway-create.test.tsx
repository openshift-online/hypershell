import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IntlProvider } from "react-intl";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { GatewayUiProvider } from "../gateway-ui-provider";
import { GatewayCreatePage } from "./gateway-create";

const { provisionGateway, navigate } = vi.hoisted(() => ({
  provisionGateway: vi.fn(),
  navigate: vi.fn(),
}));

function placementAvailability(
  available: readonly boolean[],
  localKind = false,
) {
  const selectors = [
    ["network/public", "provider/aws"],
    ["network/vpn", "provider/aws"],
    ["network/public", "provider/ibm"],
    ["network/vpn", "provider/ibm"],
  ] as const;
  return {
    localKind,
    placements: selectors.map((selector, index) => ({
      available: available[index] ?? false,
      ...(!available[index] ? { reason: "no-eligible-cluster" as const } : {}),
      selector,
    })),
  };
}

const availablePlacement = placementAvailability([true, true, true, false]);

// Mocks the batched availability endpoint by echoing every requested selector
// with availability driven by an explicit allow-list of selector keys. This
// lets version-bearing selectors (network/provider/gateway-version) be probed
// alongside the base network/provider pairs (GP-04, GP-11).
function availabilityFromAllowList(
  availableSelectorKeys: readonly string[],
  localKind = false,
) {
  const allowed = new Set(availableSelectorKeys);
  return (selectors: readonly (readonly string[])[]) =>
    Promise.resolve({
      localKind,
      placements: selectors.map((selector) => {
        const available = allowed.has(selector.join("|"));
        return {
          available,
          ...(available ? {} : { reason: "no-eligible-cluster" as const }),
          selector: [...selector],
        };
      }),
    });
}

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
  getGatewayProvisioningVersions: vi.fn(),
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
    operations.getGatewayProvisioningVersions.mockResolvedValue([]);
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
        placement: {
          selector: ["network/public", "provider/ibm"],
        },
      });
    });
  });

  it("defaults multiple available options to Public and IBM Cloud", async () => {
    renderPage();

    await waitFor(() => {
      expect(radio("Public").checked).toBe(true);
      expect(radio("IBM Cloud").checked).toBe(true);
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
        placement: { selector: ["network/vpn", "provider/aws"] },
      });
    });
  });

  it("allows IBM Cloud for VPN when its selector is available", async () => {
    operations.getGatewayPlacementAvailability.mockResolvedValue(
      placementAvailability([true, true, true, true]),
    );
    const user = userEvent.setup();
    renderPage();

    await user.type(
      screen.getByRole("textbox", { name: "Gateway name" }),
      "team-gateway",
    );
    await user.click(screen.getByRole("radio", { name: "VPN" }));

    expect(radio("IBM Cloud").disabled).toBe(false);
    expect(radio("IBM Cloud").checked).toBe(true);
    await user.click(screen.getByRole("button", { name: "Provision gateway" }));

    await waitFor(() => {
      expect(provisionGateway).toHaveBeenCalledWith({
        name: "team-gateway",
        placement: { selector: ["network/vpn", "provider/ibm"] },
      });
    });
  });

  it("loads all rendered placement selectors in one batch", async () => {
    renderPage();

    await waitFor(() => {
      expect(operations.getGatewayPlacementAvailability).toHaveBeenCalledTimes(
        1,
      );
    });
    expect(operations.getGatewayPlacementAvailability).toHaveBeenCalledWith(
      [
        ["network/public", "provider/aws"],
        ["network/vpn", "provider/aws"],
        ["network/public", "provider/ibm"],
        ["network/vpn", "provider/ibm"],
      ],
      expect.any(AbortSignal),
    );
  });

  it("selects the only available network and provider", async () => {
    operations.getGatewayPlacementAvailability.mockResolvedValue(
      placementAvailability([false, true, false, false]),
    );
    renderPage();

    await waitFor(() => {
      expect(radio("VPN").checked).toBe(true);
      expect(radio("Amazon Web Services").checked).toBe(true);
    });
  });

  it("selects the only available provider for public placement", async () => {
    operations.getGatewayPlacementAvailability.mockResolvedValue(
      placementAvailability([false, false, true, false]),
    );
    renderPage();

    await waitFor(() => {
      expect(radio("Public").checked).toBe(true);
      expect(radio("IBM Cloud").checked).toBe(true);
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
    expect(screen.getAllByText("This field is required.")).toHaveLength(1);
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
    operations.getGatewayPlacementAvailability.mockResolvedValue(
      placementAvailability([false, false, false, false], true),
    );
    const user = userEvent.setup();
    renderPage();

    expect(
      (
        await screen.findByRole<HTMLInputElement>("radio", {
          name: "Use local-kind",
        })
      ).checked,
    ).toBe(true);
    expect(screen.queryByRole("radio", { name: "Public" })).toBeNull();
    expect(screen.queryByRole("radio", { name: "VPN" })).toBeNull();
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

  it("hides local-kind when managed placement is also available", async () => {
    operations.getGatewayPlacementAvailability.mockResolvedValue(
      placementAvailability([true, true, true, false], true),
    );
    renderPage();

    expect(screen.queryByRole("radio", { name: "Use local-kind" })).toBeNull();
    await waitFor(() => {
      expect(radio("Public").checked).toBe(true);
    });
    expect(radio("VPN").checked).toBe(false);
  });

  it("fails closed when placement availability cannot be loaded", async () => {
    operations.getGatewayPlacementAvailability.mockRejectedValue(
      new Error("unavailable"),
    );
    renderPage();

    expect(
      await screen.findByText(
        "Placement availability could not be loaded. Cloud placement is disabled until availability can be verified.",
      ),
    ).toBeTruthy();
    expect(radio("Public").disabled).toBe(true);
    expect(radio("VPN").disabled).toBe(true);

    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
  });

  it("uses network-specific availability and explains the backend reason", async () => {
    operations.getGatewayPlacementAvailability.mockResolvedValue(
      placementAvailability([true, false, false, false]),
    );
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
    operations.getGatewayPlacementAvailability.mockResolvedValue(
      placementAvailability([false, false, false, false]),
    );
    renderPage();

    expect(
      await screen.findByText("No managed placement is currently available."),
    ).toBeTruthy();
    expect(screen.queryByRole("radio", { name: "Kind" })).toBeNull();
  });

  it("offers only advertised versions for the selected network and provider (GP-11)", async () => {
    operations.getGatewayProvisioningVersions.mockResolvedValue([
      "v0.1.1-rhaiv.3",
      "v0.1.2-rhaiv.7",
    ]);
    operations.getGatewayPlacementAvailability.mockImplementation(
      availabilityFromAllowList([
        "network/public|provider/aws",
        "network/public|provider/ibm",
        "network/public|provider/aws|gateway-version/v0.1.2-rhaiv.7",
        "network/public|provider/aws|gateway-version/v0.1.1-rhaiv.3",
      ]),
    );
    const user = userEvent.setup();
    renderPage();
    await user.type(
      screen.getByRole("textbox", { name: "Gateway name" }),
      "team-gateway",
    );
    await user.click(screen.getByRole("radio", { name: "Public" }));
    await user.click(
      screen.getByRole("radio", { name: "Amazon Web Services" }),
    );

    const select = await screen.findByRole("combobox", {
      name: "Provisioning version",
    });
    const options = within(select)
      .getAllByRole<HTMLOptionElement>("option")
      .map((option) => option.textContent);
    expect(options).toEqual([
      "Any version",
      "v0.1.2-rhaiv.7",
      "v0.1.1-rhaiv.3",
    ]);
  });

  it("adds the chosen version to the placement selector (GP-11)", async () => {
    operations.getGatewayProvisioningVersions.mockResolvedValue([
      "v0.1.2-rhaiv.7",
    ]);
    operations.getGatewayPlacementAvailability.mockImplementation(
      availabilityFromAllowList([
        "network/public|provider/aws",
        "network/public|provider/ibm",
        "network/public|provider/aws|gateway-version/v0.1.2-rhaiv.7",
      ]),
    );
    const user = userEvent.setup();
    renderPage();
    await user.type(
      screen.getByRole("textbox", { name: "Gateway name" }),
      "team-gateway",
    );
    await user.click(screen.getByRole("radio", { name: "Public" }));
    await user.click(
      screen.getByRole("radio", { name: "Amazon Web Services" }),
    );

    const select = await screen.findByRole("combobox", {
      name: "Provisioning version",
    });
    await user.selectOptions(select, "v0.1.2-rhaiv.7");
    await user.click(screen.getByRole("button", { name: "Provision gateway" }));

    await waitFor(() => {
      expect(provisionGateway).toHaveBeenCalledWith({
        name: "team-gateway",
        placement: {
          selector: [
            "network/public",
            "provider/aws",
            "gateway-version/v0.1.2-rhaiv.7",
          ],
        },
      });
    });
  });

  it("omits the version label when left at Any version (GP-11)", async () => {
    operations.getGatewayProvisioningVersions.mockResolvedValue([
      "v0.1.2-rhaiv.7",
    ]);
    operations.getGatewayPlacementAvailability.mockImplementation(
      availabilityFromAllowList([
        "network/public|provider/aws",
        "network/public|provider/ibm",
        "network/public|provider/ibm|gateway-version/v0.1.2-rhaiv.7",
      ]),
    );
    const user = userEvent.setup();
    renderPage();
    await enterNameAndSelectPublic(user);

    // IBM Cloud is the default public provider; leave the version at its default.
    await user.click(screen.getByRole("button", { name: "Provision gateway" }));

    await waitFor(() => {
      expect(provisionGateway).toHaveBeenCalledWith({
        name: "team-gateway",
        placement: { selector: ["network/public", "provider/ibm"] },
      });
    });
  });

  it("resets the version choice when it is no longer advertised (GP-11)", async () => {
    operations.getGatewayProvisioningVersions.mockResolvedValue([
      "v0.1.2-rhaiv.7",
    ]);
    operations.getGatewayPlacementAvailability.mockImplementation(
      availabilityFromAllowList([
        "network/public|provider/aws",
        "network/public|provider/ibm",
        "network/public|provider/aws|gateway-version/v0.1.2-rhaiv.7",
        // IBM Cloud does not advertise the version for Public.
      ]),
    );
    const user = userEvent.setup();
    renderPage();
    await user.type(
      screen.getByRole("textbox", { name: "Gateway name" }),
      "team-gateway",
    );
    await user.click(screen.getByRole("radio", { name: "Public" }));
    await user.click(
      screen.getByRole("radio", { name: "Amazon Web Services" }),
    );

    const select = await screen.findByRole<HTMLSelectElement>("combobox", {
      name: "Provisioning version",
    });
    await user.selectOptions(select, "v0.1.2-rhaiv.7");
    expect(select.value).toBe("v0.1.2-rhaiv.7");

    await user.click(screen.getByRole("radio", { name: "IBM Cloud" }));

    await waitFor(() => {
      expect(
        screen.getByRole<HTMLSelectElement>("combobox", {
          name: "Provisioning version",
        }).value,
      ).toBe("");
    });
  });
});
