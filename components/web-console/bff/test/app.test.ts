import { createServer, type Server } from "node:http";
import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";

import type { FastifyInstance } from "fastify";

import { buildApp } from "../src/app.js";
import type { ServerConfig } from "../src/config.js";

async function waitFor(
  predicate: () => Promise<boolean>,
  {
    timeoutMs = 2_000,
    stepMs = 10,
  }: { timeoutMs?: number; stepMs?: number } = {},
): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    if (await predicate()) {
      return;
    }
    if (Date.now() > deadline) {
      throw new Error("condition was not met before the timeout");
    }
    await new Promise((resolve) => setTimeout(resolve, stepMs));
  }
}

describe("web-console BFF", () => {
  let app: FastifyInstance;
  let apiServer: Server;
  let staticRoot: string;
  let requests: {
    body: string;
    headers: Record<string, string | string[] | undefined>;
    method: string | undefined;
    url: string | undefined;
  }[];

  beforeEach(async () => {
    requests = [];
    apiServer = createServer((request, response) => {
      const chunks: Buffer[] = [];
      request.on("data", (chunk: Buffer) => chunks.push(chunk));
      request.on("end", () => {
        // The BFF probes the API metadata endpoint once at startup for the
        // version relay; it is not part of the request contract under test.
        if (request.url === "/api/hypershell") {
          response.setHeader("content-type", "application/json");
          response.end(
            '{"id":"hypershell","kind":"API","version":"test-sha","build_time":"2026-01-01T00:00:00Z"}',
          );
          return;
        }
        requests.push({
          body: Buffer.concat(chunks).toString("utf8"),
          headers: request.headers,
          method: request.method,
          url: request.url,
        });
        response.setHeader("content-type", "application/json; charset=utf-8");
        response.setHeader(
          "x-hypershell-correlation-id",
          request.headers["x-hypershell-correlation-id"] ?? "",
        );
        if (request.url === "/api/slow") {
          setTimeout(() => {
            response.end('{"late":true}');
          }, 250);
          return;
        }
        if (request.url === "/api/unknown") {
          response.statusCode = 404;
          response.end('{"error":"upstream not found"}');
          return;
        }
        if (
          request.method === "POST" &&
          request.url ===
            "/api/hypershell/v1/gateways/gateway-1/service_accounts"
        ) {
          response.setHeader("cache-control", "no-store");
          response.setHeader("pragma", "no-cache");
          response.statusCode = 201;
          response.end('{"credential":{"client_secret":"one-time"}}');
          return;
        }
        response.statusCode = request.method === "PATCH" ? 202 : 200;
        response.end('{"kind":"GatewayList","items":[]}');
      });
    });
    await new Promise<void>((resolve) => {
      apiServer.listen(0, "127.0.0.1", resolve);
    });
    const address = apiServer.address();
    if (address === null || typeof address === "string") {
      throw new Error("Expected the test API server to use a TCP address");
    }

    staticRoot = await mkdtemp(path.join(tmpdir(), "hypershell-web-console-"));
    await mkdir(path.join(staticRoot, "assets"));
    await writeFile(
      path.join(staticRoot, "index.html"),
      "<!doctype html><html><head><title>console</title></head><body><script>globalThis.ready = true;</script><main>Hello world</main></body></html>",
    );
    await writeFile(
      path.join(staticRoot, "assets", "app-deadbeef.js"),
      'console.log("asset");',
    );

    const config: ServerConfig = {
      apiOrigin: `http://127.0.0.1:${String(address.port)}`,
      apiTimeoutMs: 100,
      host: "127.0.0.1",
      logLevel: "silent",
      nodeEnv: "test",
      port: 8080,
      prometheusQueryTimeoutMs: 10_000,
      prometheusUrl: "http://127.0.0.1:9090",
      sessionTtlSeconds: 28_800,
      staticRoot,
      webVersion: "unknown",
    };
    app = await buildApp(config);
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
    await rm(staticRoot, { force: true, recursive: true });
  });

  it("serves health probes without caching", async () => {
    const live = await app.inject({ method: "GET", url: "/health/live" });
    const ready = await app.inject({ method: "GET", url: "/health/ready" });

    expect(live.statusCode).toBe(200);
    expect(live.headers["cache-control"]).toBe("no-store");
    expect(ready.statusCode).toBe(200);
  });

  it("serves known application routes with an enforcing CSP and no-store HTML", async () => {
    const routeContract = JSON.parse(
      await readFile(
        new URL("../../route-contract.json", import.meta.url),
        "utf8",
      ),
    ) as { directNavigationExamples: string[] };

    for (const route of routeContract.directNavigationExamples) {
      const response = await app.inject({ method: "GET", url: route });
      expect(response.statusCode, route).toBe(200);
      expect(response.headers["content-type"], route).toContain("text/html");
      expect(response.headers["cache-control"], route).toBe("no-store");
      expect(response.headers["content-security-policy"], route).toContain(
        "default-src 'none'",
      );
      expect(response.headers["content-security-policy"], route).toContain(
        "'sha256-",
      );
      expect(response.headers["content-security-policy"], route).not.toContain(
        "upgrade-insecure-requests",
      );
      expect(response.headers["permissions-policy"], route).toContain(
        "camera=()",
      );
    }
  });

  it("injects only the allowlisted runtime config as a head meta tag", async () => {
    // The default harness configures no collector, so the browser must be told
    // to sample nothing rather than defaulting to recording every trace.
    const response = await app.inject({ method: "GET", url: "/" });

    expect(response.statusCode).toBe(200);
    expect(response.body).toContain('name="hypershell-runtime-config"');
    expect(response.body).toContain("&quot;sampleRatio&quot;:0");
    // The meta tag lands in the head, before the application markup.
    expect(response.body.indexOf("hypershell-runtime-config")).toBeLessThan(
      response.body.indexOf("<main>"),
    );
    // The runtime config surface adds no inline script, so the CSP still admits
    // only the pre-existing hashed inline script.
    expect(response.headers["content-security-policy"]).toContain("'sha256-");
  });

  it("flows the configured sample ratio into the browser runtime config", async () => {
    const tracedApp = await buildApp({
      apiOrigin: "http://127.0.0.1:1",
      apiTimeoutMs: 100,
      host: "127.0.0.1",
      logLevel: "silent",
      nodeEnv: "test",
      port: 8080,
      prometheusQueryTimeoutMs: 10_000,
      prometheusUrl: "http://127.0.0.1:9090",
      sessionTtlSeconds: 28_800,
      staticRoot,
      webVersion: "unknown",
      tracing: {
        collectorEndpoint: "http://collector.invalid:4318",
        sampleRatio: 0.5,
        serviceName: "hypershell-web-console-bff",
        tracesEndpoint: "http://collector.invalid:4318/v1/traces",
      },
    });

    try {
      const response = await tracedApp.inject({ method: "GET", url: "/" });

      expect(response.body).toContain("&quot;sampleRatio&quot;:0.5");
      // The collector endpoint stays server-side and never reaches the document.
      expect(response.body).not.toContain("collector.invalid");
    } finally {
      await tracedApp.close();
    }
  });

  it("relays the API and console build versions into the runtime config", async () => {
    const response = await app.inject({ method: "GET", url: "/" });

    // The upstream stub serves version "test-sha" on /api/hypershell; the
    // console version is unset in this environment and degrades to unknown.
    expect(response.body).toContain(
      "&quot;apiVersion&quot;:&quot;test-sha&quot;",
    );
    expect(response.body).toContain(
      "&quot;webVersion&quot;:&quot;unknown&quot;",
    );
  });

  it("degrades to unknown when the API metadata endpoint is unreachable", async () => {
    const config: ServerConfig = {
      apiOrigin: "http://127.0.0.1:1",
      apiTimeoutMs: 100,
      host: "127.0.0.1",
      logLevel: "silent",
      nodeEnv: "test",
      port: 8080,
      prometheusQueryTimeoutMs: 10_000,
      prometheusUrl: "http://127.0.0.1:9090",
      sessionTtlSeconds: 28_800,
      staticRoot,
      webVersion: "unknown",
    };
    const unreachableApp = await buildApp(config);
    try {
      const response = await unreachableApp.inject({
        method: "GET",
        url: "/",
      });
      expect(response.body).toContain(
        "&quot;apiVersion&quot;:&quot;unknown&quot;",
      );
    } finally {
      await unreachableApp.close();
    }
  });

  it("does not hold startup on an API that accepts the socket but stalls", async () => {
    // Accepts connections and never responds.
    const stalledServer = createServer(() => undefined);
    await new Promise<void>((resolve) =>
      stalledServer.listen(0, "127.0.0.1", resolve),
    );
    const address = stalledServer.address();
    if (!address || typeof address === "string") {
      throw new Error("stalled server did not bind");
    }
    const config: ServerConfig = {
      apiOrigin: `http://127.0.0.1:${String(address.port)}`,
      apiTimeoutMs: 30_000,
      host: "127.0.0.1",
      logLevel: "silent",
      nodeEnv: "test",
      port: 8080,
      prometheusQueryTimeoutMs: 10_000,
      prometheusUrl: "http://127.0.0.1:9090",
      sessionTtlSeconds: 28_800,
      staticRoot,
      webVersion: "unknown",
    };
    const startedAt = Date.now();
    const stalledApp = await buildApp(config);
    try {
      // Bounded by the dedicated probe timeout, not the 30s proxy timeout.
      expect(Date.now() - startedAt).toBeLessThan(5_000);
      const response = await stalledApp.inject({ method: "GET", url: "/" });
      expect(response.body).toContain(
        "&quot;apiVersion&quot;:&quot;unknown&quot;",
      );
    } finally {
      await stalledApp.close();
      stalledServer.closeAllConnections();
      await new Promise<void>((resolve) => {
        stalledServer.close(() => {
          resolve();
        });
      });
    }
  });

  it("refreshes the API build version into the served index without a BFF restart", async () => {
    let reportedVersion = "aaaaaaa";
    const versionServer = createServer((request, response) => {
      if (request.url === "/api/hypershell") {
        response.setHeader("content-type", "application/json");
        response.end(JSON.stringify({ version: reportedVersion }));
        return;
      }
      response.statusCode = 404;
      response.end("{}");
    });
    await new Promise<void>((resolve) =>
      versionServer.listen(0, "127.0.0.1", resolve),
    );
    const address = versionServer.address();
    if (!address || typeof address === "string") {
      throw new Error("version server did not bind");
    }
    const config: ServerConfig = {
      apiOrigin: `http://127.0.0.1:${String(address.port)}`,
      apiTimeoutMs: 100,
      // Tiny interval so the background refresh runs within the test window.
      apiVersionRefreshIntervalMs: 20,
      host: "127.0.0.1",
      logLevel: "silent",
      nodeEnv: "test",
      port: 8080,
      prometheusQueryTimeoutMs: 10_000,
      prometheusUrl: "http://127.0.0.1:9090",
      sessionTtlSeconds: 28_800,
      staticRoot,
      webVersion: "unknown",
    };
    const refreshingApp = await buildApp(config);
    try {
      const initial = await refreshingApp.inject({ method: "GET", url: "/" });
      expect(initial.body).toContain(
        "&quot;apiVersion&quot;:&quot;aaaaaaa&quot;",
      );

      // The api-server is rolled to a new build; the BFF must pick it up on a
      // later refresh and re-render the served index in place -- no restart.
      reportedVersion = "bbbbbbb";
      await waitFor(async () => {
        const response = await refreshingApp.inject({
          method: "GET",
          url: "/",
        });
        return response.body.includes(
          "&quot;apiVersion&quot;:&quot;bbbbbbb&quot;",
        );
      });
    } finally {
      await refreshingApp.close();
      await new Promise<void>((resolve) => {
        versionServer.close(() => {
          resolve();
        });
      });
    }
  });

  it("keeps the last known API build version when a refresh probe fails", async () => {
    let healthy = true;
    const flakyServer = createServer((request, response) => {
      if (request.url === "/api/hypershell") {
        if (!healthy) {
          response.statusCode = 503;
          response.end("{}");
          return;
        }
        response.setHeader("content-type", "application/json");
        response.end(JSON.stringify({ version: "ccccccc" }));
        return;
      }
      response.statusCode = 404;
      response.end("{}");
    });
    await new Promise<void>((resolve) =>
      flakyServer.listen(0, "127.0.0.1", resolve),
    );
    const address = flakyServer.address();
    if (!address || typeof address === "string") {
      throw new Error("flaky server did not bind");
    }
    const config: ServerConfig = {
      apiOrigin: `http://127.0.0.1:${String(address.port)}`,
      apiTimeoutMs: 100,
      apiVersionRefreshIntervalMs: 20,
      host: "127.0.0.1",
      logLevel: "silent",
      nodeEnv: "test",
      port: 8080,
      prometheusQueryTimeoutMs: 10_000,
      prometheusUrl: "http://127.0.0.1:9090",
      sessionTtlSeconds: 28_800,
      staticRoot,
      webVersion: "unknown",
    };
    const flakyApp = await buildApp(config);
    try {
      const initial = await flakyApp.inject({ method: "GET", url: "/" });
      expect(initial.body).toContain(
        "&quot;apiVersion&quot;:&quot;ccccccc&quot;",
      );

      // Subsequent probes fail; the display must hold the last known value
      // rather than regressing a good build to "unknown".
      healthy = false;
      await new Promise((resolve) => setTimeout(resolve, 150));
      const after = await flakyApp.inject({ method: "GET", url: "/" });
      expect(after.body).toContain(
        "&quot;apiVersion&quot;:&quot;ccccccc&quot;",
      );
      expect(after.body).not.toContain(
        "&quot;apiVersion&quot;:&quot;unknown&quot;",
      );
    } finally {
      await flakyApp.close();
      await new Promise<void>((resolve) => {
        flakyServer.close(() => {
          resolve();
        });
      });
    }
  });

  it("keeps assets immutable and does not fall back for unknown routes", async () => {
    const asset = await app.inject({
      method: "GET",
      url: "/assets/app-deadbeef.js",
    });
    const unknown = await app.inject({ method: "GET", url: "/unknown" });
    const obsolete = await app.inject({ method: "GET", url: "/fleets/a" });

    expect(asset.statusCode).toBe(200);
    expect(asset.headers["cache-control"]).toContain("immutable");
    expect(unknown.statusCode).toBe(404);
    expect(obsolete.statusCode).toBe(404);
  });

  it("proxies API reads and mutations to the configured origin", async () => {
    const correlationId = "11111111-1111-4111-8111-111111111111";
    const list = await app.inject({
      headers: {
        authorization: "Bearer browser-secret",
        cookie: "browser-session=secret",
        "x-hypershell-correlation-id": correlationId,
      },
      method: "GET",
      url: "/api/hypershell/v1/gateways?page=2&size=20",
    });
    const update = await app.inject({
      headers: { "content-type": "application/json" },
      method: "PATCH",
      payload: { name: "renamed" },
      url: "/api/hypershell/v1/gateways/gateway-1",
    });

    expect(list.statusCode).toBe(200);
    expect(list.json()).toEqual({ kind: "GatewayList", items: [] });
    expect(list.headers["x-hypershell-correlation-id"]).toBe(correlationId);
    expect(update.statusCode).toBe(202);
    expect(requests).toHaveLength(2);
    expect(requests[0]).toMatchObject({
      body: "",
      method: "GET",
      url: "/api/hypershell/v1/gateways?page=2&size=20",
    });
    expect(requests[0]?.headers.authorization).toBeUndefined();
    expect(requests[0]?.headers.cookie).toBeUndefined();
    expect(requests[0]?.headers["x-hypershell-correlation-id"]).toBe(
      correlationId,
    );
    expect(requests[1]).toMatchObject({
      body: '{"name":"renamed"}',
      method: "PATCH",
      url: "/api/hypershell/v1/gateways/gateway-1",
    });
  });

  it("forwards one-time credential cache protections", async () => {
    const response = await app.inject({
      headers: { "content-type": "application/json" },
      method: "POST",
      payload: { name: "deploy-bot", role: "openshell-user" },
      url: "/api/hypershell/v1/gateways/gateway-1/service_accounts",
    });

    expect(response.statusCode).toBe(201);
    expect(response.headers["cache-control"]).toBe("no-store");
    expect(response.headers.pragma).toBe("no-cache");
  });

  it("replaces malformed correlation identifiers", async () => {
    const response = await app.inject({
      headers: { "x-hypershell-correlation-id": "not-valid" },
      method: "GET",
      url: "/api/hypershell/v1/gateways",
    });
    const replacement = response.headers["x-hypershell-correlation-id"];

    expect(replacement).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/iu,
    );
    expect(requests[0]?.headers["x-hypershell-correlation-id"]).toBe(
      replacement,
    );
  });

  it("preserves upstream failures and bounds upstream response time", async () => {
    const missing = await app.inject({ method: "GET", url: "/api/unknown" });
    const slow = await app.inject({ method: "GET", url: "/api/slow" });

    expect(missing.statusCode).toBe(404);
    expect(missing.json()).toEqual({ error: "upstream not found" });
    expect(slow.statusCode).toBe(504);
    expect(slow.json()).toEqual({
      error: "Gateway Timeout",
      statusCode: 504,
    });
  });

  it("rejects oversized browser payloads before they reach the API", async () => {
    const response = await app.inject({
      headers: { "content-type": "application/json" },
      method: "POST",
      payload: JSON.stringify({ value: "x".repeat(1_048_576) }),
      url: "/api/hypershell/v1/gateways",
    });

    expect(response.statusCode).toBe(413);
    expect(requests).toHaveLength(0);
  });
});
