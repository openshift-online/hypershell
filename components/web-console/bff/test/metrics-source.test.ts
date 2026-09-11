import { execFileSync } from "node:child_process";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import {
  createServer as httpServer,
  type Server,
  type RequestListener,
} from "node:http";
import { createServer as httpsServer } from "node:https";
import { tmpdir } from "node:os";
import path from "node:path";
import { gzipSync } from "node:zlib";

import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";

import { buildApp } from "../src/app.js";
import { browserRuntimeConfig, loadConfig } from "../src/config.js";
import { fetchMetrics } from "../src/metrics-source.js";
import { queryClusterCpu } from "../src/metrics-cluster-cpu.js";
import { queryClusterMemory } from "../src/metrics-cluster-memory.js";
import { queryClusterNodes } from "../src/metrics-cluster-nodes.js";
import { queryClusterPods } from "../src/metrics-cluster-pods.js";
import { queryGatewayPhaseCounts } from "../src/metrics-gateways.js";
import { queryGatewayProvisionDuration } from "../src/metrics-gateway-provision-duration.js";

let directory: string;
let caFile: string;
let tokenFile: string;
let key: string;
let cert: string;
const servers: Server[] = [];

beforeAll(async () => {
  directory = await mkdtemp(path.join(tmpdir(), "hypershell-metrics-test-"));
  caFile = path.join(directory, "ca.pem");
  tokenFile = path.join(directory, "token");
  const keyFile = path.join(directory, "key.pem");
  execFileSync(
    "openssl",
    [
      "req",
      "-x509",
      "-newkey",
      "rsa:2048",
      "-nodes",
      "-days",
      "1",
      "-keyout",
      keyFile,
      "-out",
      caFile,
      "-subj",
      "/CN=localhost",
      "-addext",
      "subjectAltName=IP:127.0.0.1",
    ],
    { stdio: "ignore" },
  );
  key = await readFile(keyFile, "utf8");
  cert = await readFile(caFile, "utf8");
  await writeFile(
    path.join(directory, "index.html"),
    "<!doctype html><html><head></head><body></body></html>",
  );
});

afterEach(async () => {
  for (const server of servers.splice(0)) {
    server.closeAllConnections();
    await new Promise<void>((resolve) =>
      server.close(() => {
        resolve();
      }),
    );
  }
});
afterAll(async () => {
  await rm(directory, { recursive: true, force: true });
});

async function serve(handler: RequestListener, tls = true): Promise<string> {
  const server = tls
    ? httpsServer({ key, cert }, handler)
    : httpServer(handler);
  servers.push(server);
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  if (!address || typeof address === "string")
    throw new Error("Expected TCP address");
  return `${tls ? "https" : "http"}://127.0.0.1:${String(address.port)}`;
}

const sample = (value: string) =>
  JSON.stringify({
    status: "success",
    data: { result: [{ metric: {}, value: [0, value] }] },
  });

describe("metrics sources", () => {
  for (const tls of [false, true]) {
    const protocol = tls ? "HTTPS" : "HTTP";
    it(`accepts a response at the 4 MiB limit over ${protocol}`, async () => {
      const body = sample("4").padEnd(4 * 1024 * 1024, " ");
      const url = await serve((_req, res) => {
        res.end(body);
      }, tls);
      const source = tls ? { url, caFile } : url;
      const response = await fetchMetrics(
        source,
        "up",
        AbortSignal.timeout(5000),
      );
      expect(await response.json()).toEqual(JSON.parse(sample("4")));
    });

    it(`rejects oversized chunked responses and closes the ${protocol} stream`, async () => {
      let closed = false;
      const url = await serve((_req, res) => {
        res.on("close", () => {
          closed = true;
        });
        res.write(" ".repeat(4 * 1024 * 1024));
        res.write("x");
        // Leave the stream open: rejection must not wait for the response to end.
      }, tls);
      const source = tls ? { url, caFile } : url;
      await expect(
        fetchMetrics(source, "up", AbortSignal.timeout(5000)),
      ).rejects.toThrow("Metrics response too large");
      await expect.poll(() => closed).toBe(true);
    });
  }

  it("limits the decoded application response, including compressed bodies", async () => {
    const body = gzipSync(" ".repeat(4 * 1024 * 1024 + 1));
    const url = await serve((_req, res) => {
      res.writeHead(200, {
        "Content-Encoding": "gzip",
        "Content-Length": body.byteLength,
      });
      res.end(body);
    }, false);
    await expect(
      fetchMetrics(url, "up", AbortSignal.timeout(5000)),
    ).rejects.toThrow("Metrics response too large");
  });

  it("validates TLS and reads a rotated token on each request", async () => {
    const tokens: (string | undefined)[] = [];
    const url = await serve((req, res) => {
      tokens.push(req.headers.authorization);
      res.end(sample("4"));
    });
    await writeFile(tokenFile, "first-token\n");
    await fetchMetrics(
      { url, caFile, tokenFile },
      "up",
      AbortSignal.timeout(1000),
    );
    await writeFile(tokenFile, "second-token\n");
    await fetchMetrics(
      { url, caFile, tokenFile },
      "up",
      AbortSignal.timeout(1000),
    );
    expect(tokens).toEqual(["Bearer first-token", "Bearer second-token"]);
  });

  it("rejects an untrusted certificate before sending the token", async () => {
    let requests = 0;
    const url = await serve((_req, res) => {
      requests++;
      res.end(sample("4"));
    });
    await writeFile(tokenFile, "private-token");
    await expect(
      fetchMetrics({ url, tokenFile }, "up", AbortSignal.timeout(1000)),
    ).rejects.toThrow("Metrics HTTPS request failed");
    expect(requests).toBe(0);
  });

  it("does not send tokens to a redirect target", async () => {
    let redirected = 0;
    const target = await serve((_req, res) => {
      redirected++;
      res.end(sample("4"));
    });
    const url = await serve((_req, res) => {
      res.writeHead(302, { Location: target });
      res.end();
    });
    await writeFile(tokenFile, "private-token");
    await expect(
      fetchMetrics({ url, tokenFile, caFile }, "up", AbortSignal.timeout(1000)),
    ).rejects.toThrow("HTTP 302");
    expect(redirected).toBe(0);
  });

  it("fails closed for empty or unreadable credentials", async () => {
    const url = await serve((_req, res) => {
      res.end(sample("4"));
    });
    await writeFile(tokenFile, "");
    for (const file of [tokenFile, path.join(directory, "missing")]) {
      await expect(
        fetchMetrics(
          { url, tokenFile: file, caFile },
          "up",
          AbortSignal.timeout(1000),
        ),
      ).rejects.toThrow("Could not read metrics credentials");
    }
  });

  it("identifies credential failures without revealing file paths or contents", async () => {
    const url = "https://127.0.0.1";
    const invalid = path.join(directory, "private-mount-name");
    await writeFile(invalid, "private-token\ninvalid-header");
    const empty = path.join(directory, "empty");
    await writeFile(empty, " \n");
    const missing = path.join(directory, "missing-private-mount");
    const cases = [
      {
        source: { url, tokenFile: missing },
        variable: "CLUSTER_PROMETHEUS_TOKEN_FILE",
        reason: "is missing",
      },
      {
        source: { url, tokenFile: directory },
        variable: "CLUSTER_PROMETHEUS_TOKEN_FILE",
        reason: "is unreadable",
      },
      {
        source: { url, tokenFile: empty },
        variable: "CLUSTER_PROMETHEUS_TOKEN_FILE",
        reason: "is empty",
      },
      {
        source: { url, tokenFile: invalid },
        variable: "CLUSTER_PROMETHEUS_TOKEN_FILE",
        reason: "has an invalid token format",
      },
      {
        source: { url, caFile: missing },
        variable: "CLUSTER_PROMETHEUS_CA_FILE",
        reason: "is missing",
      },
      {
        source: { url, caFile: directory },
        variable: "CLUSTER_PROMETHEUS_CA_FILE",
        reason: "is unreadable",
      },
      {
        source: { url, caFile: empty },
        variable: "CLUSTER_PROMETHEUS_CA_FILE",
        reason: "is empty",
      },
    ];
    for (const { source, variable, reason } of cases) {
      await expect(
        fetchMetrics(source, "up", AbortSignal.timeout(1000)),
      ).rejects.toThrow(
        `Could not read metrics credentials: ${variable} ${reason}`,
      );
    }
    const app = await buildApp(
      loadConfig({
        NODE_ENV: "test",
        LOG_LEVEL: "silent",
        STATIC_ROOT: directory,
        CLUSTER_PROMETHEUS_URL: url,
        CLUSTER_PROMETHEUS_TOKEN_FILE: invalid,
      }),
    );
    try {
      const response = await app.inject({
        method: "GET",
        url: "/api/metrics/cluster-cpu",
      });
      expect(response.statusCode).toBe(502);
      expect(response.json()).toEqual({
        error: "Metrics unavailable",
        statusCode: 502,
      });
    } finally {
      await app.close();
    }
  });

  it("bounds slow HTTPS requests and reports authentication errors", async () => {
    await writeFile(tokenFile, "private-token");
    const slow = await serve(() => {
      // Keep the request open until the client timeout expires.
    });
    await expect(
      fetchMetrics(
        { url: slow, tokenFile, caFile },
        "up",
        AbortSignal.timeout(30),
      ),
    ).rejects.toThrow("timed out");
    const forbidden = await serve((_req, res) => {
      res.writeHead(403);
      res.end("private-token");
    });
    await expect(
      fetchMetrics(
        { url: forbidden, tokenFile, caFile },
        "up",
        AbortSignal.timeout(1000),
      ),
    ).rejects.toThrow(/^Metrics endpoint returned HTTP 403$/u);
  });

  it("uses protected cluster metrics and separate instance application metrics", async () => {
    const clusterQueries: string[] = [];
    const applicationQueries: string[] = [];
    await writeFile(tokenFile, "cluster-token");
    const clusterUrl = await serve((req, res) => {
      expect(req.headers.authorization).toBe("Bearer cluster-token");
      const query =
        new URL(req.url ?? "", "http://unused").searchParams.get("query") ?? "";
      clusterQueries.push(query);
      res.end(sample(query.includes('mode="idle"') ? "16" : "4"));
    });
    const applicationUrl = await serve((req, res) => {
      expect(req.headers.authorization).toBeUndefined();
      applicationQueries.push(
        new URL(req.url ?? "", "http://unused").searchParams.get("query") ?? "",
      );
      res.end(
        JSON.stringify({
          status: "success",
          data: { result: [{ metric: { phase: "Running" }, value: [0, "3"] }] },
        }),
      );
    }, false);
    const config = loadConfig({
      NODE_ENV: "test",
      LOG_LEVEL: "silent",
      STATIC_ROOT: directory,
      PROMETHEUS_URL: applicationUrl,
      PROMETHEUS_NAMESPACE: "hyp1",
      CLUSTER_PROMETHEUS_URL: clusterUrl,
      CLUSTER_PROMETHEUS_TOKEN_FILE: tokenFile,
      CLUSTER_PROMETHEUS_CA_FILE: caFile,
    });
    const app = await buildApp(config);
    try {
      const cpu = await app.inject({
        method: "GET",
        url: "/api/metrics/cluster-cpu",
      });
      expect(cpu.statusCode).toBe(200);
      expect(cpu.json()).toMatchObject({ capacity_cores: 16, used_cores: 4 });
      const gateway = await app.inject({
        method: "GET",
        url: "/api/metrics/gateways?namespace=hyp0",
      });
      expect(gateway.statusCode).toBe(200);
      expect(gateway.json()).toMatchObject({ counts: { Running: 3 } });
      expect(clusterQueries).toHaveLength(2);
      expect(applicationQueries).toEqual([
        'max by (phase) (hypershell_gateways_total{namespace="hyp1"})',
      ]);
      expect(JSON.stringify(browserRuntimeConfig(config))).not.toContain(
        "cluster-token",
      );
      expect(JSON.stringify(browserRuntimeConfig(config))).not.toContain(
        clusterUrl,
      );
    } finally {
      await app.close();
    }
  });

  it("keeps repeated gauge samples from adding or overwriting counts", async () => {
    const url = await serve((_req, res) => {
      res.end(
        JSON.stringify({
          status: "success",
          data: {
            result: [3, 3, 2].map((value) => ({
              metric: { phase: "Running" },
              value: [0, String(value)],
            })),
          },
        }),
      );
    }, false);
    expect((await queryGatewayPhaseCounts(url, 1000, "hyp1")).Running).toBe(3);
  });

  it("scopes all duration queries to the controller namespace", async () => {
    const queries: string[] = [];
    const url = await serve((req, res) => {
      queries.push(
        new URL(req.url ?? "", "http://unused").searchParams.get("query") ?? "",
      );
      res.end(sample("3"));
    }, false);
    await queryGatewayProvisionDuration(url, 1000, "hyp1");
    expect(queries).toHaveLength(4);
    for (const query of queries)
      expect(query).toContain('{k8s_namespace_name="hyp1"}');
    expect(queries[0]).toBe(
      'sum(gateway_provision_duration_seconds_count{k8s_namespace_name="hyp1"})',
    );
  });

  it("does not report zero CPU use or gateway counts when samples are missing", async () => {
    const url = await serve((req, res) => {
      const query =
        new URL(req.url ?? "", "http://unused").searchParams.get("query") ?? "";
      res.end(
        query.includes('mode="idle"')
          ? sample("4")
          : JSON.stringify({ status: "success", data: { result: [] } }),
      );
    }, false);
    await expect(queryClusterCpu(url, 1000)).rejects.toThrow("no samples");
    await expect(queryGatewayPhaseCounts(url, 1000, "hyp1")).rejects.toThrow(
      "No gateway metrics",
    );
  });

  it("accepts zero CPU use, available memory, ready nodes, and pod phase counts", async () => {
    const values = new Map([
      ['sum(count by (instance) (node_cpu_seconds_total{mode="idle"}))', "4"],
      ["sum(node_memory_MemTotal_bytes)", "100"],
      ["count(kube_node_info)", "4"],
      ['sum(kube_node_status_allocatable{resource="pods"})', "100"],
      ["count(kube_pod_info)", "1"],
      ['sum(kube_pod_status_phase{phase="Running"})', "1"],
    ]);
    const url = await serve((req, res) => {
      const query =
        new URL(req.url ?? "", "http://unused").searchParams.get("query") ?? "";
      res.end(sample(values.get(query) ?? "0"));
    }, false);
    expect(await queryClusterCpu(url, 1000)).toEqual({
      capacity_cores: 4,
      used_cores: 0,
      available_cores: 4,
    });
    expect(await queryClusterMemory(url, 1000)).toEqual({
      capacity_bytes: 100,
      used_bytes: 100,
      available_bytes: 0,
    });
    expect(await queryClusterNodes(url, 1000)).toEqual({
      total_nodes: 4,
      ready_nodes: 0,
      not_ready_nodes: 4,
    });
    expect(await queryClusterPods(url, 1000)).toEqual({
      capacity_pods: 100,
      used_pods: 1,
      available_pods: 99,
      phase_running_pods: 1,
      phase_pending_pods: 0,
      phase_failed_pods: 0,
      phase_succeeded_pods: 0,
      phase_unknown_pods: 0,
    });
  });

  it("rejects unsafe credential origins and invalid namespace selectors", () => {
    expect(() =>
      loadConfig({
        CLUSTER_PROMETHEUS_URL: "http://example.com",
        CLUSTER_PROMETHEUS_TOKEN_FILE: "/token",
      }),
    ).toThrow("HTTPS");
    expect(() =>
      loadConfig({ CLUSTER_PROMETHEUS_TOKEN_FILE: "/token" }),
    ).toThrow("HTTPS");
    expect(() => loadConfig({ PROMETHEUS_NAMESPACE: 'hyp1"} or up' })).toThrow(
      "PROMETHEUS_NAMESPACE",
    );
  });
});
