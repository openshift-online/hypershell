import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IntlProvider } from "react-intl";
import { MemoryRouter, Route, Routes } from "react-router";
import { vi } from "vitest";

import { englishMessages } from "../../i18n/catalog";
import {
  buildGatewayAddCommand,
  gatewayStatusColor,
  previewGateway,
  previewGateways,
  type GatewayConnection,
} from "./gateway-connections";
import {
  GatewayDetails,
  GatewayDetailsPage,
  GatewayDirectory,
  GatewayDirectoryPage,
} from "./gateway-directory";

const { getGatewayConnectionMock, listGatewayConnectionsMock } = vi.hoisted(
  () => ({
    getGatewayConnectionMock: vi.fn(),
    listGatewayConnectionsMock: vi.fn(),
  }),
);

vi.mock("./gateway-data", () => ({
  getGatewayConnection: getGatewayConnectionMock,
  listGatewayConnections: listGatewayConnectionsMock,
}));

const expectedCommand =
  "openshell gateway add --name openshell-gateway-test --oidc-issuer https://keycloak-openshell-keycloak.apps.rosa.hcmais01ue1.s9m2.p3.openshiftapps.com/realms/openshell --oidc-client-id openshell-cli --oidc-audience openshell-cli https://openshell-gw-openshell-gateway-test.apps.rosa.hcmais01ue1.s9m2.p3.openshiftapps.com:443";

function renderContent(
  content: React.ReactNode,
  initialEntry = "/",
  routePath?: string,
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  return render(
    <IntlProvider locale="en" messages={englishMessages}>
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[initialEntry]}>
          {routePath ? (
            <Routes>
              <Route element={content} path={routePath} />
            </Routes>
          ) : (
            content
          )}
        </MemoryRouter>
      </QueryClientProvider>
    </IntlProvider>,
  );
}

describe("buildGatewayAddCommand", () => {
  it("builds the documented OpenShell connection command", () => {
    expect(buildGatewayAddCommand(previewGateway)).toBe(expectedCommand);
  });

  it("quotes values that could be interpreted by a shell", () => {
    const gateway: GatewayConnection = {
      ...previewGateway,
      name: "gateway $(unsafe)",
    };

    expect(buildGatewayAddCommand(gateway)).toContain(
      "--name 'gateway $(unsafe)'",
    );
  });
});

describe("GatewayDirectory", () => {
  it("gives users both console and CLI connection methods", async () => {
    const user = userEvent.setup();
    const writeText = vi.fn();
    Object.defineProperty(window.navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });

    renderContent(<GatewayDirectory gateways={previewGateways} />);

    expect(
      screen.getByRole("heading", { level: 1, name: "OpenShell gateways" }),
    ).toBeTruthy();
    expect(
      screen
        .getByRole("link", {
          name: "Open console for openshell-gateway-test in a new tab",
        })
        .getAttribute("href"),
    ).toBe(previewGateway.consoleUrl);
    expect(
      screen
        .getByRole("link", { name: "openshell-gateway-test" })
        .getAttribute("href"),
    ).toBe("/gateways/openshell-gateway-test");

    await user.click(
      screen.getByRole("button", {
        name: "Copy connection command for openshell-gateway-test",
      }),
    );
    expect(writeText).toHaveBeenCalledWith(expectedCommand);
  });

  it("provides guidance when no gateways are visible", () => {
    renderContent(<GatewayDirectory gateways={[]} />);

    expect(
      screen.getByRole("heading", { level: 2, name: "No gateways available" }),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "Ask your OpenShell administrator for access to a gateway.",
      ),
    ).toBeTruthy();
  });

  it("renders an unknown status with the neutral label color", () => {
    renderContent(
      <GatewayDirectory
        gateways={[{ ...previewGateway, status: "Unknown" }]}
      />,
    );

    const statusLabel = screen.getByText("Unknown").closest(".pf-v6-c-label");
    expect(gatewayStatusColor("Unknown")).toBe("grey");
    expect(statusLabel?.classList.contains("pf-m-green")).toBe(false);
  });
});

describe("GatewayDetails", () => {
  it("presents the same connection command on the detail page", () => {
    renderContent(<GatewayDetails gateway={previewGateway} />);

    expect(
      screen.getByRole("heading", {
        level: 1,
        name: "openshell-gateway-test",
      }),
    ).toBeTruthy();
    expect(screen.getByDisplayValue(expectedCommand)).toBeTruthy();
    expect(screen.getByText("Ready")).toBeTruthy();
  });
});

describe("API-backed gateway pages", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("loads the gateway directory from the API", async () => {
    listGatewayConnectionsMock.mockResolvedValue(previewGateways);

    renderContent(<GatewayDirectoryPage />);

    expect(
      await screen.findByRole("heading", {
        level: 1,
        name: "OpenShell gateways",
      }),
    ).toBeTruthy();
    expect(listGatewayConnectionsMock).toHaveBeenCalledOnce();
  });

  it("refreshes the gateway directory without leaving the page", async () => {
    const user = userEvent.setup();
    listGatewayConnectionsMock.mockResolvedValue(previewGateways);
    renderContent(<GatewayDirectoryPage />);

    await screen.findByRole("heading", {
      level: 1,
      name: "OpenShell gateways",
    });
    await user.click(screen.getByRole("button", { name: "Refresh gateways" }));

    await waitFor(() => {
      expect(listGatewayConnectionsMock).toHaveBeenCalledTimes(2);
    });
  });

  it("shows progress and API failures for the gateway directory", async () => {
    listGatewayConnectionsMock.mockRejectedValue(new Error("unavailable"));

    renderContent(<GatewayDirectoryPage />);

    expect(screen.getByLabelText("Loading gateways")).toBeTruthy();
    expect(
      await screen.findByText("Gateways could not be loaded"),
    ).toBeTruthy();
  });

  it("loads gateway details by route ID", async () => {
    getGatewayConnectionMock.mockResolvedValue(previewGateway);

    renderContent(
      <GatewayDetailsPage />,
      "/gateways/openshell-gateway-test",
      "/gateways/:gatewayId",
    );

    expect(
      await screen.findByRole("heading", {
        level: 1,
        name: "openshell-gateway-test",
      }),
    ).toBeTruthy();
    expect(getGatewayConnectionMock).toHaveBeenCalledWith(
      "openshell-gateway-test",
      expect.any(AbortSignal),
    );
  });

  it("shows API failures for gateway details", async () => {
    getGatewayConnectionMock.mockRejectedValue(new Error("unavailable"));

    renderContent(
      <GatewayDetailsPage />,
      "/gateways/missing",
      "/gateways/:gatewayId",
    );

    expect(
      await screen.findByText("Gateways could not be loaded"),
    ).toBeTruthy();
  });
});
