import { describe, expect, it } from "vitest";

import {
  buildGatewayAddCommand,
  buildProviderCreateCommand,
  buildSandboxCreateCommand,
  buildSandboxPolicyCommand,
  buildSetupScript,
  gatewayStatusAppearance,
  isGatewayReadyToConnect,
  sandboxName,
  sandboxPolicyFileName,
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

  it("builds the google-cloud provider command pulling ADC and project id", () => {
    expect(buildProviderCreateCommand()).toBe(
      `openshell provider create \\
  --name ${vertexProviderName} \\
  --type google-cloud \\
  --from-gcloud-adc \\
  --config project_id=$ANTHROPIC_VERTEX_PROJECT_ID \\
  --config region=global`,
    );
  });

  it("creates a sandbox that attaches the provider, forwards Vertex env, and launches claude", () => {
    expect(buildSandboxCreateCommand()).toBe(
      `openshell sandbox create \\
  --name ${sandboxName} \\
  --provider ${vertexProviderName} \\
  --env=ANTHROPIC_VERTEX_PROJECT_ID=$ANTHROPIC_VERTEX_PROJECT_ID \\
  --env=CLAUDE_CODE_USE_VERTEX=1 \\
  --policy ${sandboxPolicyFileName} \\
  --no-auto-providers \\
  -- claude`,
    );
    expect(buildSandboxCreateCommand("demo")).toBe(
      `openshell sandbox create \\
  --name demo \\
  --provider ${vertexProviderName} \\
  --env=ANTHROPIC_VERTEX_PROJECT_ID=$ANTHROPIC_VERTEX_PROJECT_ID \\
  --env=CLAUDE_CODE_USE_VERTEX=1 \\
  --policy ${sandboxPolicyFileName} \\
  --no-auto-providers \\
  -- claude`,
    );
  });

  it("writes the sandbox policy as a heredoc bound to the provider", () => {
    const command = buildSandboxPolicyCommand();

    expect(command.startsWith(`cat > ${sandboxPolicyFileName} <<'EOF'\n`)).toBe(
      true,
    );
    expect(command.endsWith("\nEOF")).toBe(true);
    expect(command).toContain(`provider: ${vertexProviderName}`);
    expect(command).toContain('host: "*-aiplatform.googleapis.com"');
    expect(command).toContain("- { path: /usr/local/bin/claude }");
  });

  it("combines login, provider, and policy into one setup script when ready", () => {
    const script = buildSetupScript(gateway);

    // The three preamble commands are consolidated into a single copyable block.
    expect(script).toContain("--oidc-issuer https://issuer.example.test");
    expect(script).toContain(buildProviderCreateCommand());
    expect(script).toContain(buildSandboxPolicyCommand());
    // Ordered login -> provider -> policy so it runs top to bottom.
    const loginAt = script?.indexOf("openshell gateway add") ?? -1;
    const providerAt = script?.indexOf("openshell provider create") ?? -1;
    const policyAt = script?.indexOf(`cat > ${sandboxPolicyFileName}`) ?? -1;
    expect(loginAt).toBeGreaterThanOrEqual(0);
    expect(loginAt).toBeLessThan(providerAt);
    expect(providerAt).toBeLessThan(policyAt);
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
