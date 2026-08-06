import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IntlProvider } from "react-intl";
import { vi } from "vitest";

import { GatewayUiProvider } from "../gateway-ui-provider";
import { GatewayCreatePage } from "./gateway-create";

const { createGatewayMock, navigateMock } = vi.hoisted(() => ({
  createGatewayMock: vi.fn(),
  navigateMock: vi.fn(),
}));

const gatewayOperations = {
  getGateway: vi.fn(),
  listGateways: vi.fn(),
  provisionGateway: createGatewayMock,
  removeGateway: vi.fn(),
  renameGateway: vi.fn(),
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

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });

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
    navigateMock.mockResolvedValue(undefined);
  });

  it("provisions through the API with empty reconciler-owned IDs", async () => {
    const user = userEvent.setup();
    createGatewayMock.mockResolvedValue(createdGateway);
    renderPage();

    expect(screen.queryByLabelText("Cluster")).toBeNull();
    expect(screen.queryByLabelText("Gateway release")).toBeNull();
    expect(screen.queryByLabelText("Managed database")).toBeNull();

    await user.type(
      screen.getByRole("textbox", { name: "Gateway name" }),
      "team-gateway",
    );
    await user.click(screen.getByRole("button", { name: "Provision gateway" }));

    await waitFor(() => {
      expect(createGatewayMock).toHaveBeenCalledWith({
        name: "team-gateway",
        namespace: "openshell",
      });
    });
    await waitFor(() => {
      expect(navigateMock).toHaveBeenCalledWith("/gateways/gateway-1");
    });
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
