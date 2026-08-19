import { createServer, type Server } from "node:http";
import { mkdtemp, mkdir, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";

import type { FastifyInstance } from "fastify";

import { buildApp } from "../src/app.js";
import type { ServerConfig } from "../src/config.js";
import type {
  BffTracing,
  ProxyOutcome,
  StartProxySpanInput,
} from "../src/tracing.js";

const upstreamTraceparent =
  "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01";

function stubTracing() {
  const started: StartProxySpanInput[] = [];
  const ended: { outcome: ProxyOutcome; statusCode: number }[] = [];
  const ingested: unknown[] = [];
  const health = { relayFailures: 0, spanExportFailures: 0 };
  const tracing: BffTracing = {
    deliveryHealth: () => ({ ...health }),
    enabled: true,
    ingestTraces: (payload) => {
      ingested.push(payload);
      return Promise.resolve(
        Array.isArray((payload as { resourceSpans?: unknown }).resourceSpans)
          ? "accepted"
          : "rejected",
      );
    },
    shutdown: () => Promise.resolve(),
    startProxySpan: (input) => {
      started.push(input);
      return {
        end: (outcome, statusCode) => {
          ended.push({ outcome, statusCode });
        },
        upstream: () => ({
          traceparent: upstreamTraceparent,
          tracestate: "vendor=1",
        }),
      };
    },
  };
  return { ended, health, ingested, started, tracing };
}

describe("web-console BFF tracing wiring", () => {
  let app: FastifyInstance;
  let apiServer: Server;
  let staticRoot: string;
  let received: { headers: Record<string, string | string[] | undefined> }[];
  let trace: ReturnType<typeof stubTracing>;
  let upstreamStatus: number;

  beforeEach(async () => {
    received = [];
    upstreamStatus = 200;
    apiServer = createServer((request, response) => {
      request.on("data", () => undefined);
      request.on("end", () => {
        received.push({ headers: request.headers });
        response.statusCode = upstreamStatus;
        response.setHeader("content-type", "application/json");
        response.end('{"items":[]}');
      });
    });
    await new Promise<void>((resolve) => {
      apiServer.listen(0, "127.0.0.1", resolve);
    });
    const address = apiServer.address();
    if (address === null || typeof address === "string") {
      throw new Error("Expected the test API server to use a TCP address");
    }

    staticRoot = await mkdtemp(path.join(tmpdir(), "hypershell-bff-trace-"));
    await mkdir(path.join(staticRoot, "assets"));
    await writeFile(
      path.join(staticRoot, "index.html"),
      "<!doctype html><html><body><main>Hello</main></body></html>",
    );

    const config: ServerConfig = {
      apiOrigin: `http://127.0.0.1:${String(address.port)}`,
      apiTimeoutMs: 5_000,
      host: "127.0.0.1",
      logLevel: "silent",
      nodeEnv: "test",
      port: 8080,
      sessionTtlSeconds: 28_800,
      staticRoot,
    };
    trace = stubTracing();
    app = await buildApp(config, trace.tracing);
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

  it("propagates the span-derived trace context to the upstream API", async () => {
    const response = await app.inject({
      headers: { traceparent: "00-inbound-ignored" },
      method: "GET",
      url: "/api/hypershell/v1/gateways",
    });

    expect(response.statusCode).toBe(200);
    expect(received).toHaveLength(1);
    expect(received[0]?.headers.traceparent).toBe(upstreamTraceparent);
    expect(received[0]?.headers.tracestate).toBe("vendor=1");
  });

  it("starts a span per proxied request and ends it with the outcome", async () => {
    await app.inject({
      headers: { traceparent: "00-inbound-value" },
      method: "GET",
      url: "/api/hypershell/v1/gateways",
    });

    expect(trace.started).toHaveLength(1);
    expect(trace.started[0]).toMatchObject({
      method: "GET",
      path: "/api/hypershell/v1/gateways",
      traceparent: "00-inbound-value",
    });
    expect(trace.started[0]?.correlationId).toBeTruthy();
    expect(trace.ended).toEqual([{ outcome: "success", statusCode: 200 }]);
  });

  it("records a server-error outcome when the upstream fails", async () => {
    upstreamStatus = 502;

    const response = await app.inject({
      method: "GET",
      url: "/api/hypershell/v1/gateways",
    });

    expect(response.statusCode).toBe(502);
    expect(trace.ended).toEqual([{ outcome: "server_error", statusCode: 502 }]);
  });

  it("keeps request secrets out of the span, recording only the bounded route", async () => {
    const secret = "SEED-Bearer-token-do-not-log";

    await app.inject({
      method: "GET",
      url: `/api/hypershell/v1/gateways?access_token=${secret}&user=alice`,
    });

    expect(trace.started).toHaveLength(1);
    // The span receives the path without its query string; the adapter renders
    // the bounded route template from it, so no raw URL or query reaches it.
    expect(trace.started[0]?.path).toBe("/api/hypershell/v1/gateways");
    // No field fed to the span carries the secret or the raw query string.
    expect(JSON.stringify(trace.started[0])).not.toContain(secret);
    expect(JSON.stringify(trace.started[0])).not.toContain("access_token");
  });

  it("accepts a well-formed OTLP payload at the ingest endpoint", async () => {
    const response = await app.inject({
      method: "POST",
      payload: { resourceSpans: [{ scopeSpans: [] }] },
      url: "/telemetry/v1/traces",
    });

    expect(response.statusCode).toBe(202);
    expect(response.headers["cache-control"]).toBe("no-store");
    expect(trace.ingested).toHaveLength(1);
  });

  it("rejects a malformed telemetry payload", async () => {
    const response = await app.inject({
      method: "POST",
      payload: { notOtlp: true },
      url: "/telemetry/v1/traces",
    });

    expect(response.statusCode).toBe(400);
  });

  it("surfaces the delivery-health snapshot on readiness without gating it", async () => {
    trace.health.relayFailures = 3;
    trace.health.spanExportFailures = 5;

    const response = await app.inject({ method: "GET", url: "/health/ready" });

    // Readiness stays "ready" even with delivery losses -- tracing is
    // best-effort and must never gate serving -- but the bounded snapshot rides
    // along so the losses are observable.
    expect(response.statusCode).toBe(200);
    expect(response.json()).toEqual({
      status: "ready",
      tracing: { relayFailures: 3, spanExportFailures: 5 },
    });
  });
});
