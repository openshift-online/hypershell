import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IntlProvider } from "react-intl";
import { MemoryRouter, Route, Routes } from "react-router";
import { vi } from "vitest";

import { englishMessages } from "../../i18n/catalog";
import type { ClusterOption } from "../clusters/cluster-options";
import { AdminGatewayCreatePage } from "./admin-gateway-create";

const { createGatewayMock } = vi.hoisted(() => ({
  createGatewayMock: vi.fn(),
}));

vi.mock("../../lib/api.client", () => ({
  apiClient: {
    gateways: {
      create: createGatewayMock,
    },
  },
}));

const createdGateway = {
  cluster_id: "",
  created_at: null,
  database_id: "database-1",
  external_dns: "",
  fleet_id: "",
  href: "/api/hypershell/v1/gateways/gateway-1",
  id: "gateway-1",
  kind: "Gateway",
  name: "team-gateway",
  namespace: "openshell",
  phase: "",
  release_id: "release-1",
  service_type: "",
  status: "",
  tls_mode: "",
  updated_at: null,
};

function renderPage(
  initialEntry = "/admin/gateways/new",
  clusters?: readonly ClusterOption[],
) {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });

  return render(
    <IntlProvider locale="en" messages={englishMessages}>
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[initialEntry]}>
          <Routes>
            <Route
              element={<AdminGatewayCreatePage clusters={clusters} />}
              path="/admin/gateways/new"
            />
            <Route
              element={<div data-testid="created-gateway" />}
              path="/admin/gateways/:gatewayId"
            />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>
    </IntlProvider>,
  );
}

describe("AdminGatewayCreatePage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("provisions through the API without placement fields", async () => {
    const user = userEvent.setup();
    createGatewayMock.mockResolvedValue(createdGateway);
    renderPage();

    expect(
      screen.getByRole("textbox", { name: "Cluster" }).getAttribute("value"),
    ).toBe("Local cluster");

    await user.type(
      screen.getByRole("textbox", { name: "Gateway name" }),
      "team-gateway",
    );
    await user.type(
      screen.getByRole("textbox", { name: "Gateway release ID" }),
      "release-1",
    );
    await user.type(
      screen.getByRole("textbox", { name: "Managed database ID" }),
      "database-1",
    );
    await user.click(screen.getByRole("button", { name: "Provision gateway" }));

    await waitFor(() => {
      expect(createGatewayMock).toHaveBeenCalledWith({
        database_id: "database-1",
        name: "team-gateway",
        namespace: "openshell",
        release_id: "release-1",
      });
    });
    expect(screen.getByTestId("created-gateway")).toBeTruthy();
  });

  it("shows cluster context selected through the URL", () => {
    renderPage("/admin/gateways/new?cluster=cluster-east", [
      {
        description: "A future remote placement.",
        id: "cluster-east",
        name: "East cluster",
      },
    ]);

    expect(
      screen.getByRole("textbox", { name: "Cluster" }).getAttribute("value"),
    ).toBe("East cluster");
  });

  it("validates required values before sending a request", async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByRole("button", { name: "Provision gateway" }));

    expect(await screen.findAllByText("This field is required.")).toHaveLength(
      3,
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
    await user.type(
      screen.getByRole("textbox", { name: "Gateway release ID" }),
      "release-1",
    );
    await user.type(
      screen.getByRole("textbox", { name: "Managed database ID" }),
      "database-1",
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
    expect(await screen.findByTestId("created-gateway")).toBeTruthy();
  });
});
