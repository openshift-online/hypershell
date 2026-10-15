import { createServer, type Server } from "node:http";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";

import type { FastifyInstance } from "fastify";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { buildApp } from "../src/app.js";
import type { ServerConfig } from "../src/config.js";

const testSessionSecret =
  "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef";

function createOidcServer(): Promise<{ close: () => void; issuer: string }> {
  const server = createServer((request, response) => {
    const address = server.address();
    if (address === null || typeof address === "string") {
      response.statusCode = 500;
      response.end();
      return;
    }
    const origin = `http://127.0.0.1:${String(address.port)}`;
    const url = new URL(request.url ?? "/", origin);
    if (url.pathname === "/.well-known/openid-configuration") {
      response.setHeader("content-type", "application/json");
      response.end(
        JSON.stringify({
          authorization_endpoint: `${origin}/authorize`,
          issuer: origin,
          jwks_uri: `${origin}/jwks`,
          token_endpoint: `${origin}/token`,
        }),
      );
      return;
    }
    if (url.pathname === "/jwks") {
      response.setHeader("content-type", "application/json");
      response.end(JSON.stringify({ keys: [] }));
      return;
    }
    response.statusCode = 404;
    response.end();
  });

  return new Promise((resolve) => {
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      if (address === null || typeof address === "string") {
        throw new Error("expected OIDC TCP listener");
      }
      resolve({
        close: () => server.close(),
        issuer: `http://127.0.0.1:${String(address.port)}`,
      });
    });
  });
}

function emptyPrometheusPayload(): string {
  return JSON.stringify({
    status: "success",
    data: { resultType: "vector", result: [] },
  });
}

describe("usage metrics routes", () => {
  let app: FastifyInstance;
  let apiServer: Server;
  let prometheusServer: Server;
  let oidcServer: { close: () => void; issuer: string } | undefined;
  let staticRoot: string;
  let apiRequests: {
    authorization: string | undefined;
    url: string | undefined;
  }[] = [];
  let prometheusQueries: string[] = [];
  let gatewayListResponse: unknown;

  beforeEach(async () => {
    apiRequests = [];
    prometheusQueries = [];
    gatewayListResponse = {
      items: [
        {
          active_sandbox_count: 2,
          namespace: "openshell-gateway-1",
          phase: "Running",
        },
      ],
      page: 1,
      total: 1,
    };
    apiServer = createServer((request, response) => {
      if (request.url === "/api/hypershell") {
        response.setHeader("content-type", "application/json");
        response.end(
          '{"id":"hypershell","kind":"API","version":"test-sha","build_time":"2026-01-01T00:00:00Z"}',
        );
        return;
      }
      apiRequests.push({
        authorization: request.headers.authorization,
        url: request.url,
      });
      response.setHeader("content-type", "application/json");
      response.end(JSON.stringify(gatewayListResponse));
    });
    await new Promise<void>((resolve) => {
      apiServer.listen(0, "127.0.0.1", resolve);
    });

    prometheusServer = createServer((request, response) => {
      const url = new URL(request.url ?? "/", "http://127.0.0.1");
      prometheusQueries.push(url.searchParams.get("query") ?? "");
      response.setHeader("content-type", "application/json");
      response.end(emptyPrometheusPayload());
    });
    await new Promise<void>((resolve) => {
      prometheusServer.listen(0, "127.0.0.1", resolve);
    });

    staticRoot = await mkdtemp(path.join(tmpdir(), "hypershell-web-console-"));
    await mkdir(path.join(staticRoot, "assets"));
    await writeFile(
      path.join(staticRoot, "index.html"),
      "<!doctype html><html><body><main>App</main></body></html>",
    );
  });

  afterEach(async () => {
    await app.close();
    await new Promise<void>((resolve, reject) => {
      apiServer.close((error) => {
        if (error) reject(error);
        else resolve();
      });
    });
    await new Promise<void>((resolve, reject) => {
      prometheusServer.close((error) => {
        if (error) reject(error);
        else resolve();
      });
    });
    oidcServer?.close();
    oidcServer = undefined;
    await rm(staticRoot, { force: true, recursive: true });
  });

  function serverOrigin(server: Server): string {
    const address = server.address();
    if (address === null || typeof address === "string") {
      throw new Error("expected TCP listener");
    }
    return `http://127.0.0.1:${String(address.port)}`;
  }

  async function buildTestApp(
    overrides: Partial<ServerConfig> = {},
  ): Promise<FastifyInstance> {
    const config: ServerConfig = {
      apiOrigin: serverOrigin(apiServer),
      apiTimeoutMs: 5_000,
      host: "127.0.0.1",
      logLevel: "silent",
      nodeEnv: "test",
      port: 8080,
      prometheusQueryTimeoutMs: 5_000,
      prometheusUrl: serverOrigin(prometheusServer),
      sessionTtlSeconds: 28_800,
      staticRoot,
      webVersion: "unknown",
      ...overrides,
    };
    return buildApp(config);
  }

  async function buildOidcApp(): Promise<FastifyInstance> {
    oidcServer = await createOidcServer();
    return buildTestApp({
      oidcClientId: "test-client",
      oidcIssuer: oidcServer.issuer,
      oidcRedirectUri: "http://127.0.0.1:8080/auth/callback",
      sessionSecret: Buffer.from(testSessionSecret, "hex"),
    });
  }

  function sessionCookie(roles: string[]): string {
    const session = app.createSecureSession({
      accessToken: "test-access-token",
      expiresAt: Math.floor(Date.now() / 1000) + 3600,
      roles,
      sub: "user-123",
    });
    return `session=${encodeURIComponent(app.encodeSecureSession(session))}`;
  }

  it("requires an authenticated session for every usage route", async () => {
    app = await buildOidcApp();

    for (const route of [
      "/api/metrics/usage/gateways",
      "/api/metrics/usage/sandboxes",
      "/api/metrics/usage/sandbox-lifetime",
    ]) {
      const response = await app.inject({ method: "GET", url: route });
      expect(response.statusCode).toBe(401);
      expect(response.json()).toMatchObject({
        error: "reauth_required",
        statusCode: 401,
      });
    }
  });

  it("allows non-admins and scopes queries to authorized namespaces", async () => {
    app = await buildOidcApp();
    const response = await app.inject({
      headers: { cookie: sessionCookie(["hypershell-users"]) },
      method: "GET",
      url: "/api/metrics/usage/gateways",
    });

    expect(response.statusCode).toBe(200);
    expect(response.json()).toMatchObject({
      active_gateways: 0,
      total_gateways: 1,
      unused_gateways: 1,
    });
    expect(apiRequests).toHaveLength(1);
    expect(apiRequests[0]).toMatchObject({
      authorization: "Bearer test-access-token",
      url: "/api/hypershell/v1/gateways?page=1&size=100",
    });
    expect(prometheusQueries.length).toBeGreaterThan(0);
    for (const query of prometheusQueries) {
      if (query.includes("openshell_server_")) {
        expect(query).toContain('namespace=~"openshell-gateway-1"');
      }
    }
  });

  it("uses fleet scope for platform administrators", async () => {
    app = await buildOidcApp();
    const response = await app.inject({
      headers: { cookie: sessionCookie(["platform:admin"]) },
      method: "GET",
      url: "/api/metrics/usage/gateways?range=30",
    });

    expect(response.statusCode).toBe(200);
    expect(apiRequests).toHaveLength(1);
    expect(apiRequests[0]).toMatchObject({
      authorization: "Bearer test-access-token",
      url: "/api/hypershell/v1/gateways?page=1&size=100",
    });
    expect(prometheusQueries.some((query) => query.includes("[30d]"))).toBe(
      true,
    );
    for (const query of prometheusQueries) {
      expect(query).not.toContain("namespace=~");
    }
  });

  it("does not query fleet sandbox data for a non-admin with no gateways", async () => {
    gatewayListResponse = { items: [], page: 1, total: 0 };
    app = await buildOidcApp();
    const response = await app.inject({
      headers: { cookie: sessionCookie(["hypershell-users"]) },
      method: "GET",
      url: "/api/metrics/usage/sandboxes",
    });

    expect(response.statusCode).toBe(200);
    expect(response.json()).toMatchObject({ active_sandboxes: 0 });
    const body: unknown = response.json();
    expect(
      (body as { daily_utilization_percent?: unknown[] })
        .daily_utilization_percent,
    ).toHaveLength(7);
    expect(prometheusQueries.length).toBeGreaterThan(0);
    for (const query of prometheusQueries) {
      if (query.includes("hypershell_gateways_active_sandboxes_total")) {
        expect(query).toContain('{gateway_namespace=""}');
      } else {
        expect(query).toContain('{namespace=""}');
      }
    }
  });

  it("treats intentional no-auth mode as an administrator", async () => {
    app = await buildTestApp();
    const response = await app.inject({
      method: "GET",
      url: "/api/metrics/usage/gateways",
    });

    expect(response.statusCode).toBe(200);
    expect(apiRequests).toHaveLength(1);
    expect(apiRequests[0]).toMatchObject({
      authorization: undefined,
      url: "/api/hypershell/v1/gateways?page=1&size=100",
    });
    expect(prometheusQueries.length).toBeGreaterThan(0);
    for (const query of prometheusQueries) {
      if (query.includes("openshell_server_")) {
        expect(query).not.toContain("namespace=~");
      }
    }
  });
});
