import { describe, expect, it } from "vitest";

import {
  buildGatewayAddCommand,
  gatewayStatusColor,
  type GatewayConnection,
} from "./gateway-connections";

const gateway: GatewayConnection = {
  clusterName: "Hub cluster",
  consoleUrl: "https://console.example.test",
  endpoint: "https://gateway.example.test:443",
  id: "gateway-1",
  name: "gateway-1",
  oidcAudience: "openshell-cli",
  oidcClientId: "openshell-cli",
  oidcIssuer: "https://issuer.example.test/realms/openshell",
  status: "Ready",
};

describe("gateway connections", () => {
  it("builds the documented OpenShell connection command", () => {
    expect(buildGatewayAddCommand(gateway)).toBe(
      "openshell gateway add --name gateway-1 --oidc-issuer https://issuer.example.test/realms/openshell --oidc-client-id openshell-cli --oidc-audience openshell-cli https://gateway.example.test:443",
    );
  });

  it("quotes values that could be interpreted by a shell", () => {
    const unsafeGateway: GatewayConnection = {
      ...gateway,
      name: "gateway $(unsafe)",
    };

    expect(buildGatewayAddCommand(unsafeGateway)).toContain(
      "--name 'gateway $(unsafe)'",
    );
  });

  it("does not construct a command from incomplete API data", () => {
    expect(
      buildGatewayAddCommand({ ...gateway, oidcIssuer: undefined }),
    ).toBeUndefined();
  });

  it.each([
    ["Ready", "green"],
    ["Failed", "orange"],
    ["Degraded", "yellow"],
    ["Provisioning", "blue"],
    ["Unknown", "grey"],
    ["Unexpected provider state", "grey"],
    ["", "grey"],
  ] as const)("maps %s to the bounded %s status color", (status, color) => {
    expect(gatewayStatusColor(status)).toBe(color);
  });
});
