import { describe, expect, it } from "vitest";

import {
  buildGatewayAddCommand,
  gatewayStatusColor,
  previewGateway,
  type GatewayConnection,
} from "./gateway-connections";

describe("gateway connections", () => {
  it("builds the documented OpenShell connection command", () => {
    expect(buildGatewayAddCommand(previewGateway)).toBe(
      "openshell gateway add --name openshell-gateway-test --oidc-issuer https://keycloak-openshell-keycloak.apps.rosa.hcmais01ue1.s9m2.p3.openshiftapps.com/realms/openshell --oidc-client-id openshell-cli --oidc-audience openshell-cli https://openshell-gw-openshell-gateway-test.apps.rosa.hcmais01ue1.s9m2.p3.openshiftapps.com:443",
    );
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

  it("uses a neutral color for an unknown status", () => {
    expect(gatewayStatusColor("Unknown")).toBe("grey");
    expect(gatewayStatusColor("Ready")).toBe("green");
  });
});
