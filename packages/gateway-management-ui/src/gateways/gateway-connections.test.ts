import { describe, expect, it } from "vitest";

import {
  buildGatewayAddCommand,
  buildProviderCreateCommand,
  buildProviderFromExistingCommand,
  buildSandboxCreateCommand,
  gatewayStatusAppearance,
  sandboxNamePlaceholder,
  vertexClaudeProviderName,
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
      `openshell gateway add \\
  --name gateway-1 \\
  --oidc-issuer https://issuer.example.test/realms/openshell \\
  --oidc-client-id openshell-cli \\
  --oidc-audience openshell-cli \\
  https://gateway.example.test:443`,
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
      `openshell gateway add \\
  --name gateway-1 \\
  https://gateway.example.test:443`,
    );
  });

  it("omits OIDC flags when OIDC is only partially configured", () => {
    expect(
      buildGatewayAddCommand({ ...gateway, oidcClientId: undefined }),
    ).toBe(
      `openshell gateway add \\
  --name gateway-1 \\
  https://gateway.example.test:443`,
    );
  });

  it("does not construct a command without an endpoint", () => {
    expect(
      buildGatewayAddCommand({ ...gateway, endpoint: undefined }),
    ).toBeUndefined();
  });

  it("builds the Vertex AI provider command pulling ADC and gcloud project", () => {
    expect(buildProviderCreateCommand()).toBe(
      `openshell provider create \\
  --name ${vertexClaudeProviderName} \\
  --type google-vertex-ai \\
  --from-gcloud-adc \\
  --config VERTEX_AI_PROJECT_ID="$(gcloud config get-value project)" \\
  --config VERTEX_AI_REGION=global`,
    );
  });

  it("builds the environment-only provider command", () => {
    expect(buildProviderFromExistingCommand()).toBe(
      `openshell provider create \\
  --name ${vertexClaudeProviderName} \\
  --type google-vertex-ai \\
  --from-existing`,
    );
  });

  it("creates a sandbox that attaches the provider and launches claude", () => {
    expect(buildSandboxCreateCommand()).toBe(
      `openshell sandbox create \\
  --name ${sandboxNamePlaceholder} \\
  --provider ${vertexClaudeProviderName} \\
  --no-auto-providers \\
  -- claude`,
    );
    expect(buildSandboxCreateCommand("demo")).toBe(
      `openshell sandbox create \\
  --name demo \\
  --provider ${vertexClaudeProviderName} \\
  --no-auto-providers \\
  -- claude`,
    );
  });

  it.each([
    ["Healthy", "success"],
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
