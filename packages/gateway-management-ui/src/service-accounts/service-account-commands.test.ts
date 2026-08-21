import { describe, expect, it } from "vitest";

import type { OpenShellGatewayServiceAccountConnection } from "../application/gateway-types";
import {
  buildClientCredentialsScript,
  buildOpenShellServiceAccountScript,
  serviceAccountGatewayAlias,
} from "./service-account-commands";

const connection: OpenShellGatewayServiceAccountConnection = {
  accessTokenLifetimeSeconds: 300,
  audience: "gateway-audience",
  clientId: "service-client",
  gatewayEndpoint: "https://gateway.example.test:443",
  gatewayName: "team gateway",
  issuer: "https://issuer.example.test/realms/openshell",
  tokenEndpoint:
    "https://issuer.example.test/realms/openshell/protocol/openid-connect/token",
};

describe("service-account command builders", () => {
  it("uses a distinct alias and never embeds a client secret", () => {
    const script = buildOpenShellServiceAccountScript("deploy bot", connection);

    expect(serviceAccountGatewayAlias("team gateway", "deploy bot")).toBe(
      "team gateway-deploy bot",
    );
    expect(script).toContain("--name 'team gateway-deploy bot'");
    expect(script).toContain(
      "openshell -g 'team gateway-deploy bot' whoami --output json",
    );
    expect(script).toContain("OPENSHELL_OIDC_CLIENT_SECRET");
    expect(script).not.toContain("--overwrite");
    expect(script).not.toContain("literal-secret");
  });

  it("passes the secret to curl on standard input and keeps the JWT in a variable", () => {
    const script = buildClientCredentialsScript(connection);

    expect(script).toContain("--data-urlencode client_secret@-");
    expect(script).toContain("ACCESS_TOKEN=$(");
    expect(script).toContain("grant_type=client_credentials");
    expect(script).not.toContain("refresh_token");
    expect(script).not.toContain("echo $ACCESS_TOKEN");
  });

  it("refuses to generate partial commands", () => {
    expect(
      buildOpenShellServiceAccountScript("bot", {
        ...connection,
        gatewayEndpoint: undefined,
      }),
    ).toBeUndefined();
    expect(buildOpenShellServiceAccountScript("", connection)).toBeUndefined();
    expect(
      buildClientCredentialsScript({ ...connection, audience: "" }),
    ).toBeUndefined();
  });

  it("shell-quotes hostile connection values", () => {
    const script = buildOpenShellServiceAccountScript("bot'$(touch /tmp/x)", {
      ...connection,
      clientId: "client'$(false)",
    });

    expect(script).toContain(`'client'"'"'$(false)'`);
    expect(script).toContain(`'team gateway-bot'"'"'$(touch /tmp/x)'`);
  });
});
