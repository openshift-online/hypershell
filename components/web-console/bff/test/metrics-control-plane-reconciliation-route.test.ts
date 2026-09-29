import { createServer, type Server } from "node:http";
import { mkdtemp, mkdir, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";

import type { FastifyInstance } from "fastify";
import { afterEach, describe, expect, it } from "vitest";

import { buildApp } from "../src/app.js";
import type { ServerConfig } from "../src/config.js";

describe("GET /api/metrics/control-plane-reconciliation", () => {
  let app: FastifyInstance | undefined;
  let prometheus: Server | undefined;
  let staticRoot: string;

  afterEach(async () => {
    await app?.close();
    await new Promise<void>((resolve) => {
      prometheus?.close(() => {
        resolve();
      });
    });
  });

  async function startPrometheus(statusCode: number): Promise<string> {
    prometheus = createServer((_request, response) => {
      response.statusCode = statusCode;
      response.setHeader("content-type", "application/json");
      response.end(JSON.stringify({ status: "error", data: { result: [] } }));
    });
    await new Promise<void>((resolve) =>
      prometheus?.listen(0, "127.0.0.1", resolve),
    );
    const address = prometheus.address();
    if (address === null || typeof address === "string")
      throw new Error("expected listener");
    return `http://127.0.0.1:${String(address.port)}`;
  }

  async function makeApp(prometheusUrl: string): Promise<FastifyInstance> {
    staticRoot = await mkdtemp(
      path.join(tmpdir(), "hypershell-reconciliation-route-"),
    );
    await mkdir(path.join(staticRoot, "assets"));
    await writeFile(
      path.join(staticRoot, "index.html"),
      "<!doctype html><html><body>App</body></html>",
    );
    const config: ServerConfig = {
      apiOrigin: "http://127.0.0.1:1",
      apiTimeoutMs: 5_000,
      host: "127.0.0.1",
      logLevel: "silent",
      nodeEnv: "test",
      port: 8080,
      prometheusQueryTimeoutMs: 1_000,
      prometheusUrl,
      sessionTtlSeconds: 28_800,
      staticRoot,
    };
    return buildApp(config);
  }

  it("returns 502 when a required reconciliation query fails", async () => {
    app = await makeApp(await startPrometheus(500));
    const response = await app.inject({
      method: "GET",
      url: "/api/metrics/control-plane-reconciliation",
    });
    expect(response.statusCode).toBe(502);
    expect(response.json()).toEqual({
      error: "Metrics unavailable",
      statusCode: 502,
    });
  });
});
