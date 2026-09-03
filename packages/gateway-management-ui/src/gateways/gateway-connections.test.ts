import { describe, expect, it } from "vitest";

import {
  buildGatewayAddCommand,
  buildInferenceSetCommand,
  buildProviderCreateCommand,
  buildSandboxCreateCommand,
  buildSetupScript,
  claudeModel,
  gatewayStatusAppearance,
  isGatewayReadyToConnect,
  sandboxName,
  vertexProviderName,
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
  phase: "Running",
  status: "Running",
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

  it("does not construct a command until the phase is Running", () => {
    // A populated endpoint (route address) may exist while the gateway is still
    // provisioning; the command is withheld until phase reaches Running.
    for (const phase of ["Provisioning", "Pending", "Degraded", "Failed", ""]) {
      expect(buildGatewayAddCommand({ ...gateway, phase })).toBeUndefined();
    }
    expect(
      buildGatewayAddCommand({ ...gateway, phase: undefined }),
    ).toBeUndefined();
  });

  it("reports readiness only when phase is Running and an endpoint exists", () => {
    expect(isGatewayReadyToConnect(gateway)).toBe(true);
    expect(isGatewayReadyToConnect({ ...gateway, phase: "running" })).toBe(
      true,
    );
    expect(isGatewayReadyToConnect({ ...gateway, phase: "Provisioning" })).toBe(
      false,
    );
    expect(isGatewayReadyToConnect({ ...gateway, phase: undefined })).toBe(
      false,
    );
    expect(isGatewayReadyToConnect({ ...gateway, endpoint: undefined })).toBe(
      false,
    );
  });

  it("builds the google-vertex-ai provider command pulling ADC and project id", () => {
    expect(buildProviderCreateCommand()).toBe(
      `openshell provider create \\
  --name ${vertexProviderName} \\
  --type google-vertex-ai \\
  --from-gcloud-adc \\
  --config VERTEX_AI_PROJECT_ID="$ANTHROPIC_VERTEX_PROJECT_ID" \\
  --config VERTEX_AI_REGION=global`,
    );
  });

  it("points inference at the Claude model through the provider", () => {
    expect(buildInferenceSetCommand()).toBe(
      `openshell inference set --provider ${vertexProviderName} --model ${claudeModel}`,
    );
  });

  it("substitutes a chosen provider name into the provider command", () => {
    expect(buildProviderCreateCommand("MARKER")).toContain("--name MARKER");
  });

  it("substitutes chosen provider and model names into inference set", () => {
    expect(buildInferenceSetCommand("PROV", "MODEL")).toBe(
      "openshell inference set --provider PROV --model MODEL",
    );
  });

  it("threads provider and model overrides through the setup script", () => {
    const script = buildSetupScript(gateway, {
      model: "MODEL",
      providerName: "PROV",
    });

    // The provider name is mirrored across both the create and inference steps.
    expect(script).toContain("--name PROV");
    expect(script).toContain("--provider PROV --model MODEL");
    expect(script).not.toContain(vertexProviderName);
    expect(script).not.toContain(claudeModel);
  });

  it("creates a sandbox that runs claude against the local inference endpoint", () => {
    expect(buildSandboxCreateCommand()).toBe(
      `openshell sandbox create \\
  --name ${sandboxName} \\
  --cpu 0.2 \\
  --memory 512Mi \\
  --env=ANTHROPIC_BASE_URL=https://inference.local \\
  --env=ANTHROPIC_API_KEY=unused \\
  --no-auto-providers \\
  -- claude --bare --model ${claudeModel}`,
    );
    expect(buildSandboxCreateCommand("demo")).toBe(
      `openshell sandbox create \\
  --name demo \\
  --cpu 0.2 \\
  --memory 512Mi \\
  --env=ANTHROPIC_BASE_URL=https://inference.local \\
  --env=ANTHROPIC_API_KEY=unused \\
  --no-auto-providers \\
  -- claude --bare --model ${claudeModel}`,
    );
  });

  it("passes a custom model to the sandbox command", () => {
    const cmd = buildSandboxCreateCommand("mysand", "claude-opus-5");
    expect(cmd).toContain("--no-auto-providers");
    expect(cmd).toContain("-- claude --bare --model claude-opus-5");
  });

  it("combines login, provider, and inference into one setup script when ready", () => {
    const script = buildSetupScript(gateway);

    // The three preamble commands are consolidated into a single copyable block.
    expect(script).toContain("--oidc-issuer https://issuer.example.test");
    expect(script).toContain(buildProviderCreateCommand());
    expect(script).toContain(buildInferenceSetCommand());
    // The policy heredoc is gone; the inference-based flow needs no policy file.
    expect(script).not.toContain("cat >");
    // Ordered login -> provider -> inference so it runs top to bottom.
    const loginAt = script?.indexOf("openshell gateway add") ?? -1;
    const providerAt = script?.indexOf("openshell provider create") ?? -1;
    const inferenceAt = script?.indexOf("openshell inference set") ?? -1;
    expect(loginAt).toBeGreaterThanOrEqual(0);
    expect(loginAt).toBeLessThan(providerAt);
    expect(providerAt).toBeLessThan(inferenceAt);
  });

  it("withholds the setup script until the gateway is ready to connect", () => {
    expect(buildSetupScript({ ...gateway, phase: "Provisioning" })).toBe(
      undefined,
    );
    expect(buildSetupScript({ ...gateway, endpoint: undefined })).toBe(
      undefined,
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
    ["Revoking", "progress"],
    ["Deleting", "progress"],
    ["Expired", "inactive"],
    ["Revoked", "inactive"],
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
