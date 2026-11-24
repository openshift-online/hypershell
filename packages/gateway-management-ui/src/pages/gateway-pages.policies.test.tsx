import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { IntlProvider } from "react-intl";
import { describe, expect, it, vi } from "vitest";

import type {
  GatewayOperations,
  GatewayRecord,
} from "../application/gateway-types";
import { GatewayUiProvider } from "../gateway-ui-provider";
import { GatewayPage, toGatewayDetailTab } from "./gateway-pages";

const gateway: GatewayRecord = {
  clusterId: "cluster-east",
  consoleUrl: "https://console.example.test",
  externalDns: "gateway.example.test",
  gatewayVersion: "0.0.109",
  id: "payments-gateway",
  name: "payments-gateway",
  namespace: "openshell",
  oidcAudience: "openshell-cli",
  oidcClientId: "openshell-cli",
  oidcIssuer: "https://issuer.example.test/realms/openshell",
  phase: "Running",
  provisioningConditions: [
    { conditionStatus: "Complete", message: "", type: "GatewayHealthy" },
  ],
  status: "Ready",
};

function renderGateway(
  activeTab: "connection" | "policies",
  getGatewayEffectivePolicies = vi.fn(() =>
    Promise.resolve({
      defaults: {
        cpuLimit: "3",
        cpuRequest: "750m",
        memoryLimit: "6Gi",
        memoryRequest: "2Gi",
      },
      findings: [],
      findingsLast24h: { wouldAddDefaults: 0, wouldReject: 0 },
      gatewayId: "payments-gateway",
      globalMode: "audit" as const,
      policies: [],
      schemaVersion: "v1",
    }),
  ),
) {
  const gateways = {
    getGatewayEffectivePolicies,
    getGatewayPlacement: vi.fn(() =>
      Promise.resolve({ id: "cluster-east", name: "Cluster East" }),
    ),
    listOpenShellGatewayServiceAccounts: vi.fn(
      () => new Promise(() => undefined),
    ),
  } as unknown as GatewayOperations;
  render(
    <IntlProvider locale="en">
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <GatewayUiProvider
          gateways={gateways}
          navigation={{
            collectionHref: "/gateways",
            createHref: "/gateways/new",
            detailHref: (id) => `/gateways/${id}`,
            navigate: () => undefined,
          }}
        >
          <GatewayPage
            activeTab={activeTab}
            gateway={gateway}
            gatewayId={gateway.id}
          />
        </GatewayUiProvider>
      </QueryClientProvider>
    </IntlProvider>,
  );
  return getGatewayEffectivePolicies;
}

describe("GatewayPage policies", () => {
  it("accepts the policies tab", () => {
    expect(toGatewayDetailTab("policies")).toBe("policies");
  });

  // Verifies: WEB-GLIS-26
  it("renders the read-only Policies tab", async () => {
    renderGateway("policies");

    expect(screen.getByRole("tab", { name: "Policies" })).toBeTruthy();
    expect(await screen.findByText("No platform policies")).toBeTruthy();
    expect(
      screen.getByDisplayValue(
        "openshell sandbox create -g payments-gateway --name my-sandbox --cpu 3 --memory 6Gi",
      ),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: /edit/iu })).toBeNull();
  });

  // Verifies: WEB-GLIS-26, WEB-GLIS-11
  it("feeds the fleet defaults into the Connection tab command", async () => {
    const user = userEvent.setup();
    const read = renderGateway("connection");

    await waitFor(() => {
      expect(read).toHaveBeenCalledWith("payments-gateway", expect.anything());
    });
    await user.click(screen.getByRole("button", { name: "Copy all commands" }));
    await waitFor(async () => {
      const copied = await navigator.clipboard.readText();
      expect(copied).toContain("--cpu 3");
      expect(copied).toContain("--memory 6Gi");
    });
  });
});
