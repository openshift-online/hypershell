import { createServer, type Server } from "node:http";
import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";

import type { FastifyInstance } from "fastify";

import { buildApp } from "../src/app.js";
import type { ServerConfig } from "../src/config.js";

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
