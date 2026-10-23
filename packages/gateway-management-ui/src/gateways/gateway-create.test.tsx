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

function placementAvailability(
  available: readonly boolean[],
  localKind = false,
) {
  const selectors = [
    ["network/public", "provider/aws"],
    ["network/vpn", "provider/aws"],
    ["network/public", "provider/ibm"],
    ["network/vpn", "provider/ibm"],
    ["network/public", "provider/aws", "runtime/kata"],
    ["network/vpn", "provider/aws", "runtime/kata"],
    ["network/public", "provider/ibm", "runtime/kata"],
    ["network/vpn", "provider/ibm", "runtime/kata"],
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

const availablePlacement = placementAvailability([
  true,
  true,
  true,
  false,
  false,
  false,
  true,
  false,
]);

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
  updateOpenShellGatewayServiceAccountRole: vi.fn(),
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
  await user.click(screen.getByRole("radio", { name: "No" }));
}

function radio(name: string) {
  const accessibleName =
    name === "IBM Cloud"
      ? "IBM (default)"
      : name === "Amazon Web Services"
        ? "AWS (For workloads that rely heavily on AWS services or data)"
        : name === "Yes"
          ? "Yes (Requires Red Hat VPN)"
          : name;
  return screen.getByRole<HTMLInputElement>("radio", { name: accessibleName });
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

  // Verifies: WEB-PLC-03.01, WEB-PLC-01.03
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
          excludedSelector: ["runtime/kata"],
          selector: ["network/public", "provider/ibm"],
        },
      });
    });
  });

  it("submits an explicit Kata request when Kata is selected", async () => {
    const user = userEvent.setup();
    renderPage();
    await enterNameAndSelectPublic(user);
    await user.click(screen.getByRole("button", { name: "Advanced options" }));
    await user.click(screen.getByRole("radio", { name: "Yes, use Kata" }));
    await user.click(screen.getByRole("button", { name: "Provision gateway" }));

    await waitFor(() => {
      expect(provisionGateway).toHaveBeenCalledWith({
        name: "team-gateway",
        placement: {
          selector: ["network/public", "provider/ibm", "runtime/kata"],
        },
      });
    });
  });

  // Verifies: WEB-PLC-03.01
  it("defaults multiple available options to Public and IBM Cloud", async () => {
    renderPage();

    await waitFor(() => {
      expect(radio("No").checked).toBe(true);
      expect(radio("IBM Cloud").checked).toBe(true);
    });
  });

  // Verifies: WEB-PLC-01.04, WEB-PLC-03.02
  it("forces AWS for VPN and disables IBM Cloud", async () => {
    const user = userEvent.setup();
    renderPage();
    await user.type(
      screen.getByRole("textbox", { name: "Gateway name" }),
      "team-gateway",
    );
    await user.click(radio("Yes"));

    expect(radio("Amazon Web Services").checked).toBe(true);
    expect(radio("IBM Cloud").disabled).toBe(true);
    await user.click(screen.getByRole("button", { name: "Provision gateway" }));

    await waitFor(() => {
      expect(provisionGateway).toHaveBeenCalledWith({
        name: "team-gateway",
        placement: {
          excludedSelector: ["runtime/kata"],
          selector: ["network/vpn", "provider/aws"],
        },
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
    await user.click(radio("Yes"));

    expect(radio("IBM Cloud").disabled).toBe(false);
    expect(radio("IBM Cloud").checked).toBe(true);
    await user.click(screen.getByRole("button", { name: "Provision gateway" }));

    await waitFor(() => {
      expect(provisionGateway).toHaveBeenCalledWith({
        name: "team-gateway",
        placement: {
          excludedSelector: ["runtime/kata"],
          selector: ["network/vpn", "provider/ibm"],
        },
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
        ["network/public", "provider/aws", "runtime/kata"],
        ["network/vpn", "provider/aws", "runtime/kata"],
        ["network/public", "provider/ibm", "runtime/kata"],
        ["network/vpn", "provider/ibm", "runtime/kata"],
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
      expect(radio("Yes").checked).toBe(true);
      expect(radio("Amazon Web Services").checked).toBe(true);
    });
  });

  it("selects the only available provider for public placement", async () => {
    operations.getGatewayPlacementAvailability.mockResolvedValue(
      placementAvailability([false, false, true, false]),
    );
    renderPage();

    await waitFor(() => {
      expect(radio("No").checked).toBe(true);
      expect(radio("IBM Cloud").checked).toBe(true);
    });
  });

  // Verifies: WEB-PLC-02.02
  it("uses accessible radio groups with mutually exclusive choices", async () => {
    const user = userEvent.setup();
    renderPage();

    expect(
      screen.getByRole("radiogroup", {
        name: "Does the gateway need to reach GitLab or other internal Red Hat services?",
      }),
    ).toBeTruthy();
    await user.click(screen.getByRole("radio", { name: "No" }));
    expect(radio("No").checked).toBe(true);
    await user.click(radio("Yes"));
    expect(radio("Yes").checked).toBe(true);
    expect(radio("No").checked).toBe(false);
  });

  // Verifies: WEB-PLC-06.01
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

  // Verifies: WEB-PLC-05.01
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
    expect(screen.queryByRole("radio", { name: "No" })).toBeNull();
    expect(
      screen.queryByRole("radio", { name: "Yes (Requires Red Hat VPN)" }),
    ).toBeNull();
    expect(screen.queryByText("Public IBM Cloud")).toBeNull();
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

  // Verifies: WEB-PLC-05.02
  it("hides local-kind when managed placement is also available", async () => {
    operations.getGatewayPlacementAvailability.mockResolvedValue(
      placementAvailability([true, true, true, false], true),
    );
    renderPage();

    expect(screen.queryByRole("radio", { name: "Use local-kind" })).toBeNull();
    await waitFor(() => {
      expect(radio("No").checked).toBe(true);
    });
    expect(radio("Yes").checked).toBe(false);
  });

  it("does not show a default preview while availability loads", async () => {
    let resolveAvailability:
      ((value: ReturnType<typeof placementAvailability>) => void) | undefined;
    operations.getGatewayPlacementAvailability.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveAvailability = resolve;
        }),
    );
    renderPage();

    expect(screen.queryByText("Public IBM Cloud")).toBeNull();

    resolveAvailability?.(
      placementAvailability([false, false, false, false], true),
    );
    const localKind = await screen.findByRole<HTMLInputElement>("radio", {
      name: "Use local-kind",
    });
    expect(localKind.checked).toBe(true);
    expect(screen.queryByText("Public IBM Cloud")).toBeNull();
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
    expect(radio("No").disabled).toBe(true);
    expect(radio("Yes").disabled).toBe(true);

    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
  });

  // Verifies: WEB-PLC-10.01
  it("uses network-specific availability and explains the backend reason", async () => {
    operations.getGatewayPlacementAvailability.mockResolvedValue(
      placementAvailability([true, false, false, false]),
    );
    const user = userEvent.setup();
    renderPage();
    const vpn = radio("Yes");
    expect(vpn.disabled).toBe(true);
    expect(vpn.getAttribute("aria-describedby")).toBe(
      "internal-services-yes-description",
    );
    const vpnReason = await screen.findByText(
      "VPN is unavailable: no eligible managed cluster is connected.",
    );
    expect(vpnReason.className).toContain("visuallyHidden");
    expect(vpnReason.id).toBe("internal-services-yes-description");
    expect(vpn.getAttribute("aria-describedby")).toBe(
      "internal-services-yes-description",
    );
    await user.click(screen.getByRole("radio", { name: "No" }));

    expect(radio("IBM Cloud").disabled).toBe(true);
    expect(radio("IBM Cloud").getAttribute("aria-describedby")).toBe(
      "cloud-provider-ibm-description",
    );
    const ibmReason = screen.getByText(
      "IBM Cloud is unavailable: no eligible managed cluster is connected.",
    );
    expect(ibmReason.className).toContain("visuallyHidden");
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
    expect(screen.queryByText("Public IBM Cloud")).toBeNull();
  });

  it("omits version selection and version labels from managed placement (WEB-PLC-11)", async () => {
    const user = userEvent.setup();
    renderPage();
    await enterNameAndSelectPublic(user);
    await user.click(screen.getByRole("button", { name: "Advanced options" }));

    expect(
      screen.queryByRole("combobox", { name: "OpenShell version" }),
    ).toBeNull();
    expect(screen.queryByText("Any version")).toBeNull();
    expect(screen.queryByText(/^OpenShell version:/)).toBeNull();
    await user.click(screen.getByRole("button", { name: "Provision gateway" }));

    await waitFor(() => {
      expect(provisionGateway).toHaveBeenCalledWith({
        name: "team-gateway",
        placement: {
          excludedSelector: ["runtime/kata"],
          selector: ["network/public", "provider/ibm"],
        },
      });
    });
  });
});
