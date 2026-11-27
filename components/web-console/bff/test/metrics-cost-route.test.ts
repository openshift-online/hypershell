import { createServer, type Server } from "node:http";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";

import type { FastifyInstance } from "fastify";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { buildApp } from "../src/app.js";
import type { ServerConfig } from "../src/config.js";
import {
  rateCardFromDocument,
  type RateCardSource,
  type RateCardState,
} from "../src/cost-rate-card.js";

const testSessionSecret =
  "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef";

function listen(server: Server): Promise<string> {
  return new Promise((resolve) => {
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      if (address === null || typeof address === "string") {
        throw new Error("expected TCP listener");
      }
      resolve(`http://127.0.0.1:${String(address.port)}`);
    });
  });
}

function close(server: Server): Promise<void> {
  return new Promise((resolve, reject) => {
    server.close((error) => {
      if (error) reject(error);
      else resolve();
    });
  });
}

function vector(result: { metric: Record<string, string>; value: number }[]) {
  return JSON.stringify({
    status: "success",
    data: {
      resultType: "vector",
      result: result.map((sample) => ({
        metric: sample.metric,
        value: [1704067200, String(sample.value)],
      })),
    },
  });
}

const cluster = { cluster: "c1", environment: "production", provider: "ibm" };

const readyRateCard: RateCardState = {
  status: "ready",
  rateCard: rateCardFromDocument({
    currency: "USD",
    version: "test-rates",
    defaults: { cpu_core_hour: 0.03, memory_gib_hour: 0 },
  }),
};

function staticRates(state: RateCardState): RateCardSource {
  return { close: () => undefined, current: () => state };
}

describe("cost metrics routes", () => {
  let app: FastifyInstance;
  let apiServer: Server;
  let prometheusServer: Server;
  let oidcServer: Server;
  let staticRoot: string;
  let apiOrigin: string;
  let prometheusOrigin: string;
  let oidcIssuer: string;
  let prometheusQueries: string[];
  let apiRequests: string[];
  let gateways: {
    id: string;
    namespace: string;
    name: string;
    workspace_mode: string;
  }[];
  let access: Record<
    string,
    {
      role: string;
      username: string;
      granted_at: string;
      role_binding_id: string;
    }[]
  >;

  beforeEach(async () => {
    prometheusQueries = [];
    apiRequests = [];
    gateways = [
      {
        id: "a",
        name: "Alpha",
        namespace: "openshell-a",
        workspace_mode: "managed",
      },
      {
        id: "b",
        name: "Beta",
        namespace: "openshell-b",
        workspace_mode: "managed",
      },
    ];
    access = {
      a: [
        {
          role: "owner",
          username: "alice",
          granted_at: "2026-01-01T00:00:00Z",
          role_binding_id: "rb1",
        },
        {
          role: "owner",
          username: "bob",
          granted_at: "2026-02-01T00:00:00Z",
          role_binding_id: "rb2",
        },
      ],
      b: [
        {
          role: "owner",
          username: "carol",
          granted_at: "2026-01-01T00:00:00Z",
          role_binding_id: "rb3",
        },
        {
          role: "user",
          username: "alice",
          granted_at: "2025-06-01T00:00:00Z",
          role_binding_id: "rb4",
        },
      ],
    };

    apiServer = createServer((request, response) => {
      response.setHeader("content-type", "application/json");
      if (request.url === "/api/hypershell") {
        response.end('{"id":"hypershell","kind":"API","version":"test"}');
        return;
      }
      apiRequests.push(request.url ?? "");
      const url = new URL(request.url ?? "/", "http://127.0.0.1");
      const accessMatch =
        /^\/api\/hypershell\/v1\/gateways\/([^/]+)\/access$/u.exec(
          url.pathname,
        );
      if (accessMatch) {
        const role = url.searchParams.get("role");
        const items = (access[accessMatch[1] ?? ""] ?? [])
          .filter((item) => role === null || item.role === role)
          .map((item) => ({ ...item, user_id: `id-${item.username}` }));
        response.end(
          JSON.stringify({
            capabilities: {},
            items,
            page: 1,
            size: 100,
            total: items.length,
          }),
        );
        return;
      }
      if (url.pathname === "/api/hypershell/v1/gateways") {
        response.end(
          JSON.stringify({ items: gateways, page: 1, total: gateways.length }),
        );
        return;
      }
      response.end(JSON.stringify({ items: [], total: 0 }));
    });
    apiOrigin = await listen(apiServer);

    prometheusServer = createServer((request, response) => {
      const url = new URL(request.url ?? "/", "http://127.0.0.1");
      const query = url.searchParams.get("query") ?? "";
      prometheusQueries.push(query);
      response.setHeader("content-type", "application/json");
      if (url.pathname === "/api/v1/query_range") {
        response.end(
          JSON.stringify({ status: "success", data: { result: [] } }),
        );
        return;
      }
      if (
        query.includes("namespace=~") &&
        query.includes("namespace_cpu_requests")
      ) {
        response.end(
          vector([
            { metric: { ...cluster, namespace: "openshell-a" }, value: 10 },
            { metric: { ...cluster, namespace: "openshell-a-ws" }, value: 20 },
            { metric: { ...cluster, namespace: "openshell-b" }, value: 5 },
            { metric: { ...cluster, namespace: "openshell-b-ws" }, value: 5 },
          ]),
        );
        return;
      }
      if (
        query.includes("cluster_cpu_allocatable") &&
        !query.includes("timestamp")
      ) {
        response.end(vector([{ metric: cluster, value: 100 }]));
        return;
      }
      if (query.includes("cluster_memory_allocatable")) {
        response.end(vector([{ metric: cluster, value: 0 }]));
        return;
      }
      if (query.includes("namespace_cpu_requests")) {
        response.end(vector([{ metric: cluster, value: 60 }]));
        return;
      }
      response.end(vector([]));
    });
    prometheusOrigin = await listen(prometheusServer);

    oidcServer = createServer((request, response) => {
      response.setHeader("content-type", "application/json");
      if (request.url === "/.well-known/openid-configuration") {
        response.end(
          JSON.stringify({
            authorization_endpoint: `${oidcIssuer}/authorize`,
            issuer: oidcIssuer,
            jwks_uri: `${oidcIssuer}/jwks`,
            token_endpoint: `${oidcIssuer}/token`,
          }),
        );
        return;
      }
      response.end(JSON.stringify({ keys: [] }));
    });
    oidcIssuer = await listen(oidcServer);

    staticRoot = await mkdtemp(path.join(tmpdir(), "hypershell-web-console-"));
    await mkdir(path.join(staticRoot, "assets"));
    await writeFile(
      path.join(staticRoot, "index.html"),
      "<!doctype html><html><body></body></html>",
    );
  });

  afterEach(async () => {
    await app.close();
    await close(apiServer);
    await close(prometheusServer);
    await close(oidcServer);
    await rm(staticRoot, { force: true, recursive: true });
  });

  async function buildTestApp(rates: RateCardState = readyRateCard) {
    const config: ServerConfig = {
      apiOrigin,
      apiTimeoutMs: 5_000,
      host: "127.0.0.1",
      logLevel: "silent",
      nodeEnv: "test",
      oidcClientId: "test-client",
      oidcIssuer,
      oidcRedirectUri: "http://127.0.0.1:8080/auth/callback",
      port: 8080,
      prometheusQueryTimeoutMs: 5_000,
      prometheusUrl: prometheusOrigin,
      sessionSecret: Buffer.from(testSessionSecret, "hex"),
      sessionTtlSeconds: 28_800,
      staticRoot,
      webVersion: "unknown",
      webBuildTime: "unknown",
    };
    return buildApp(config, undefined, staticRates(rates));
  }

  function cookie(username: string, roles: string[] = ["hypershell-users"]) {
    const session = app.createSecureSession({
      accessToken: "test-access-token",
      expiresAt: Math.floor(Date.now() / 1000) + 3600,
      preferredUsername: username,
      roles,
      sub: `sub-${username}`,
    });
    return `session=${encodeURIComponent(app.encodeSecureSession(session))}`;
  }

  // Verifies: WEB-COST-02.01
  it("forbids platform cost routes to non-admins", async () => {
    app = await buildTestApp();
    for (const route of ["summary", "breakdown", "gateways"]) {
      const response = await app.inject({
        headers: { cookie: cookie("alice") },
        method: "GET",
        url: `/api/metrics/cost/${route}`,
      });
      expect(response.statusCode).toBe(403);
    }
    expect(prometheusQueries).toEqual([]);
  });

  // Verifies: WEB-COST-03.01
  it("returns not-configured without a rate card", async () => {
    app = await buildTestApp({ status: "not-configured" });
    const response = await app.inject({
      headers: { cookie: cookie("admin", ["platform:admin"]) },
      method: "GET",
      url: "/api/metrics/cost/summary",
    });
    expect(response.statusCode).toBe(200);
    expect(response.json()).toEqual({
      status: "not-configured",
      estimate: true,
    });
  });

  it("fails readiness for an invalid rate card", async () => {
    app = await buildTestApp({ status: "invalid", error: "bad" });
    const response = await app.inject({ method: "GET", url: "/health/ready" });
    expect(response.statusCode).toBe(503);
  });

  it("serves the platform summary to an admin without exposing rates", async () => {
    app = await buildTestApp();
    const response = await app.inject({
      headers: { cookie: cookie("admin", ["platform:admin"]) },
      method: "GET",
      url: "/api/metrics/cost/summary",
    });
    expect(response.statusCode).toBe(200);
    const body = response.json<Record<string, unknown>>();
    expect(body).toMatchObject({
      status: "ok",
      estimate: true,
      currency: "USD",
      rate_card_version: "test-rates",
    });
    expect(JSON.stringify(body)).not.toContain("cpu_core_hour");
    expect(body.monthly_total).toBeGreaterThan(0);
  });

  // Verifies: WEB-COST-09.01, WEB-COST-09.02
  it("charges each gateway to its oldest owner and scopes queries", async () => {
    app = await buildTestApp();
    const summary = await app.inject({
      headers: { cookie: cookie("alice") },
      method: "GET",
      url: "/api/metrics/cost/me/summary",
    });
    expect(summary.statusCode).toBe(200);
    const body = summary.json<{
      charged: { gateway_count: number };
      shared: { gateway_count: number };
    }>();
    expect(body.charged.gateway_count).toBe(1);
    expect(body.shared.gateway_count).toBe(1);

    const scopedQueries = prometheusQueries.filter((query) =>
      query.includes("namespace_cpu_requests"),
    );
    expect(scopedQueries.length).toBeGreaterThan(0);
    // Every namespace query is scoped to the caller's gateways (the charged
    // trend to gateway A alone), never the fleet.
    for (const query of scopedQueries) {
      expect(query).toMatch(/namespace=~"\(openshell-a(\|openshell-b)?\)/u);
    }

    const bob = await app.inject({
      headers: { cookie: cookie("bob") },
      method: "GET",
      url: "/api/metrics/cost/me/gateways",
    });
    const bobBody = bob.json<{
      gateways: {
        id: string;
        charged: boolean;
        billing_owner?: string;
        role: string;
      }[];
      workspaces: { workspace: string }[];
    }>();
    // Bob co-owns A but is not its billing owner; he holds no binding on B.
    expect(bobBody.gateways).toEqual([
      expect.objectContaining({
        id: "a",
        charged: false,
        billing_owner: "alice",
        role: "owner",
      }),
    ]);
    expect(bobBody.workspaces.map((row) => row.workspace)).toEqual(["ws"]);
  });

  it("hides workspaces on gateways the caller only views", async () => {
    app = await buildTestApp();
    const response = await app.inject({
      headers: { cookie: cookie("alice") },
      method: "GET",
      url: "/api/metrics/cost/me/gateways",
    });
    const body = response.json<{ workspaces: { gateway_id: string }[] }>();
    expect(body.workspaces.every((row) => row.gateway_id === "a")).toBe(true);
  });

  it("shows an admin only their own bindings in My cost", async () => {
    app = await buildTestApp();
    const response = await app.inject({
      headers: { cookie: cookie("dave", ["platform:admin"]) },
      method: "GET",
      url: "/api/metrics/cost/me/summary",
    });
    expect(response.json()).toMatchObject({
      empty: true,
      charged: { gateway_count: 0 },
      shared: { gateway_count: 0 },
    });
  });

  it("requires a session for My cost", async () => {
    app = await buildTestApp();
    const response = await app.inject({
      method: "GET",
      url: "/api/metrics/cost/me/summary",
    });
    expect(response.statusCode).toBe(401);
    expect(apiRequests).toEqual([]);
  });
});
