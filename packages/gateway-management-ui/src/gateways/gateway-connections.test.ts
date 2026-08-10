import { describe, expect, it } from "vitest";

import {
  buildGatewayAddCommand,
  gatewayStatusAppearance,
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

  it("omits OIDC flags when OIDC is not configured", () => {
    const noAuth: GatewayConnection = {
      ...gateway,
      oidcAudience: undefined,
      oidcClientId: undefined,
      oidcIssuer: undefined,
    };

    expect(buildGatewayAddCommand(noAuth)).toBe(
      "openshell gateway add --name gateway-1 https://gateway.example.test:443",
    );
  });

  it("omits OIDC flags when OIDC is only partially configured", () => {
    expect(
      buildGatewayAddCommand({ ...gateway, oidcClientId: undefined }),
    ).toBe(
      "openshell gateway add --name gateway-1 https://gateway.example.test:443",
    );
  });

  it("does not construct a command without an endpoint", () => {
    expect(
      buildGatewayAddCommand({ ...gateway, endpoint: undefined }),
    ).toBeUndefined();
  });

  it.each([
    ["Ready", "success"],
    ["Running", "success"],
    ["Failed", "danger"],
    ["Degraded", "warning"],
    ["Pending", "pending"],
    ["Provisioning", "progress"],
    ["Unknown", "unknown"],
    ["Unexpected provider state", "unknown"],
    ["", "unknown"],
  ] as const)(
    "maps %s to a bounded status appearance",
    (status, appearance) => {
      expect(gatewayStatusAppearance(status)).toEqual(appearance);
    },
  );
});
