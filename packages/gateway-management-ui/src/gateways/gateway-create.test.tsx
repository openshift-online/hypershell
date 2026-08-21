import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IntlProvider } from "react-intl";
import { vi } from "vitest";

import { GatewayUiProvider } from "../gateway-ui-provider";
import { GatewayCreatePage } from "./gateway-create";

const { createGatewayMock, findGatewayPlacementsMock, navigateMock } =
  vi.hoisted(() => ({
    createGatewayMock: vi.fn(),
    findGatewayPlacementsMock: vi.fn(),
    navigateMock: vi.fn(),
  }));

const gatewayOperations = {
  createOpenShellGatewayServiceAccount: vi.fn(),
  deleteOpenShellGatewayServiceAccount: vi.fn(),
  findGatewayPlacements: findGatewayPlacementsMock,
  getGateway: vi.fn(),
  getGatewayPlacement: vi.fn(),
  getGatewayPlacements: vi.fn(),
  getOpenShellGatewayServiceAccount: vi.fn(),
  listGateways: vi.fn(),
  listOpenShellGatewayServiceAccounts: vi.fn(),
  provisionGateway: createGatewayMock,
  removeGateway: vi.fn(),
  renameGateway: vi.fn(),
  revokeOpenShellGatewayServiceAccount: vi.fn(),
};

const navigation = {
  collectionHref: "/",
  createHref: "/gateways/new",
  detailHref: (gatewayId: string) => `/gateways/${gatewayId}`,
  navigate: navigateMock,
};

const createdGateway = {
  clusterId: "",
  databaseId: "",
  externalDns: "",
  id: "gateway-1",
  name: "team-gateway",
  namespace: "openshell",
  phase: "",
  releaseId: "",
  status: "",
};

function createTestQueryClient() {
  return new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });
}

function renderPage(queryClient = createTestQueryClient()) {
  return render(
    <IntlProvider locale="en">
      <QueryClientProvider client={queryClient}>
        <GatewayUiProvider gateways={gatewayOperations} navigation={navigation}>
          <GatewayCreatePage />
        </GatewayUiProvider>
      </QueryClientProvider>
    </IntlProvider>,
  );
}

describe("GatewayCreatePage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    findGatewayPlacementsMock.mockResolvedValue({
      hasMore: false,
      items: [
        {
          id: "cluster-east",
          name: "Cluster East",
          provider: "AWS",
          region: "us-east-1",
          status: "Ready",
        },
      ],
    });
    navigateMock.mockResolvedValue(undefined);
  });

  it("provisions on the hub by default without exposing a namespace", async () => {
    const user = userEvent.setup();
    createGatewayMock.mockResolvedValue(createdGateway);
    renderPage();

    expect(
      screen.getByRole<HTMLInputElement>("combobox", { name: "Cluster" }).value,
    ).toBe("Hub cluster (default)");
    expect(screen.queryByLabelText("Namespace")).toBeNull();
    expect(screen.queryByLabelText("Gateway release")).toBeNull();
    expect(screen.queryByLabelText("Managed database")).toBeNull();

    await user.type(
      screen.getByRole("textbox", { name: "Gateway name" }),
      "  team-gateway  ",
    );
    await user.click(screen.getByRole("button", { name: "Provision gateway" }));

    await waitFor(() => {
      expect(createGatewayMock).toHaveBeenCalledWith({
        clusterId: "",
        name: "team-gateway",
      });
    });
    await waitFor(() => {
      expect(navigateMock).toHaveBeenCalledWith("/gateways/gateway-1");
    });
  });

  it("searches managed clusters and provisions on the selected result", async () => {
    const user = userEvent.setup();
    createGatewayMock.mockResolvedValue({
      ...createdGateway,
      clusterId: "cluster-east",
    });
    renderPage();

    await user.click(
      screen.getByRole("button", { name: "Clear cluster search" }),
    );
    const clusterInput = screen.getByRole("combobox", { name: "Cluster" });
    await user.type(clusterInput, "East");
    const clusterOption = await screen.findByText("Cluster East");
    expect(screen.getByText("Provider: AWS; region: us-east-1")).toBeTruthy();
    await user.click(clusterOption);
    expect((clusterInput as HTMLInputElement).value).toBe("Cluster East");

    await user.type(
      screen.getByRole("textbox", { name: "Gateway name" }),
      "team-gateway",
    );
    await user.click(screen.getByRole("button", { name: "Provision gateway" }));

    await waitFor(() => {
      expect(createGatewayMock).toHaveBeenCalledWith({
        clusterId: "cluster-east",
        name: "team-gateway",
      });
    });
    expect(findGatewayPlacementsMock).toHaveBeenCalledWith(
      "East",
      expect.any(AbortSignal),
    );
  });

  it("debounces placement searches into one API request", async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => {
      expect(findGatewayPlacementsMock).toHaveBeenCalledWith(
        "",
        expect.any(AbortSignal),
      );
    });
    findGatewayPlacementsMock.mockClear();

    await user.click(
      screen.getByRole("button", { name: "Clear cluster search" }),
    );
    await user.type(screen.getByRole("combobox", { name: "Cluster" }), "East");

    expect(findGatewayPlacementsMock).not.toHaveBeenCalled();
    await waitFor(() => {
      expect(findGatewayPlacementsMock).toHaveBeenCalledWith(
        "East",
        expect.any(AbortSignal),
      );
    });
    expect(findGatewayPlacementsMock).toHaveBeenCalledOnce();
  });

  it("shows one loading message while placement options are pending", async () => {
    const user = userEvent.setup();
    findGatewayPlacementsMock.mockReturnValue(new Promise(() => undefined));
    renderPage();

    const clusterInput = screen.getByRole("combobox", { name: "Cluster" });
    await user.click(clusterInput);

    expect(await screen.findAllByText("Loading managed clusters")).toHaveLength(
      1,
    );
    expect(clusterInput.getAttribute("aria-busy")).toBe("true");
  });

  it("reuses fresh placement results after remounting", async () => {
    const user = userEvent.setup();
    const queryClient = createTestQueryClient();
    const firstRender = renderPage(queryClient);
    await user.click(screen.getByRole("combobox", { name: "Cluster" }));
    expect(await screen.findByText("Cluster East")).toBeTruthy();
    expect(findGatewayPlacementsMock).toHaveBeenCalledOnce();
    firstRender.unmount();

    renderPage(queryClient);
    await user.click(screen.getByRole("combobox", { name: "Cluster" }));
    expect(await screen.findByText("Cluster East")).toBeTruthy();
    await new Promise((resolve) => window.setTimeout(resolve, 50));
    expect(findGatewayPlacementsMock).toHaveBeenCalledOnce();
  });

  it("keeps hub provisioning available when managed clusters fail to load", async () => {
    const user = userEvent.setup();
    findGatewayPlacementsMock.mockRejectedValue(
      new Error("managed cluster API unavailable"),
    );
    createGatewayMock.mockResolvedValue(createdGateway);
    renderPage();

    expect(
      await screen.findByText("Managed clusters could not be loaded"),
    ).toBeTruthy();
    expect(
      screen.getByRole<HTMLInputElement>("combobox", { name: "Cluster" }).value,
    ).toBe("Hub cluster (default)");
    expect(screen.getByRole("button", { name: "Retry" })).toBeTruthy();

    await user.type(
      screen.getByRole("textbox", { name: "Gateway name" }),
      "team-gateway",
    );
    await user.click(screen.getByRole("button", { name: "Provision gateway" }));
    await waitFor(() => {
      expect(createGatewayMock).toHaveBeenCalledWith({
        clusterId: "",
        name: "team-gateway",
      });
    });
  });

  it("requires an explicit option after a free-form cluster search", async () => {
    const user = userEvent.setup();
    findGatewayPlacementsMock.mockResolvedValue({
      hasMore: false,
      items: [],
    });
    renderPage();

    await user.click(
      screen.getByRole("button", { name: "Clear cluster search" }),
    );
    await user.type(
      screen.getByRole("combobox", { name: "Cluster" }),
      "Unknown placement",
    );
    expect(await screen.findByText("No matching clusters")).toBeTruthy();
    await user.type(
      screen.getByRole("textbox", { name: "Gateway name" }),
      "team-gateway",
    );
    await user.click(screen.getByRole("button", { name: "Provision gateway" }));

    expect(await screen.findByText("This field is required.")).toBeTruthy();
    expect(createGatewayMock).not.toHaveBeenCalled();
  });

  it("supports keyboard selection and bounded-result guidance", async () => {
    const user = userEvent.setup();
    findGatewayPlacementsMock.mockResolvedValue({
      hasMore: true,
      items: [
        {
          id: "cluster-west",
          name: "Cluster West",
          provider: "GCP",
          region: "us-west1",
          status: "Ready",
        },
      ],
    });
    renderPage();

    expect(
      await screen.findByText(
        "More clusters are available. Refine your search to find a specific cluster.",
      ),
    ).toBeTruthy();
    await user.click(
      screen.getByRole("button", { name: "Clear cluster search" }),
    );
    const clusterInput = screen.getByRole<HTMLInputElement>("combobox", {
      name: "Cluster",
    });
    await user.type(clusterInput, "West");
    await screen.findByText("Cluster West");
    await user.keyboard("{ArrowDown}{Enter}");

    expect(clusterInput.value).toBe("Cluster West");
  });

  it("restores the last accepted placement when Escape cancels a search", async () => {
    const user = userEvent.setup();
    createGatewayMock.mockResolvedValue({
      ...createdGateway,
      clusterId: "cluster-east",
    });
    renderPage();

    await user.click(
      screen.getByRole("button", { name: "Clear cluster search" }),
    );
    const clusterInput = screen.getByRole<HTMLInputElement>("combobox", {
      name: "Cluster",
    });
    await user.type(clusterInput, "East");
    await user.click(await screen.findByText("Cluster East"));

    await user.clear(clusterInput);
    await user.type(clusterInput, "Uncommitted search");
    await user.keyboard("{Escape}");

    expect(clusterInput.value).toBe("Cluster East");
    expect(clusterInput.getAttribute("aria-expanded")).toBe("false");

    await user.type(
      screen.getByRole("textbox", { name: "Gateway name" }),
      "team-gateway",
    );
    await user.click(screen.getByRole("button", { name: "Provision gateway" }));
    await waitFor(() => {
      expect(createGatewayMock).toHaveBeenCalledWith({
        clusterId: "cluster-east",
        name: "team-gateway",
      });
    });
  });

  it("shows the available provider and region context for cluster options", async () => {
    const user = userEvent.setup();
    findGatewayPlacementsMock.mockResolvedValue({
      hasMore: false,
      items: [
        {
          id: "provider-only",
          name: "Provider cluster",
          provider: "AWS",
        },
        {
          id: "region-only",
          name: "Region cluster",
          provider: "",
          region: "eu-west-1",
        },
        {
          id: "without-context",
          name: "Unclassified cluster",
          provider: "",
        },
      ],
    });
    renderPage();

    await user.click(screen.getByRole("combobox", { name: "Cluster" }));
    expect(await screen.findByText("Provider cluster")).toBeTruthy();
    expect(screen.getByText("Provider: AWS")).toBeTruthy();
    expect(screen.getByText("Region: eu-west-1")).toBeTruthy();
    expect(screen.getByText("Unclassified cluster")).toBeTruthy();
  });

  it("retries a failed managed-cluster request", async () => {
    const user = userEvent.setup();
    findGatewayPlacementsMock
      .mockRejectedValueOnce(new Error("managed cluster API unavailable"))
      .mockResolvedValue({
        hasMore: false,
        items: [
          {
            id: "cluster-east",
            name: "Cluster East",
            provider: "AWS",
            region: "us-east-1",
            status: "Ready",
          },
        ],
      });
    renderPage();

    await user.click(await screen.findByRole("button", { name: "Retry" }));
    await user.click(screen.getByRole("combobox", { name: "Cluster" }));
    expect(await screen.findByText("Cluster East")).toBeTruthy();
    expect(
      screen.queryByText("Managed clusters could not be loaded"),
    ).toBeNull();
  });

  it("validates required values before sending a request", async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByRole("button", { name: "Provision gateway" }));

    expect(await screen.findAllByText("This field is required.")).toHaveLength(
      1,
    );
    expect(createGatewayMock).not.toHaveBeenCalled();
  });

  it("shows progress only while the create request is pending", async () => {
    const user = userEvent.setup();
    let resolveRequest: ((gateway: typeof createdGateway) => void) | undefined;
    createGatewayMock.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveRequest = resolve;
        }),
    );
    renderPage();

    await user.type(
      screen.getByRole("textbox", { name: "Gateway name" }),
      "team-gateway",
    );
    const submitButton = screen.getByRole("button", {
      name: "Provision gateway",
    });
    expect(submitButton.classList.contains("pf-m-progress")).toBe(false);

    await user.click(submitButton);

    await waitFor(() => {
      expect(submitButton.classList.contains("pf-m-progress")).toBe(true);
    });
    resolveRequest?.(createdGateway);
    await waitFor(() => {
      expect(navigateMock).toHaveBeenCalledWith("/gateways/gateway-1");
    });
  });
});
