import { mkdtemp, mkdir, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";

import type { FastifyInstance } from "fastify";

import { buildApp } from "../src/app.js";
import type { ServerConfig } from "../src/config.js";

describe("web-console BFF", () => {
  let app: FastifyInstance;
  let staticRoot: string;

  beforeEach(async () => {
    staticRoot = await mkdtemp(path.join(tmpdir(), "hypershell-web-console-"));
    await mkdir(path.join(staticRoot, "assets"));
    await writeFile(
      path.join(staticRoot, "index.html"),
      "<!doctype html><html><body><script>globalThis.ready = true;</script><main>Hello world</main></body></html>",
    );
    await writeFile(
      path.join(staticRoot, "assets", "app-deadbeef.js"),
      'console.log("asset");',
    );

    const config: ServerConfig = {
      host: "127.0.0.1",
      logLevel: "silent",
      nodeEnv: "test",
      port: 8080,
      staticRoot,
    };
    app = await buildApp(config);
  });

  afterEach(async () => {
    await app.close();
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
    const response = await app.inject({
      method: "GET",
      url: "/fleets/fleet-a/gateways/gateway-a",
    });

    expect(response.statusCode).toBe(200);
    expect(response.headers["content-type"]).toContain("text/html");
    expect(response.headers["cache-control"]).toBe("no-store");
    expect(response.headers["content-security-policy"]).toContain(
      "default-src 'none'",
    );
    expect(response.headers["content-security-policy"]).toContain("'sha256-");
    expect(response.headers["content-security-policy"]).not.toContain(
      "upgrade-insecure-requests",
    );
    expect(response.headers["permissions-policy"]).toContain("camera=()");
  });

  it("keeps assets immutable and does not fall back for unknown or API routes", async () => {
    const asset = await app.inject({
      method: "GET",
      url: "/assets/app-deadbeef.js",
    });
    const api = await app.inject({ method: "GET", url: "/api/unknown" });
    const unknown = await app.inject({ method: "GET", url: "/unknown" });

    expect(asset.statusCode).toBe(200);
    expect(asset.headers["cache-control"]).toContain("immutable");
    expect(api.statusCode).toBe(404);
    expect(api.headers["content-type"]).toContain("application/json");
    expect(unknown.statusCode).toBe(404);
  });
});
