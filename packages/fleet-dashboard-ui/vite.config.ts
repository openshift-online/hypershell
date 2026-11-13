import type { IncomingMessage, ServerResponse } from "node:http";
import { defineConfig, type Plugin } from "vite";

// --- Dev-only E2E mock -------------------------------------------------------
// `pnpm dev:mock` (FD_DEV_MOCK=1) serves deterministic /api/e2e/* fixtures from
// the Vite dev server so the glass E2E run view can be eyeballed in a browser
// without a running BFF or a live Loki. It is never part of a production build
// (configureServer runs only under `vite serve`) and is inert unless the flag is
// set. The fixtures deliberately exercise the recent UI work: nested run
// attempts, failed-run table duration fallback, the parsed failure header, and
// the failure-diagnostics section.
const mockEnabled = process.env.FD_DEV_MOCK === "1";

interface Envelope<T> {
  readonly data: T;
  readonly generatedAt: string;
  readonly stale: boolean;
  readonly error: string | null;
}

function envelope<T>(data: T): Envelope<T> {
  return {
    data,
    generatedAt: new Date().toISOString(),
    stale: false,
    error: null,
  };
}

const TENANT = "tenant-a";
const CLUSTER = "cluster-a";
const NAMESPACE = "ns-a";

const bundleA = {
  version: "v20261104",
  digest: "sha256:aaaa1111",
  date: "2026-11-04T00:00:00Z",
};
const bundleB = {
  version: "v20261103",
  digest: "sha256:bbbb2222",
  date: "2026-11-03T00:00:00Z",
};

// Pipe-collapsed testify assertion, exactly as it reaches the glass view. The
// header should surface only the trailing "primary gateway did not reach Running";
// the full diff stays in the failed-step code block below it.
const FAILURE_MESSAGE =
  'Not equal: | expected: "Running" | actual : "Provisioning" | ' +
  "Diff: | --- Expected | +++ Actual | @@ -1 +1 @@ | -Running | +Provisioning | " +
  "primary gateway did not reach Running";

interface MockRun {
  readonly runId: string;
  readonly bundle: typeof bundleA;
  readonly revision: string;
  readonly startedAgoMs: number;
  readonly ranForMs: number | null;
  readonly durationMs: number | null;
  readonly verdict: "passed" | "failed" | "unknown";
  readonly running?: boolean;
}

// Group A: three attempts of rev-alpha (latest passed, two older failed) - the
// collapsing/attempt-counter case. Group B: rev-beta with one passed and one
// still-running attempt. The failed runs carry durationMs: 0 but real start/end
// timestamps, so the table has to fall back to end-start to show a duration.
const RUNS: readonly MockRun[] = [
  {
    runId: "deab0fa5",
    bundle: bundleA,
    revision: "rev-alpha",
    startedAgoMs: 32 * 60_000,
    ranForMs: 92_000,
    durationMs: 92_000,
    verdict: "passed",
  },
  {
    runId: "14052c8f",
    bundle: bundleA,
    revision: "rev-alpha",
    startedAgoMs: 44 * 60_000,
    ranForMs: 64_000,
    durationMs: 0,
    verdict: "failed",
  },
  {
    runId: "46df1ad4",
    bundle: bundleA,
    revision: "rev-alpha",
    startedAgoMs: 59 * 60_000,
    ranForMs: 71_000,
    durationMs: 0,
    verdict: "failed",
  },
  {
    runId: "99aa11bb",
    bundle: bundleB,
    revision: "rev-beta",
    startedAgoMs: 120 * 60_000,
    ranForMs: 80_000,
    durationMs: 80_000,
    verdict: "passed",
  },
  {
    runId: "7c3e5f20",
    bundle: bundleB,
    revision: "rev-beta",
    startedAgoMs: 9 * 60_000,
    ranForMs: null,
    durationMs: null,
    verdict: "unknown",
    running: true,
  },
];

const iso = (nowMs: number, agoMs: number): string =>
  new Date(nowMs - agoMs).toISOString();

function summary(run: MockRun, nowMs: number): Record<string, unknown> {
  const startedAt = iso(nowMs, run.startedAgoMs);
  const endedAt =
    run.ranForMs === null
      ? undefined
      : iso(nowMs, run.startedAgoMs - run.ranForMs);
  return {
    bundle: run.bundle,
    id: `${TENANT}/${NAMESPACE}/${run.runId}`,
    runId: run.runId,
    tenant: TENANT,
    cluster: CLUSTER,
    bodyCluster: null,
    namespace: NAMESPACE,
    revision: run.revision,
    sourceRevision: run.revision,
    startedAt,
    endedAt,
    durationMs: run.durationMs ?? undefined,
    verdict: run.verdict,
    reportingComplete: run.running !== true,
    hasStructuredSteps: true,
    stepCount: 4,
  };
}

function logLine(
  seq: number,
  nowMs: number,
  agoMs: number,
  stepId: string,
  message: string,
): Record<string, unknown> {
  return {
    sequence: seq,
    timestamp: iso(nowMs, agoMs),
    stepId,
    message,
    truncated: false,
  };
}

function logsFor(run: MockRun, nowMs: number): Record<string, unknown>[] {
  const base = run.startedAgoMs;
  const lines: Record<string, unknown>[] = [
    logLine(
      1,
      nowMs,
      base,
      "setup",
      "suite setup: provisioning namespace ns-a",
    ),
    logLine(
      2,
      nowMs,
      base - 5_000,
      "P1.1",
      "waiting for primary gateway to reach Running",
    ),
  ];
  if (run.verdict === "failed") {
    lines.push(
      logLine(3, nowMs, base - 50_000, "P1.1", FAILURE_MESSAGE),
      logLine(
        4,
        nowMs,
        base - 55_000,
        "cleanup",
        "suite teardown: deleting namespace ns-a",
      ),
    );
  } else {
    lines.push(
      logLine(
        3,
        nowMs,
        base - 40_000,
        "P1.1",
        "primary gateway did not reach Running",
      ),
      logLine(4, nowMs, base - 42_000, "P1.2", "routes verified"),
      logLine(
        5,
        nowMs,
        base - 45_000,
        "cleanup",
        "suite teardown: deleting namespace ns-a",
      ),
    );
  }
  return lines;
}

function stepsFor(run: MockRun, nowMs: number): Record<string, unknown>[] {
  const base = run.startedAgoMs;
  if (run.verdict === "failed") {
    return [
      {
        stepId: "setup",
        name: "Suite setup",
        state: "passed",
        startedAt: iso(nowMs, base),
        endedAt: iso(nowMs, base - 5_000),
        durationMs: 5_000,
      },
      {
        stepId: "P1.1",
        name: "Provision primary gateway",
        state: "failed",
        startedAt: iso(nowMs, base - 5_000),
        endedAt: iso(nowMs, base - 57_000),
        durationMs: 52_000,
      },
      { stepId: "P1.2", name: "Verify routes", state: "not-run" },
      {
        stepId: "diagnostics",
        name: "failure diagnostics",
        state: "passed",
        startedAt: iso(nowMs, base - 57_000),
        endedAt: iso(nowMs, base - 60_000),
        durationMs: 3_000,
      },
      {
        stepId: "cleanup",
        name: "Suite teardown",
        state: "passed",
        durationMs: 4_000,
      },
    ];
  }
  if (run.running === true) {
    return [
      {
        stepId: "setup",
        name: "Suite setup",
        state: "passed",
        startedAt: iso(nowMs, base),
        endedAt: iso(nowMs, base - 5_000),
        durationMs: 5_000,
      },
      {
        stepId: "P1.1",
        name: "Provision primary gateway",
        state: "running",
        startedAt: iso(nowMs, base - 5_000),
      },
      { stepId: "P1.2", name: "Verify routes", state: "not-run" },
      {
        stepId: "diagnostics",
        name: "failure diagnostics",
        state: "not-run",
      },
      { stepId: "cleanup", name: "Suite teardown", state: "not-run" },
    ];
  }
  return [
    {
      stepId: "setup",
      name: "Suite setup",
      state: "passed",
      startedAt: iso(nowMs, base),
      endedAt: iso(nowMs, base - 5_000),
      durationMs: 5_000,
    },
    {
      stepId: "P1.1",
      name: "Provision primary gateway",
      state: "passed",
      startedAt: iso(nowMs, base - 5_000),
      endedAt: iso(nowMs, base - 50_000),
      durationMs: 45_000,
    },
    {
      stepId: "P1.2",
      name: "Verify routes",
      state: "passed",
      startedAt: iso(nowMs, base - 50_000),
      endedAt: iso(nowMs, base - 58_000),
      durationMs: 8_000,
    },
    {
      stepId: "diagnostics",
      name: "failure diagnostics",
      state: "skipped",
    },
    {
      stepId: "cleanup",
      name: "Suite teardown",
      state: "passed",
      durationMs: 4_000,
    },
  ];
}

function diagnosticsFor(run: MockRun): Record<string, unknown>[] {
  if (run.verdict !== "failed") {
    return [];
  }
  return [
    {
      title: "Pod status (namespace ns-a)",
      body:
        "NAME                   READY   STATUS             RESTARTS\n" +
        "gateway-ns-a-7c9d8     0/1     CrashLoopBackOff   5\n" +
        "gateway-db-ns-a-0      1/1     Running            0",
      severity: "error",
    },
    {
      title: "Recent events",
      body:
        "LAST SEEN   TYPE      REASON    OBJECT                 MESSAGE\n" +
        "30s         Warning   BackOff   pod/gateway-ns-a-7c9d8  Back-off restarting failed container",
      severity: "warn",
    },
    {
      title: "Gateway resource",
      body:
        "status.phase: Provisioning\n" +
        "conditions:\n- type: Ready  status: False  reason: GatewayProvisioning",
      severity: "info",
    },
  ];
}

function detailFor(run: MockRun, nowMs: number): Record<string, unknown> {
  const failed = run.verdict === "failed";
  return {
    ...summary(run, nowMs),
    failure: failed ? { stepId: "P1.1", message: FAILURE_MESSAGE } : null,
    plannedSteps: [
      { stepId: "setup", name: "Suite setup" },
      { stepId: "P1.1", name: "Provision primary gateway" },
      { stepId: "P1.2", name: "Verify routes" },
      { stepId: "diagnostics", name: "failure diagnostics" },
      { stepId: "cleanup", name: "Suite teardown" },
    ],
    steps: stepsFor(run, nowMs),
    logs: logsFor(run, nowMs),
    logsTruncated: false,
  };
}

function sendJson(res: ServerResponse, body: unknown): void {
  res.setHeader("Content-Type", "application/json");
  res.end(JSON.stringify(body));
}

function e2eMockPlugin(): Plugin {
  return {
    name: "fd-e2e-mock",
    configureServer(server) {
      server.middlewares.use(
        (req: IncomingMessage, res: ServerResponse, next: () => void) => {
          const rawUrl = req.url ?? "";
          if (!rawUrl.startsWith("/api/")) {
            next();
            return;
          }
          const url = new URL(rawUrl, "http://localhost");
          const path = url.pathname;
          const nowMs = Date.now();

          if (path === "/api/e2e/runs") {
            sendJson(
              res,
              envelope({
                enabled: true,
                runs: RUNS.map((r) => summary(r, nowMs)),
                truncated: false,
                windowSeconds: 168 * 3600,
                limit: 200,
              }),
            );
            return;
          }

          if (path.startsWith("/api/e2e/runs/")) {
            const runId = decodeURIComponent(path.split("/").pop() ?? "");
            const run = RUNS.find((r) => r.runId === runId);
            if (run) {
              sendJson(res, envelope(detailFor(run, nowMs)));
              return;
            }
          }

          if (path === "/api/e2e/diagnostics") {
            const id = url.searchParams.get("run") ?? "";
            const runId = id.split("/").pop() ?? "";
            const run = RUNS.find((r) => r.runId === runId);
            sendJson(
              res,
              envelope({
                diagnostics: run ? diagnosticsFor(run) : [],
              }),
            );
            return;
          }

          if (path === "/api/e2e/logs") {
            const id = url.searchParams.get("run") ?? "";
            const runId = id.split("/").pop() ?? "";
            const run = RUNS.find((r) => r.runId === runId);
            sendJson(
              res,
              envelope({
                entries: run ? logsFor(run, nowMs) : [],
                nextCursor: null,
                hasMore: false,
                boundaryTruncated: false,
              }),
            );
            return;
          }

          if (path === "/api/e2e/stream") {
            res.setHeader("Content-Type", "text/event-stream");
            res.setHeader("Cache-Control", "no-cache");
            res.setHeader("Connection", "keep-alive");
            res.write(
              `data: ${JSON.stringify({ kind: "status", status: { state: "live", droppedEntries: 0 } })}\n\n`,
            );
            const timer = setInterval(() => {
              res.write(`data: ${JSON.stringify({ kind: "heartbeat" })}\n\n`);
            }, 15_000);
            req.on("close", () => {
              clearInterval(timer);
            });
            return;
          }

          // Other planes (fleet/promotion/topology/...) are not needed to
          // validate the E2E view; return an inert stale envelope so those tabs
          // degrade cleanly instead of hitting a dead proxy.
          sendJson(res, {
            data: null,
            generatedAt: new Date().toISOString(),
            stale: true,
            error: "dev mock: plane not implemented",
          });
        },
      );
    },
  };
}

// No @vitejs/plugin-react: Vite's built-in esbuild transforms .tsx using the
// tsconfig `jsx: react-jsx` setting, which is sufficient for a production build.
// (Fast Refresh / the React Compiler babel pass are dev niceties we forgo to
// avoid adding a dependency; the React Compiler lint rules still run.)
export default defineConfig({
  plugins: mockEnabled ? [e2eMockPlugin()] : [],
  build: {
    outDir: "dist",
    // The Go BFF serves this bundle from an embedded FS with an SPA fallback.
    sourcemap: false,
  },
  server: {
    // `pnpm dev` proxies API calls to a locally running BFF (see the BFF's
    // default dev port). No fleet identity is encoded here - only a loopback.
    // `pnpm dev:mock` serves fixtures instead (see e2eMockPlugin) and skips the
    // proxy so unimplemented planes do not hit a dead port.
    proxy: mockEnabled ? undefined : { "/api": "http://localhost:8081" },
  },
});
