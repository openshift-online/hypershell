import {
  createServer,
  type IncomingMessage,
  type Server,
  type ServerResponse,
} from "node:http";
import { mkdtemp, mkdir, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";

import type { FastifyInstance } from "fastify";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { buildApp } from "../src/app.js";
import type { ServerConfig } from "../src/config.js";
import {
  clusterPodPhasePromql,
  clusterPodsCapacityPromql,
  clusterPodsUsedPromql,
  type ClusterPodPhase,
} from "../src/metrics-cluster-pods.js";

const testSessionSecret =
  "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef";

function prometheusSample(value: string) {
  return JSON.stringify({
    status: "success",
    data: {
      result: [
        {
          metric: {},
          value: ["1704067200", value],
        },
      ],
    },
  });
}

const defaultPhaseCounts: Record<ClusterPodPhase, string> = {
  Failed: "16",
  Pending: "12",
  Running: "500",
  Succeeded: "20",
  Unknown: "0",
};

function handleClusterPodsQuery(
  query: string | null,
  response: ServerResponse,
  options: {
    capacity?: string;
    phases?: Partial<Record<ClusterPodPhase, string>>;
    used?: string;
  } = {},
): boolean {
  response.setHeader("content-type", "application/json");
  if (query === clusterPodsCapacityPromql) {
    response.end(prometheusSample(options.capacity ?? "2000"));
    return true;
  }
  if (query === clusterPodsUsedPromql) {
    response.end(prometheusSample(options.used ?? "548"));
    return true;
  }
  for (const phase of [
    "Pending",
    "Running",
    "Succeeded",
    "Failed",
    "Unknown",
  ] as const) {
    if (query === clusterPodPhasePromql(phase)) {
      response.end(
        prometheusSample(options.phases?.[phase] ?? defaultPhaseCounts[phase]),
      );
      return true;
    }
  }
  return false;
}

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
        throw new Error("expected tcp listener address");
      }
      resolve({
        close: () => server.close(),
        issuer: `http://127.0.0.1:${String(address.port)}`,
      });
    });
  });
}

describe("GET /api/metrics/cluster-pods", () => {
  let app: FastifyInstance;
  let apiServer: Server;
  let prometheusServer: Server | undefined;
  let oidcServer: { close: () => void; issuer: string } | undefined;
  let staticRoot: string;
  let apiOrigin = "";

  beforeEach(async () => {
    apiServer = createServer((_request, response) => {
      response.setHeader("content-type", "application/json");
      response.end('{"kind":"GatewayList","items":[]}');
    });
    await new Promise<void>((resolve) => {
      apiServer.listen(0, "127.0.0.1", resolve);
    });
    const apiAddress = apiServer.address();
    if (apiAddress === null || typeof apiAddress === "string") {
      throw new Error("expected tcp listener address");
    }
    apiOrigin = `http://127.0.0.1:${String(apiAddress.port)}`;

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
        if (error) {
          reject(error);
        } else {
          resolve();
        }
      });
    });
    if (prometheusServer !== undefined) {
      await new Promise<void>((resolve, reject) => {
        prometheusServer?.close((error) => {
          if (error) {
            reject(error);
          } else {
            resolve();
          }
        });
      });
      prometheusServer = undefined;
    }
    oidcServer?.close();
    oidcServer = undefined;
  });

  async function buildTestApp(
    overrides: Partial<ServerConfig> = {},
  ): Promise<FastifyInstance> {
    const config: ServerConfig = {
      apiOrigin,
      apiTimeoutMs: 5_000,
      host: "127.0.0.1",
      logLevel: "silent",
      nodeEnv: "test",
      port: 8080,
      prometheusQueryTimeoutMs: 10_000,
      prometheusUrl: "http://127.0.0.1:9090",
      sessionTtlSeconds: 28_800,
      staticRoot,
      ...overrides,
    };
    return buildApp(config);
  }

  async function startPrometheusStub(
    handler: (request: IncomingMessage, response: ServerResponse) => void,
  ): Promise<string> {
    prometheusServer = createServer(handler);
    await new Promise<void>((resolve) => {
      prometheusServer?.listen(0, "127.0.0.1", resolve);
    });
    const address = prometheusServer.address();
    if (address === null || typeof address === "string") {
      throw new Error("expected tcp listener address");
    }
    return `http://127.0.0.1:${String(address.port)}`;
  }

  it("returns cluster pod counts when Prometheus succeeds", async () => {
    const prometheusUrl = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "", "http://127.0.0.1");
      const query = url.searchParams.get("query");
      if (handleClusterPodsQuery(query, response)) {
        return;
      }
      response.statusCode = 400;
      response.end();
    });

    app = await buildTestApp({ prometheusUrl });
    const response = await app.inject({
      method: "GET",
      url: "/api/metrics/cluster-pods",
    });

    expect(response.statusCode).toBe(200);
    expect(response.json()).toEqual({
      available_pods: 1452,
      capacity_pods: 2000,
      phase_failed_pods: 16,
      phase_pending_pods: 12,
      phase_running_pods: 500,
      phase_succeeded_pods: 20,
      phase_unknown_pods: 0,
      used_pods: 548,
    });
  });

  it("returns 502 when Prometheus fails", async () => {
    const prometheusUrl = await startPrometheusStub((_request, response) => {
      response.statusCode = 500;
      response.end();
    });

    app = await buildTestApp({ prometheusUrl });
    const response = await app.inject({
      method: "GET",
      url: "/api/metrics/cluster-pods",
    });

    expect(response.statusCode).toBe(502);
    expect(response.json()).toEqual({
      error: "Metrics unavailable",
      statusCode: 502,
    });
  });

  it("requires a session when OIDC is enabled", async () => {
    oidcServer = await createOidcServer();
    app = await buildTestApp({
      oidcClientId: "test-client",
      oidcIssuer: oidcServer.issuer,
      oidcRedirectUri: "http://127.0.0.1:8080/auth/callback",
      sessionSecret: Buffer.from(testSessionSecret, "hex"),
    });

    const response = await app.inject({
      method: "GET",
      url: "/api/metrics/cluster-pods",
    });

    expect(response.statusCode).toBe(401);
    expect(response.json()).toMatchObject({
      error: "reauth_required",
      statusCode: 401,
    });
  });

  it("allows dashboard administrators when OIDC is enabled", async () => {
    oidcServer = await createOidcServer();
    const prometheusUrl = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "", "http://127.0.0.1");
      const query = url.searchParams.get("query");
      if (
        handleClusterPodsQuery(query, response, {
          capacity: "100",
          phases: {
            Failed: "0",
            Pending: "2",
            Running: "40",
            Succeeded: "0",
            Unknown: "0",
          },
          used: "42",
        })
      ) {
        return;
      }
      response.statusCode = 400;
      response.end();
    });

    app = await buildTestApp({
      oidcClientId: "test-client",
      oidcIssuer: oidcServer.issuer,
      oidcRedirectUri: "http://127.0.0.1:8080/auth/callback",
      prometheusUrl,
      sessionSecret: Buffer.from(testSessionSecret, "hex"),
    });

    const session = app.createSecureSession({
      accessToken: "test-access-token",
      email: "test@example.com",
      expiresAt: Math.floor(Date.now() / 1000) + 3600,
      name: "Test User",
      preferredUsername: "testuser",
      roles: ["hypershell-admins"],
      sub: "user-123",
    });

    const response = await app.inject({
      headers: {
        cookie: `session=${encodeURIComponent(app.encodeSecureSession(session))}`,
      },
      method: "GET",
      url: "/api/metrics/cluster-pods",
    });

    expect(response.statusCode).toBe(200);
    expect(response.json()).toEqual({
      available_pods: 58,
      capacity_pods: 100,
      phase_failed_pods: 0,
      phase_pending_pods: 2,
      phase_running_pods: 40,
      phase_succeeded_pods: 0,
      phase_unknown_pods: 0,
      used_pods: 42,
    });
  });

  it("rejects authenticated non-admin callers when OIDC is enabled", async () => {
    oidcServer = await createOidcServer();
    app = await buildTestApp({
      oidcClientId: "test-client",
      oidcIssuer: oidcServer.issuer,
      oidcRedirectUri: "http://127.0.0.1:8080/auth/callback",
      sessionSecret: Buffer.from(testSessionSecret, "hex"),
    });

    const session = app.createSecureSession({
      accessToken: "test-access-token",
      expiresAt: Math.floor(Date.now() / 1000) + 3600,
      roles: ["hypershell-users"],
      sub: "user-123",
    });

    const response = await app.inject({
      headers: {
        cookie: `session=${encodeURIComponent(app.encodeSecureSession(session))}`,
      },
      method: "GET",
      url: "/api/metrics/cluster-pods",
    });

    expect(response.statusCode).toBe(403);
    expect(response.json()).toEqual({
      error: "Forbidden",
      statusCode: 403,
    });
  });
});
