import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IntlProvider } from "react-intl";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  MAX_LOG_BUFFER,
  type E2eLogPage,
  type E2eDiagnosticsData,
  type E2eRunDetail,
  type E2eRunsData,
  type E2eRunSummary,
} from "../../../domain/e2e-run";
import type { E2eStreamHandlers, FleetApi } from "../../../application/ports";
import type { Plane } from "../../../domain/plane";
import { FleetApiProvider } from "../../query/api-context";
import { createMemoryHistory } from "../../routing/history-port";
import { RouterProvider } from "../../routing/router";
import { GateExecution } from "./gate-execution";
import { E2eRunView } from "./e2e-run-view";

// Neutral placeholders only (hub-a/tenant-1/run-1); no fleet identifiers (firewall).
const NOW = Date.parse("2026-01-01T01:00:00Z");

function plane<T>(data: T): Plane<T> {
  return {
    data,
    generatedAt: "2026-01-01T00:00:00Z",
    stale: false,
    error: null,
  };
}

function summary(over: Partial<E2eRunSummary> = {}): E2eRunSummary {
  return {
    id: "tenant-1/ns/run-1",
    runId: "run-1",
    tenant: "tenant-1",
    cluster: "tenant-1",
    bodyCluster: "hub-a",
    namespace: "ns",
    revision: "abc1234",
    sourceRevision: "def5678",
    startedAt: "2026-01-01T00:30:00Z",
    endedAt: "2026-01-01T00:35:00Z",
    durationMs: 300000,
    verdict: "passed",
    reportingComplete: true,
    hasStructuredSteps: true,
    stepCount: 2,
    ...over,
  };
}

function detail(over: Partial<E2eRunDetail> = {}): E2eRunDetail {
  return {
    ...summary(),
    plannedSteps: [
      { stepId: "a", name: "Build" },
      { stepId: "b", name: "Test" },
    ],
    steps: [
      {
        stepId: "a",
        name: "Build",
        state: "passed",
        startedAt: "2026-01-01T00:30:00Z",
        endedAt: "2026-01-01T00:31:00Z",
        durationMs: 60000,
      },
      {
        stepId: "b",
        name: "Test",
        state: "skipped",
        startedAt: null,
        endedAt: null,
        durationMs: null,
      },
    ],
    failure: null,
    logs: [
      {
        sequence: 1,
        timestamp: "t1",
        stepId: "a",
        message: "build log",
        truncated: false,
      },
    ],
    logsTruncated: false,
    ...over,
  };
}

interface Harness {
  api: FleetApi;
  handlers: { current: E2eStreamHandlers | null };
}

function makeApi(
  runDetail: E2eRunDetail,
  overrides: Partial<FleetApi> = {},
): Harness {
  const handlers: { current: E2eStreamHandlers | null } = { current: null };
  const api: FleetApi = {
    getFleet: vi.fn(),
    getPromotion: vi.fn(),
    getTopology: vi.fn(),
    getInstances: vi.fn(),
    getSdlc: vi.fn(),
    getSdlcAgents: vi.fn(),
    getE2eRuns: vi.fn(),
    getDelivery: vi.fn(),
    getE2eRun: vi.fn().mockResolvedValue(plane(runDetail)),
    getE2eDiagnostics: vi
      .fn()
      .mockResolvedValue(plane({ diagnostics: runDetail.diagnostics ?? [] })),
    getE2eRunLogs: vi
      .fn()
      .mockResolvedValue(
        plane({ entries: [], nextCursor: null, hasMore: false }),
      ),
    streamE2eRun: vi.fn((_id, h: E2eStreamHandlers) => {
      handlers.current = h;
      return () => {
        handlers.current = null;
      };
    }),
    ...overrides,
  };
  return { api, handlers };
}

function renderView(
  api: FleetApi,
  data: E2eRunsData,
  translations?: Record<string, string>,
): void {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <IntlProvider locale="en" defaultLocale="en" messages={translations}>
      <QueryClientProvider client={queryClient}>
        <FleetApiProvider value={api}>
          <RouterProvider
            history={createMemoryHistory({ path: "/e2e", search: "" })}
          >
            <E2eRunView data={data} nowMs={NOW} />
          </RouterProvider>
        </FleetApiProvider>
      </QueryClientProvider>
    </IntlProvider>,
  );
}

/** Render a gate execution drawer with a gate selected so route.map.exec applies. */
function renderGate(api: FleetApi, ui: React.ReactElement): void {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <IntlProvider locale="en">
      <QueryClientProvider client={queryClient}>
        <FleetApiProvider value={api}>
          <RouterProvider
            history={createMemoryHistory({ path: "/", search: "?gate=g1" })}
          >
            {ui}
          </RouterProvider>
        </FleetApiProvider>
      </QueryClientProvider>
    </IntlProvider>,
  );
}

const enabled = (runs: readonly E2eRunSummary[]): E2eRunsData => ({
  enabled: true,
  runs,
  truncated: false,
  windowSeconds: 604800,
  limit: 20,
});

describe("E2eRunView", () => {
  it("shows the feature-off state when disabled", () => {
    const { api } = makeApi(detail());
    renderView(api, {
      enabled: false,
      runs: [],
      truncated: false,
      windowSeconds: 0,
      limit: 0,
    });
    expect(
      screen.getByText("End-to-end run reporting is not enabled."),
    ).toBeDefined();
  });

  // Verifies: E2E-LOG-12.01
  it("lists recent runs newest-first with revision and verdict", () => {
    const { api } = makeApi(detail());
    const older = summary({
      id: "tenant-1/ns/run-0",
      runId: "run-0",
      revision: "old111",
      startedAt: "2026-01-01T00:00:00Z",
    });
    const newer = summary({
      id: "tenant-1/ns/run-1",
      runId: "run-1",
      revision: "new222",
      startedAt: "2026-01-01T00:40:00Z",
    });
    renderView(api, enabled([older, newer]));

    // The run selection links render in DOM order; newest-first puts new222 first.
    const buttons = screen.getAllByRole("link", { name: /Open run/ });
    const firstLabel = buttons[0]?.getAttribute("aria-label") ?? "";
    const secondLabel = buttons[1]?.getAttribute("aria-label") ?? "";
    expect(firstLabel).toContain("new222");
    expect(secondLabel).toContain("old111");
  });

  it.each([
    "Revision",
    "Attempt",
    "Started",
    "Duration",
    "Verdict",
    "Bundle",
    "Target",
  ])("sorts the history %s column both ways", async (column) => {
    const user = userEvent.setup();
    const { api } = makeApi(detail());
    renderView(
      api,
      enabled([
        summary({
          id: "one",
          cluster: "aaa",
          bundle: {
            version: "aaa",
            digest: "sha256:aaa",
            tag: null,
            date: null,
            prs: [],
          },
          runId: "aaa",
          revision: "aaa",
          startedAt: "2026-01-01T00:00:00Z",
          durationMs: 2000,
          verdict: "failed",
        }),
        summary({
          id: "two",
          cluster: "zzz",
          bundle: {
            version: "zzz",
            digest: "sha256:zzz",
            tag: null,
            date: null,
            prs: [],
          },
          runId: "zzz",
          revision: "zzz",
          startedAt: "2026-01-01T00:01:00Z",
          durationMs: 10000,
          verdict: "passed",
        }),
      ]),
    );
    await user.click(screen.getByRole("button", { name: column }));
    const first = screen
      .getAllByRole("link", { name: /Open run/ })[0]
      ?.getAttribute("aria-label");
    expect(first).toContain("aaa");
    await user.click(screen.getByRole("button", { name: column }));
    expect(
      screen
        .getAllByRole("link", { name: /Open run/ })[0]
        ?.getAttribute("aria-label"),
    ).toContain("zzz");
  });

  it("closes the drawer without clearing the history filter or selection and restores focus", async () => {
    const user = userEvent.setup();
    const { api, handlers } = makeApi(detail());
    renderView(api, enabled([summary()]));
    await user.type(
      screen.getByRole("textbox", {
        name: "Search instances, revisions, or attempts",
      }),
      "abc",
    );
    const trigger = screen.getByRole("link", { name: /Open run/ });
    await user.click(trigger);
    await screen.findByRole("button", { name: "Build" });
    await user.click(screen.getByRole("button", { name: "Close details" }));
    expect(screen.queryByRole("button", { name: "Build" })).toBeNull();
    expect(handlers.current).toBeNull();
    expect(
      screen.getByRole("textbox", {
        name: "Search instances, revisions, or attempts",
      }),
    ).toHaveProperty("value", "abc");
    expect(document.activeElement).toBe(trigger);
  });

  it.each(["Step", "State", "Duration"])(
    "makes the progress %s column sortable",
    async (column) => {
      const user = userEvent.setup();
      const { api } = makeApi(detail());
      renderView(api, enabled([summary()]));
      await user.click(screen.getByRole("link", { name: /Open run/ }));
      await screen.findByRole("button", { name: "Build" });
      const table = screen.getByRole("grid", { name: "Run steps" });
      await user.click(within(table).getByRole("button", { name: column }));
      expect(
        within(table)
          .getByRole("columnheader", { name: column })
          .getAttribute("aria-sort"),
      ).toBe("ascending");
      await user.click(within(table).getByRole("button", { name: column }));
      expect(
        within(table)
          .getByRole("columnheader", { name: column })
          .getAttribute("aria-sort"),
      ).toBe("descending");
    },
  );

  // Verifies: E2E-LOG-04.03
  it("marks a pre-reporting run as having no structured steps", () => {
    const { api } = makeApi(detail());
    renderView(api, enabled([summary({ hasStructuredSteps: false })]));
    expect(screen.getAllByText("No structured steps").length).toBeGreaterThan(
      0,
    );
  });

  // Verifies: E2E-LOG-12.04
  it("shows ordered steps, exact-SHA correlation, and distinguishes skipped from not-run", async () => {
    const user = userEvent.setup();
    const d = detail({
      steps: [
        {
          stepId: "a",
          name: "Build",
          state: "passed",
          startedAt: null,
          endedAt: null,
          durationMs: 1000,
        },
        {
          stepId: "b",
          name: "Test",
          state: "skipped",
          startedAt: null,
          endedAt: null,
          durationMs: null,
        },
        {
          stepId: "c",
          name: "Deploy",
          state: "not-run",
          startedAt: null,
          endedAt: null,
          durationMs: null,
        },
      ],
    });
    const { api } = makeApi(d);
    renderView(api, enabled([summary()]));

    await user.click(screen.getByRole("link", { name: /Open run abc1234/ }));

    // Deployed revision and application source revision are shown separately.
    expect(await screen.findByText("Source", { exact: false })).toBeDefined();
    // Scope to the steps table via the Build step cell (role names differ by PF
    // table variant, so reach the <table> element directly).
    const stepsTable = (
      await screen.findByRole("button", { name: "Build" })
    ).closest("table");
    if (!stepsTable) throw new Error("expected a steps table");
    const scope = within(stepsTable);
    expect(scope.getByText("Skipped")).toBeDefined();
    expect(scope.getByText("Not run")).toBeDefined();
    // Neither skipped nor not-run renders as Passed; only the genuine passed step.
    expect(scope.getAllByText("Passed")).toHaveLength(1);
  });

  it("applies live stream events (new steps and logs arrive) and escapes log markup", async () => {
    const user = userEvent.setup();
    const d = detail({
      steps: [
        {
          stepId: "a",
          name: "Build",
          state: "running",
          startedAt: "2026-01-01T00:30:00Z",
          endedAt: null,
          durationMs: null,
        },
      ],
      logs: [],
      reportingComplete: false,
      verdict: "unknown",
    });
    const { api, handlers } = makeApi(d);
    renderView(
      api,
      enabled([summary({ reportingComplete: false, verdict: "unknown" })]),
    );

    await user.click(screen.getByRole("link", { name: /Open run abc1234/ }));
    // Wait for the snapshot detail to resolve.
    await screen.findByRole("tab", { name: "Progress" });
    await user.click(screen.getByRole("tab", { name: "Logs" }));
    await waitFor(() => {
      /* eslint-disable-next-line @typescript-eslint/unbound-method -- asserting mock calls */
      expect(api.getE2eRunLogs).toHaveBeenCalledTimes(1);
    });

    act(() => {
      handlers.current?.onStatus({ state: "live", droppedEntries: 0 });
      // A live log line rides the `log` frame (E2E-LOG-21) and appends into the same
      // rendered log list the pane paginates.
      handlers.current?.onLog({
        sequence: 10,
        timestamp: "t",
        stepId: "a",
        message: "<img src=x onerror=1>",
        truncated: false,
      });
      handlers.current?.onEvent({
        event: "step_finished",
        runId: "run-1",
        sequence: 11,
        timestamp: "t",
        stepId: "a",
        name: null,
        outcome: "passed",
        durationMs: 5000,
        message: null,
        truncated: false,
        plannedSteps: [],
      });
    });

    // The live log line rendered as TEXT (escaped): no <img> element was injected.
    const logLine = await screen.findByTestId("e2e-log-line");
    expect(logLine.textContent).toContain("<img src=x onerror=1>");
    expect(logLine.querySelector("img")).toBeNull();
    // The running step transitioned to passed from the live event.
    expect(screen.getByText("Live")).toBeDefined();
  });

  it("keeps the newest live log lines after exceeding the buffer window", async () => {
    const user = userEvent.setup();
    const d = detail({
      steps: [
        {
          stepId: "a",
          name: "Build",
          state: "running",
          startedAt: "2026-01-01T00:30:00Z",
          endedAt: null,
          durationMs: null,
        },
      ],
      logs: [],
      reportingComplete: false,
      verdict: "unknown",
    });
    const { api, handlers } = makeApi(d);
    renderView(
      api,
      enabled([summary({ reportingComplete: false, verdict: "unknown" })]),
    );
    await user.click(screen.getByRole("link", { name: /Open run abc1234/ }));
    await screen.findByRole("tab", { name: "Progress" });
    await user.click(screen.getByRole("tab", { name: "Logs" }));
    await waitFor(() => {
      /* eslint-disable-next-line @typescript-eslint/unbound-method -- asserting mock calls */
      expect(api.getE2eRunLogs).toHaveBeenCalledTimes(1);
    });

    // Stream far more log lines than the buffer window: the buffer must stay bounded
    // (dropping the oldest, E2E-LOG-22) while still showing the newest line.
    const overflow = MAX_LOG_BUFFER + 10;
    act(() => {
      handlers.current?.onStatus({ state: "live", droppedEntries: 0 });
      for (let i = 0; i < overflow; i++) {
        handlers.current?.onLog({
          sequence: 100 + i,
          timestamp: "t",
          stepId: "a",
          message: `live-line-${String(i)}`,
          truncated: false,
        });
      }
    });

    expect(
      await screen.findByText(`live-line-${String(overflow - 1)}`),
    ).toBeDefined();
    // The earliest line slid out of the bounded window and is no longer rendered.
    expect(screen.queryByText("live-line-0")).toBeNull();
  });

  it("surfaces the live-disconnected degraded state distinctly", async () => {
    const user = userEvent.setup();
    // The live tail (and thus a disconnect) only applies to an in-progress run
    // (E2E-LOG-21).
    const { api, handlers } = makeApi(
      detail({ reportingComplete: false, verdict: "unknown" }),
    );
    renderView(
      api,
      enabled([summary({ reportingComplete: false, verdict: "unknown" })]),
    );
    await user.click(screen.getByRole("link", { name: /Open run abc1234/ }));
    await screen.findByRole("tab", { name: "Progress" });

    act(() => {
      handlers.current?.onStatus({ state: "disconnected", droppedEntries: 0 });
    });
    expect(
      await screen.findByText("Live updates disconnected - reconnecting."),
    ).toBeDefined();
  });

  // Verifies: E2E-LOG-21.02
  it("does not open a live tail for a completed run", async () => {
    const user = userEvent.setup();
    const { api } = makeApi(detail()); // reportingComplete: true (finished)
    renderView(api, enabled([summary()]));
    await user.click(screen.getByRole("link", { name: /Open run abc1234/ }));
    // The detail (steps) renders from the snapshot...
    await screen.findByRole("tab", { name: "Progress" });
    // ...but no live tail opens for a finished run; it is read by pagination alone.
    /* eslint-disable-next-line @typescript-eslint/unbound-method -- asserting mock calls */
    expect(api.streamE2eRun).not.toHaveBeenCalled();
  });

  it("shows a failed step's last output from the loaded log buffer", async () => {
    const user = userEvent.setup();
    const d = detail({
      verdict: "failed",
      steps: [
        {
          stepId: "a",
          name: "Build",
          state: "failed",
          startedAt: "2026-01-01T00:30:00Z",
          endedAt: "2026-01-01T00:31:00Z",
          durationMs: 60000,
        },
      ],
    });
    const getE2eRunLogs = vi.fn().mockResolvedValue(
      plane({
        entries: [
          {
            sequence: 1,
            timestamp: "t1",
            stepId: "a",
            message: "boom: build failed",
            truncated: false,
          },
        ],
        nextCursor: null,
        hasMore: false,
      }),
    );
    const { api } = makeApi(d, { getE2eRunLogs });
    renderView(api, enabled([summary({ verdict: "failed" })]));
    await user.click(screen.getByRole("link", { name: /Open run abc1234/ }));
    const initialGrid = await screen.findByRole("grid", { name: "Run steps" });
    // Progress does not fetch raw logs just to populate the optional preview.
    expect(within(initialGrid).queryByText(/boom: build failed/)).toBeNull();
    expect(getE2eRunLogs).not.toHaveBeenCalled();

    // Once Logs has loaded the bounded page, returning to Progress reuses that
    // buffer for the failed-step preview without another request.
    await user.click(screen.getByRole("tab", { name: "Logs" }));
    expect((await screen.findByTestId("e2e-log-line")).textContent).toContain(
      "boom: build failed",
    );
    await user.click(screen.getByRole("tab", { name: "Progress" }));
    const grid = await screen.findByRole("grid", { name: "Run steps" });
    expect(await within(grid).findByText(/boom: build failed/)).toBeDefined();
    expect(getE2eRunLogs).toHaveBeenCalledTimes(1);
  });

  it("keeps the terminal failure visible outside the diagnostic logs", async () => {
    const user = userEvent.setup();
    const d = detail({
      verdict: "failed",
      failure: {
        stepId: "P0.1",
        message: "intentional OpenShift E2E failure",
      },
    });
    const { api } = makeApi(d);
    renderView(api, enabled([summary({ verdict: "failed" })]));
    await user.click(screen.getByRole("link", { name: /Open run abc1234/ }));

    expect(await screen.findByText("This run failed.")).toBeDefined();
    expect(screen.getByText(/Failed step:/)).toBeDefined();
    expect(screen.getByText("P0.1")).toBeDefined();
    expect(screen.getByText("intentional OpenShift E2E failure")).toBeDefined();
  });

  it("shows failure diagnostics as a selectable section before suite teardown", async () => {
    const user = userEvent.setup();
    const d = detail({
      verdict: "failed",
      steps: [
        {
          stepId: "cleanup",
          name: "suite-teardown",
          state: "passed",
          startedAt: "2026-01-01T00:34:00Z",
          endedAt: "2026-01-01T00:35:00Z",
          durationMs: 60000,
        },
        {
          stepId: "diagnostics",
          name: "failure-diagnostics",
          state: "passed",
          startedAt: "2026-01-01T00:34:05Z",
          endedAt: "2026-01-01T00:34:45Z",
          durationMs: 40000,
        },
      ],
    });
    const getE2eRunLogs = vi
      .fn()
      .mockResolvedValue(
        plane({ entries: [], nextCursor: null, hasMore: false }),
      );
    const { api } = makeApi(d, { getE2eRunLogs });
    renderView(api, enabled([summary({ verdict: "failed" })]));
    await user.click(screen.getByRole("link", { name: /Open run abc1234/ }));

    const grid = await screen.findByRole("grid", { name: "Run steps" });
    const teardown = within(grid).getByRole("button", {
      name: "suite teardown",
    });
    const diagnostics = within(grid).getByRole("button", {
      name: "failure diagnostics",
    });
    expect(
      diagnostics.compareDocumentPosition(teardown) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).not.toBe(0);

    // Opening the detail on Progress uses lifecycle data only. Raw logs, including
    // diagnostics, stay idle until the operator opens the Logs tab.
    expect(getE2eRunLogs).not.toHaveBeenCalled();

    await user.click(screen.getByRole("tab", { name: "Logs" }));
    await waitFor(() => {
      expect(getE2eRunLogs).toHaveBeenLastCalledWith(
        "tenant-1/ns/run-1",
        null,
        null,
        expect.anything(),
      );
    });
    expect(
      screen.getByRole("option", { name: "failure-diagnostics" }),
    ).toBeDefined();

    await user.selectOptions(
      screen.getByLabelText("Log scope"),
      screen.getByRole("option", { name: "failure-diagnostics" }),
    );
    await waitFor(() => {
      expect(getE2eRunLogs).toHaveBeenLastCalledWith(
        "tenant-1/ns/run-1",
        null,
        "diagnostics",
        expect.anything(),
      );
    });
  });

  it("keeps live diagnostics out of whole-run logs until explicitly selected", async () => {
    const user = userEvent.setup();
    const d = detail({
      reportingComplete: false,
      verdict: "unknown",
      endedAt: null,
      steps: [
        {
          stepId: "diagnostics",
          name: "failure-diagnostics",
          state: "running",
          startedAt: "2026-01-01T00:34:05Z",
          endedAt: null,
          durationMs: null,
        },
      ],
    });
    const { api, handlers } = makeApi(d);
    renderView(
      api,
      enabled([summary({ reportingComplete: false, verdict: "unknown" })]),
    );
    await user.click(screen.getByRole("link", { name: /Open run abc1234/ }));
    await user.click(await screen.findByRole("tab", { name: "Logs" }));
    await waitFor(() => {
      /* eslint-disable-next-line @typescript-eslint/unbound-method -- asserting mock calls */
      expect(api.getE2eRunLogs).toHaveBeenCalledTimes(1);
    });

    act(() => {
      handlers.current?.onLog({
        sequence: 10,
        timestamp: "t10",
        stepId: "diagnostics",
        message: "diagnostic payload must stay opt-in",
        truncated: false,
      });
      handlers.current?.onLog({
        sequence: 11,
        timestamp: "t11",
        stepId: "cleanup",
        message: "ordinary whole-run line",
        truncated: false,
      });
    });
    expect(await screen.findByText(/ordinary whole-run line/)).toBeDefined();
    expect(
      screen.queryByText(/diagnostic payload must stay opt-in/),
    ).toBeNull();

    await user.selectOptions(
      screen.getByLabelText("Log scope"),
      screen.getByRole("option", { name: "failure-diagnostics" }),
    );
    await waitFor(() => {
      /* eslint-disable-next-line @typescript-eslint/unbound-method -- asserting mock calls */
      expect(api.getE2eRunLogs).toHaveBeenCalledTimes(2);
    });
    act(() => {
      handlers.current?.onLog({
        sequence: 12,
        timestamp: "t12",
        stepId: "diagnostics",
        message: "selected diagnostic payload",
        truncated: false,
      });
    });
    expect(
      await screen.findByText(/selected diagnostic payload/),
    ).toBeDefined();
  });

  it("shows the reporting-incomplete state for a finished-but-terminal-less run", async () => {
    const user = userEvent.setup();
    const d = detail({
      reportingComplete: false,
      verdict: "unknown",
      endedAt: "2026-01-01T00:35:00Z",
    });
    const { api } = makeApi(d);
    renderView(
      api,
      enabled([summary({ reportingComplete: false, verdict: "unknown" })]),
    );
    await user.click(screen.getByRole("link", { name: /Open run abc1234/ }));
    expect(
      await screen.findByText(
        "Reporting incomplete - no final result was recorded.",
      ),
    ).toBeDefined();
  });

  it("surfaces backend-unavailable when the run detail load fails", async () => {
    const user = userEvent.setup();
    const { api } = makeApi(detail(), {
      getE2eRun: vi.fn().mockRejectedValue(new Error("loki down")),
    });
    renderView(api, enabled([summary()]));
    await user.click(screen.getByRole("link", { name: /Open run abc1234/ }));
    expect(
      await screen.findByText("Run details are unavailable right now."),
    ).toBeDefined();
  });

  it("flags truncated run history", () => {
    const { api } = makeApi(detail());
    renderView(api, { ...enabled([summary()]), truncated: true });
    expect(
      screen.getByText("Older runs are not shown - run history is truncated."),
    ).toBeDefined();
  });
});

describe("gate execution", () => {
  // Verifies: FLT-NAV-18.01
  it("opens step logs and preserves filters while changing tabs without a second stream", async () => {
    const user = userEvent.setup();
    // In-progress run: the live tail opens (E2E-LOG-21) so the single-stream
    // invariant across tab changes is observable.
    const { api } = makeApi(detail({ reportingComplete: false }));
    renderGate(
      api,
      <GateExecution
        target={{
          revision: "abc1234",
          cluster: "tenant-1",
          namespace: "ns",
        }}
        data={enabled([summary()])}
      />,
    );
    await user.click(await screen.findByRole("button", { name: "Build" }));
    expect(
      screen.getByRole("tab", { name: "Logs" }).getAttribute("aria-selected"),
    ).toBe("true");
    expect(screen.getByRole("combobox", { name: "Log scope" })).toHaveProperty(
      "value",
      "a",
    );
    await user.type(
      screen.getByRole("textbox", { name: "Search logs" }),
      "build",
    );
    await user.click(screen.getByRole("tab", { name: "Progress" }));
    await user.click(screen.getByRole("tab", { name: "Logs" }));
    expect(screen.getByRole("textbox", { name: "Search logs" })).toHaveProperty(
      "value",
      "build",
    );
    /* eslint-disable-next-line @typescript-eslint/unbound-method -- asserting mock calls */
    expect(api.streamE2eRun).toHaveBeenCalledTimes(1);
  });
  it("keeps the inspected attempt pinned when a newer retry arrives", async () => {
    const { api } = makeApi(detail({ reportingComplete: false }));
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const history = createMemoryHistory({ path: "/", search: "?gate=g1" });
    const view = (runs: readonly E2eRunSummary[]) => (
      <IntlProvider locale="en">
        <QueryClientProvider client={queryClient}>
          <FleetApiProvider value={api}>
            <RouterProvider history={history}>
              <GateExecution
                target={{
                  revision: "abc1234",
                  cluster: "tenant-1",
                  namespace: "ns",
                }}
                data={enabled(runs)}
              />
            </RouterProvider>
          </FleetApiProvider>
        </QueryClientProvider>
      </IntlProvider>
    );
    const rendered = render(view([summary()]));
    await screen.findByRole("button", { name: "Build" });
    rendered.rerender(
      view([
        summary({
          id: "tenant-1/ns/run-2",
          runId: "run-2",
          startedAt: "2026-01-01T00:40:00Z",
        }),
        summary(),
      ]),
    );
    expect(
      screen.getByRole("combobox", { name: "Test attempt" }),
    ).toHaveProperty("value", "tenant-1/ns/run-1");
    /* eslint-disable-next-line @typescript-eslint/unbound-method -- asserting mock calls */
    expect(api.streamE2eRun).toHaveBeenCalledTimes(1);
  });
  // Verifies: FLT-NAV-10.02
  it("follows the current attempt in explicit live mode (FLT-NAV-10)", async () => {
    const { api } = makeApi(detail({ reportingComplete: false }));
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const history = createMemoryHistory({
      path: "/",
      search: "?gate=g1&gattempt=live",
    });
    const view = (runs: readonly E2eRunSummary[]) => (
      <IntlProvider locale="en">
        <QueryClientProvider client={queryClient}>
          <FleetApiProvider value={api}>
            <RouterProvider history={history}>
              <GateExecution
                target={{
                  revision: "abc1234",
                  cluster: "tenant-1",
                  namespace: "ns",
                }}
                data={enabled(runs)}
              />
            </RouterProvider>
          </FleetApiProvider>
        </QueryClientProvider>
      </IntlProvider>
    );
    const rendered = render(view([summary()]));
    await screen.findByRole("button", { name: "Build" });
    // A newer retry arrives; live mode follows it (a second stream for the new run).
    rendered.rerender(
      view([
        summary({
          id: "tenant-1/ns/run-2",
          runId: "run-2",
          startedAt: "2026-01-01T00:40:00Z",
        }),
        summary(),
      ]),
    );
    // The URL still addresses the live/latest mode, distinct from a pinned attempt.
    expect(
      screen.getByRole("combobox", { name: "Test attempt" }),
    ).toHaveProperty("value", "live");
    // The new run's live tail opens once its (in-progress) snapshot resolves, so the
    // stream count reaches two (E2E-LOG-21).
    await waitFor(() => {
      /* eslint-disable-next-line @typescript-eslint/unbound-method -- asserting mock calls */
      expect(api.streamE2eRun).toHaveBeenCalledTimes(2);
    });
  });
  it("does not open a different revision's latest run", () => {
    const { api } = makeApi(detail());
    renderGate(
      api,
      <GateExecution
        target={{ revision: "other", cluster: "tenant-1", namespace: "ns" }}
        data={enabled([summary()])}
      />,
    );
    expect(
      screen.getByText("Waiting for a test run for this revision."),
    ).toBeDefined();
    /* eslint-disable-next-line @typescript-eslint/unbound-method -- asserting mock calls */
    expect(api.getE2eRun).not.toHaveBeenCalled();
  });
});

describe("log pagination and degraded log pane", () => {
  interface MockObserver {
    cb: IntersectionObserverCallback;
    elements: Element[];
  }
  let observers: MockObserver[];
  const originalIO = globalThis.IntersectionObserver;

  beforeEach(() => {
    observers = [];
    class MockIntersectionObserver {
      cb: IntersectionObserverCallback;
      elements: Element[] = [];
      constructor(cb: IntersectionObserverCallback) {
        this.cb = cb;
        observers.push(this);
      }
      observe(el: Element): void {
        this.elements.push(el);
      }
      unobserve(): void {
        /* no-op */
      }
      disconnect(): void {
        /* no-op */
      }
      takeRecords(): IntersectionObserverEntry[] {
        return [];
      }
    }
    globalThis.IntersectionObserver =
      MockIntersectionObserver as unknown as typeof IntersectionObserver;
  });

  afterEach(() => {
    globalThis.IntersectionObserver = originalIO;
  });

  /** Fire an intersection for every observed sentinel, as a scroll-into-view would. */
  function scrollSentinelIntoView(): void {
    act(() => {
      for (const o of observers) {
        o.cb(
          o.elements.map(
            (target) =>
              ({ isIntersecting: true, target }) as IntersectionObserverEntry,
          ),
          o as unknown as IntersectionObserver,
        );
      }
    });
  }

  const logLine = (sequence: number) => ({
    sequence,
    timestamp: `t${String(sequence)}`,
    stepId: null,
    message: `m${String(sequence)}`,
    truncated: false,
  });

  const logStepLine = (sequence: number, stepId: string, message: string) => ({
    sequence,
    timestamp: `t${String(sequence)}`,
    stepId,
    message,
    truncated: false,
  });

  // Verifies: E2E-LOG-22.01
  it("auto-loads older pages from the scroll observer (no Load more button) and stops at the end", async () => {
    const user = userEvent.setup();
    const getE2eRunLogs = vi
      .fn()
      .mockResolvedValueOnce(
        plane({
          entries: [logLine(5), logLine(6)],
          nextCursor: "500",
          hasMore: true,
        }),
      )
      .mockResolvedValueOnce(
        plane({
          entries: [logLine(3), logLine(4)],
          nextCursor: null,
          hasMore: false,
        }),
      );
    const { api } = makeApi(detail({ reportingComplete: false }), {
      getE2eRunLogs,
    });
    renderView(
      api,
      enabled([summary({ reportingComplete: false, verdict: "unknown" })]),
    );
    await user.click(screen.getByRole("link", { name: /Open run abc1234/ }));
    await user.click(await screen.findByRole("tab", { name: "Logs" }));

    // The newest page loaded on open.
    await screen.findByText("m6");
    expect(screen.queryByText("m4")).toBeNull();
    // There is no "Load more" button: older pages load from the observer.
    expect(screen.queryByRole("button", { name: /load more/i })).toBeNull();

    // Scrolling the top sentinel into view loads the older page.
    scrollSentinelIntoView();
    await screen.findByText("m4");
    expect(getE2eRunLogs).toHaveBeenCalledTimes(2);

    // No older page remains: a further intersection does not fetch again.
    scrollSentinelIntoView();
    expect(getE2eRunLogs).toHaveBeenCalledTimes(2);
  });

  // A stale older-page fetch stops automatic paging behind an explicit, recoverable
  // Retry and keeps the loaded lines, rather than silently re-firing against the
  // still-visible sentinel (E2E-LOG-26.04, superseding the old silent-stop behavior).
  // Verifies: E2E-LOG-26.01, E2E-LOG-26.04
  it("stops auto-paging and offers Retry when an older page comes back stale", async () => {
    const user = userEvent.setup();
    const getE2eRunLogs = vi
      .fn()
      .mockResolvedValueOnce(
        plane({
          entries: [logLine(5), logLine(6)],
          nextCursor: "500",
          hasMore: true,
        }),
      )
      .mockResolvedValueOnce({
        data: { entries: [], nextCursor: null, hasMore: false },
        generatedAt: "2026-01-01T00:00:00Z",
        stale: true,
        error: "e2e log backend unavailable",
      })
      .mockResolvedValueOnce(
        plane({
          entries: [logLine(3), logLine(4)],
          nextCursor: null,
          hasMore: false,
        }),
      );
    const { api } = makeApi(detail({ reportingComplete: false }), {
      getE2eRunLogs,
    });
    renderView(
      api,
      enabled([summary({ reportingComplete: false, verdict: "unknown" })]),
    );
    await user.click(screen.getByRole("link", { name: /Open run abc1234/ }));
    await user.click(await screen.findByRole("tab", { name: "Logs" }));

    // The newest page loaded on open.
    await screen.findByText("m6");

    // Scrolling the sentinel into view fetches the older page, which is stale.
    scrollSentinelIntoView();
    await waitFor(() => {
      expect(getE2eRunLogs).toHaveBeenCalledTimes(2);
    });

    // The already-loaded lines are kept, the whole-panel error is not shown, and the
    // explicit Retry action is exposed.
    expect(screen.getByText("m6")).toBeDefined();
    expect(
      screen.queryByText("Run details are unavailable right now."),
    ).toBeNull();
    const retry = await screen.findByRole("button", { name: "Retry" });

    // Auto-fetch is stopped: a further intersection does NOT re-issue the request.
    scrollSentinelIntoView();
    expect(getE2eRunLogs).toHaveBeenCalledTimes(2);

    // Retry recovers the older page.
    await user.click(retry);
    await screen.findByText("m4");
    expect(getE2eRunLogs).toHaveBeenCalledTimes(3);
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
  });

  // Verifies: E2E-LOG-26.01, E2E-LOG-26.04, E2E-LOG-20
  it("shows an explicit failed state with Retry when the initial log load fails", async () => {
    const user = userEvent.setup();
    const staleLogs = {
      data: { entries: [], nextCursor: null, hasMore: false },
      generatedAt: "2026-01-01T00:00:00Z",
      stale: true,
      error: "e2e log backend unavailable",
    };
    const getE2eRunLogs = vi
      .fn()
      .mockResolvedValueOnce(staleLogs)
      .mockResolvedValueOnce(
        plane({ entries: [logLine(7)], nextCursor: null, hasMore: false }),
      );
    const { api } = makeApi(detail({ reportingComplete: false }), {
      getE2eRunLogs,
    });
    renderView(
      api,
      enabled([summary({ reportingComplete: false, verdict: "unknown" })]),
    );
    await user.click(screen.getByRole("link", { name: /Open run abc1234/ }));
    await user.click(await screen.findByRole("tab", { name: "Logs" }));

    // The log-pane-only failed notice, NOT the whole-panel backend-unavailable message.
    expect(
      await screen.findByText("Could not load log history."),
    ).toBeDefined();
    expect(
      screen.queryByText("Run details are unavailable right now."),
    ).toBeNull();
    // Steps still render.
    expect(screen.getByRole("tab", { name: "Progress" })).toBeDefined();

    // Retry recovers the load.
    await user.click(screen.getByRole("button", { name: "Retry" }));
    await screen.findByText("m7");
    expect(screen.queryByText("Could not load log history.")).toBeNull();
  });

  // The selected-step view requests that step's logs directly, scoped to
  // (run id, step id), rather than filtering a run-wide buffer at render time
  // (E2E-LOG-23.01).
  // Verifies: E2E-LOG-23.01
  it("requests the selected step's logs scoped to that step", async () => {
    const user = userEvent.setup();
    const runWidePage = plane({
      entries: [logStepLine(1, "a", "run-wide-line")],
      nextCursor: null,
      hasMore: false,
    });
    const stepBPage = plane({
      entries: [logStepLine(2, "b", "step-b-line")],
      nextCursor: null,
      hasMore: false,
    });
    // Call order: run-wide reader fetches on open, then the step reader fetches when
    // step b is selected.
    const getE2eRunLogs = vi
      .fn()
      .mockResolvedValueOnce(runWidePage)
      .mockResolvedValueOnce(stepBPage);
    const { api } = makeApi(detail(), { getE2eRunLogs });
    renderView(api, enabled([summary()]));
    await user.click(screen.getByRole("link", { name: /Open run abc1234/ }));
    await user.click(await screen.findByRole("tab", { name: "Logs" }));

    // Run-wide scope on open: the line renders with its step-id prefix, so match the
    // message substring.
    await screen.findByText(/run-wide-line/);
    expect(getE2eRunLogs).toHaveBeenLastCalledWith(
      "tenant-1/ns/run-1",
      null,
      null,
      expect.anything(),
    );

    // Selecting step b re-requests scoped to that step.
    await user.selectOptions(
      screen.getByLabelText("Log scope"),
      screen.getByRole("option", { name: "Test" }),
    );
    await screen.findByText("step-b-line");
    expect(getE2eRunLogs).toHaveBeenLastCalledWith(
      "tenant-1/ns/run-1",
      null,
      "b",
      expect.anything(),
    );
  });

  // Rapid run/step switching aborts the obsolete request and ignores its late
  // result, so no earlier scope's lines contaminate the newly selected scope
  // (E2E-LOG-23.03).
  // Verifies: E2E-LOG-23.03
  it("ignores a late result from a previous scope when switching rapidly", async () => {
    const user = userEvent.setup();
    let resolveStepA!: (v: Plane<E2eLogPage>) => void;
    const stepA = new Promise<Plane<E2eLogPage>>((resolve) => {
      resolveStepA = resolve;
    });
    const emptyPage = plane({ entries: [], nextCursor: null, hasMore: false });
    const stepBPage = plane({
      entries: [logStepLine(2, "b", "step-b-line")],
      nextCursor: null,
      hasMore: false,
    });
    // Call order: run-wide on open, then step a (deferred), then step b.
    const getE2eRunLogs = vi
      .fn()
      .mockResolvedValueOnce(emptyPage)
      .mockReturnValueOnce(stepA)
      .mockResolvedValueOnce(stepBPage);
    const { api } = makeApi(detail(), { getE2eRunLogs });
    renderView(api, enabled([summary()]));
    await user.click(screen.getByRole("link", { name: /Open run abc1234/ }));
    await user.click(await screen.findByRole("tab", { name: "Logs" }));

    // Select step a (its request hangs), then rapidly switch to step b.
    await user.selectOptions(
      screen.getByLabelText("Log scope"),
      screen.getByRole("option", { name: "Build" }),
    );
    await user.selectOptions(
      screen.getByLabelText("Log scope"),
      screen.getByRole("option", { name: "Test" }),
    );
    await screen.findByText("step-b-line");

    // Step a's response resolves late; it must be ignored (its request was aborted on
    // the scope change), not merged into step b's view.
    act(() => {
      resolveStepA(
        plane({
          entries: [logStepLine(1, "a", "step-a-late")],
          nextCursor: null,
          hasMore: false,
        }),
      );
    });
    await waitFor(() => {
      expect(screen.queryByText("step-a-late")).toBeNull();
    });
    expect(screen.getByText("step-b-line")).toBeDefined();
  });

  // A completed step that emitted no logs settles into a clear empty state rather
  // than a persistent loading indicator (E2E-LOG-26.02).
  // Verifies: E2E-LOG-26.02
  it("settles a completed step with no logs into an empty state", async () => {
    const user = userEvent.setup();
    const { api } = makeApi(detail(), {
      getE2eRunLogs: vi
        .fn()
        .mockResolvedValue(
          plane({ entries: [], nextCursor: null, hasMore: false }),
        ),
    });
    renderView(api, enabled([summary()]));
    await user.click(screen.getByRole("link", { name: /Open run abc1234/ }));
    await user.click(await screen.findByRole("tab", { name: "Logs" }));

    // Scope to a completed step that emitted no logs.
    await user.selectOptions(
      screen.getByLabelText("Log scope"),
      screen.getByRole("option", { name: "Build" }),
    );

    expect(await screen.findByText("No log output.")).toBeDefined();
    // No stuck loading indicator and no failed/Retry state.
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
  });

  // An older page whose oldest entries all share the cursor's nanosecond is reported
  // by the backend as a locally-limited boundary (boundaryTruncated), not end-of-
  // history. The pane shows the locally-limited state and does not claim all history
  // was read, nor keep re-requesting the same older page (E2E-LOG-26.05).
  // Verifies: E2E-LOG-26.05
  it("renders a same-nanosecond boundary as locally limited, not end-of-history", async () => {
    const getE2eRunLogs = vi
      .fn()
      .mockResolvedValueOnce(
        plane({
          entries: [logLine(5), logLine(6)],
          nextCursor: "500",
          hasMore: true,
        }),
      )
      .mockResolvedValueOnce(
        plane({
          entries: [logLine(3), logLine(4)],
          nextCursor: null,
          hasMore: false,
          boundaryTruncated: true,
        }),
      );
    const user = userEvent.setup();
    const { api } = makeApi(detail({ reportingComplete: false }), {
      getE2eRunLogs,
    });
    renderView(
      api,
      enabled([summary({ reportingComplete: false, verdict: "unknown" })]),
    );
    await user.click(screen.getByRole("link", { name: /Open run abc1234/ }));
    await user.click(await screen.findByRole("tab", { name: "Logs" }));

    // Newest page on open, then the older page that hits the same-nanosecond boundary.
    await screen.findByText("m6");
    scrollSentinelIntoView();
    await screen.findByText("m4");
    expect(getE2eRunLogs).toHaveBeenCalledTimes(2);

    // The locally-limited state is shown, NOT end-of-history.
    expect(
      screen.getByText("Showing the most recent log lines held in memory."),
    ).toBeDefined();
    expect(screen.queryByText("Beginning of log history.")).toBeNull();

    // Auto-fetch stops: a further intersection does not re-request the boundary page.
    scrollSentinelIntoView();
    expect(getE2eRunLogs).toHaveBeenCalledTimes(2);
  });

  it("extracts Messages for the failed-run header and reveals the full diff when the failed state is expanded", async () => {
    const user = userEvent.setup();
    const rawDiff =
      'Not equal: | expected: "Running" | actual: "Provisioning" | Diff: | --- Expected | +++ Actual | @@ -1 +1 @@ | -Running | +Provisioning | primary gateway did not reach Running';
    const d = detail({
      verdict: "failed",
      failure: {
        stepId: "P1.1",
        message: rawDiff,
      },
      steps: [
        {
          stepId: "P1.1",
          name: "Primary gateway",
          state: "failed",
          startedAt: "2026-01-01T00:30:00Z",
          endedAt: "2026-01-01T00:31:30Z",
          durationMs: 0,
        },
      ],
      logs: [
        {
          sequence: 1,
          timestamp: "2026-01-01T00:31:30Z",
          stepId: "P1.1",
          message: rawDiff,
          truncated: false,
        },
      ],
    });
    const { api } = makeApi(d);
    renderView(api, enabled([summary({ verdict: "failed" })]));

    await user.click(screen.getByRole("link", { name: /Open run abc1234/ }));
    await screen.findByText("This run failed.");

    // The failed-run header and the failed step both show the concise Messages
    // text (not the raw diff), and the header names the failed step.
    expect(
      screen.getAllByText("primary gateway did not reach Running").length,
    ).toBeGreaterThan(0);
    expect(screen.getByText("Failed step:")).toBeDefined();

    const stepsTable = (
      await screen.findByRole("button", { name: "Primary gateway" })
    ).closest("table");
    if (!stepsTable) throw new Error("expected a steps table");
    // The full diff is hidden until the failed step's details are expanded.
    expect(within(stepsTable).queryByText(/\+Provisioning/)).toBeNull();
    // Step duration derives 1m 30s from startedAt/endedAt even though durationMs was 0.
    expect(within(stepsTable).getByText("1m 30s")).toBeDefined();

    // Expanding "Show details" reveals the full diff, de-collapsed to one line
    // per segment in the mono code panel.
    await user.click(
      within(stepsTable).getByRole("button", { name: "Show details" }),
    );
    expect(within(stepsTable).getByText(/\+Provisioning/)).toBeDefined();
  });

  it("shows derived non-zero duration in the runs table for failed runs when durationMs is 0", () => {
    const { api } = makeApi(detail());
    renderView(
      api,
      enabled([
        summary({
          verdict: "failed",
          startedAt: "2026-01-01T00:30:00Z",
          endedAt: "2026-01-01T00:32:15Z",
          durationMs: 0,
        }),
      ]),
    );
    expect(screen.getByText("2m 15s")).toBeDefined();
  });

  it("nests multiple attempts of the same revision under an attempt counter, latest first", async () => {
    const user = userEvent.setup();
    const { api } = makeApi(detail());
    const attempt1 = summary({
      id: "tenant-1/ns/run-1",
      runId: "1111aaaa",
      revision: "rev99999",
      startedAt: "2026-01-01T00:10:00Z",
      verdict: "failed",
    });
    const attempt2 = summary({
      id: "tenant-1/ns/run-2",
      runId: "2222bbbb",
      revision: "rev99999",
      startedAt: "2026-01-01T00:20:00Z",
      verdict: "failed",
    });
    const attempt3 = summary({
      id: "tenant-1/ns/run-3",
      runId: "3333cccc",
      revision: "rev99999",
      startedAt: "2026-01-01T00:30:00Z",
      verdict: "passed",
    });
    renderView(api, enabled([attempt1, attempt2, attempt3]));

    // Primary row displays the latest attempt as "Most Recent" and the total count badge.
    expect(screen.getByText("Most Recent")).toBeDefined();
    expect(screen.getByText("3 attempts")).toBeDefined();

    // Expand nested attempts to reveal Attempt 2 and Attempt 1.
    const toggle = screen.getByRole("button", {
      name: "Show 2 earlier attempts",
    });
    expect(toggle.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByRole("link", { name: /1111aaaa/ })).toBeNull();
    const hiddenAttempt = screen.getByText("Attempt 1").closest("tr");
    expect(hiddenAttempt?.hasAttribute("inert")).toBe(true);
    expect(hiddenAttempt?.getAttribute("aria-hidden")).toBe("true");
    await user.click(toggle);
    expect(
      screen
        .getByRole("button", { name: "Hide earlier attempts" })
        .getAttribute("aria-expanded"),
    ).toBe("true");
    expect(screen.getByText("Attempt 2")).toBeDefined();
    expect(screen.getByText("Attempt 1")).toBeDefined();
    expect(hiddenAttempt?.hasAttribute("inert")).toBe(false);
    expect(hiddenAttempt?.getAttribute("aria-hidden")).not.toBe("true");
  });

  it("sorts attempt groups by the latest visible attempt", async () => {
    const user = userEvent.setup();
    const { api } = makeApi(detail());
    renderView(
      api,
      enabled([
        summary({
          id: "old",
          runId: "old",
          revision: "group-a",
          startedAt: "2026-01-01T00:00:00Z",
          durationMs: 1000,
        }),
        summary({
          id: "latest",
          runId: "latest",
          revision: "group-a",
          startedAt: "2026-01-01T00:40:00Z",
          durationMs: 10_000,
        }),
        summary({
          id: "other",
          runId: "other",
          revision: "group-b",
          startedAt: "2026-01-01T00:20:00Z",
          durationMs: 5000,
        }),
      ]),
    );
    await user.click(screen.getByRole("button", { name: "Duration" }));
    expect(
      screen
        .getAllByRole("link", { name: /Open run/ })[0]
        ?.getAttribute("aria-label"),
    ).toContain("group-b");
    await user.click(screen.getByRole("button", { name: "Duration" }));
    expect(
      screen
        .getAllByRole("link", { name: /Open run/ })[0]
        ?.getAttribute("aria-label"),
    ).toContain("group-a");
  });

  it("restores focus to the group toggle when the selected attempt is collapsed", async () => {
    const user = userEvent.setup();
    const { api } = makeApi(detail());
    renderView(
      api,
      enabled([
        summary(),
        summary({
          id: "tenant-1/ns/run-2",
          runId: "run-2",
          startedAt: "2026-01-01T00:40:00Z",
        }),
      ]),
    );
    await user.click(
      screen.getByRole("button", { name: "Show 1 earlier attempt" }),
    );
    await user.click(
      screen.getByRole("link", { name: /Open run abc1234.*run-1/ }),
    );
    await screen.findByRole("button", { name: "Build" });
    await user.click(
      screen.getByRole("button", { name: "Hide earlier attempts" }),
    );
    await user.click(screen.getByRole("button", { name: "Close details" }));
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "Show 1 earlier attempt" }),
    );
  });

  it("localizes diagnostic severity and uses semantic danger colors", async () => {
    const user = userEvent.setup();
    const { api } = makeApi(
      detail({
        steps: [
          {
            stepId: "diagnostics",
            name: "failure diagnostics",
            state: "passed",
            startedAt: null,
            endedAt: null,
            durationMs: 1000,
          },
        ],
        diagnostics: [
          { title: "All pods", body: "CrashLoopBackOff", severity: "error" },
        ],
      }),
    );
    renderView(api, enabled([summary()]), {
      "fleet.e2e.diagnostics.error": "Erreur",
    });
    await user.click(screen.getByRole("link", { name: /Open run abc1234/ }));
    const region = await screen.findByRole("region", { name: "Diagnostics" });
    await user.click(
      within(region).getByRole("button", { name: "Show Diagnostics" }),
    );
    const severity = await within(region).findByText("Erreur");
    expect(severity.querySelector("svg")?.getAttribute("style")).toContain(
      "--pf-t--global--color--status--danger--default",
    );
    expect(within(region).queryByText("error")).toBeNull();
  });

  it.each(["rejected", "degraded"])(
    "shows a recoverable diagnostics failure for a %s response",
    async (failure) => {
      const user = userEvent.setup();
      const runDetail = detail({
        steps: [
          {
            stepId: "diagnostics",
            name: "failure diagnostics",
            state: "passed",
            startedAt: null,
            endedAt: null,
            durationMs: 1000,
          },
        ],
      });
      const getE2eDiagnostics =
        vi.fn<() => Promise<Plane<E2eDiagnosticsData>>>();
      if (failure === "rejected") {
        getE2eDiagnostics.mockRejectedValueOnce(
          new Error("network unavailable"),
        );
      } else {
        getE2eDiagnostics.mockResolvedValueOnce({
          ...plane({ diagnostics: [] }),
          stale: true,
          error: "e2e log backend unavailable",
        });
      }
      getE2eDiagnostics.mockResolvedValue(
        plane({
          diagnostics: [
            {
              title: "Recovered diagnostics",
              body: "available",
              severity: "info",
            },
          ],
        }),
      );
      const { api } = makeApi(runDetail, { getE2eDiagnostics });
      renderView(api, enabled([summary()]));
      await user.click(screen.getByRole("link", { name: /Open run abc1234/ }));
      const region = await screen.findByRole("region", { name: "Diagnostics" });
      await user.click(
        within(region).getByRole("button", { name: "Show Diagnostics" }),
      );
      expect(
        await within(region).findByText("Could not load diagnostics."),
      ).toBeDefined();
      expect(
        within(region).queryByText("No failure diagnostics collected."),
      ).toBeNull();
      await user.click(within(region).getByRole("button", { name: "Retry" }));
      expect(
        await within(region).findByText("Recovered diagnostics"),
      ).toBeDefined();
      expect(
        within(region).queryByText("Could not load diagnostics."),
      ).toBeNull();
      expect(getE2eDiagnostics).toHaveBeenCalledTimes(2);
    },
  );

  it("renders a collapsed-by-default Show Diagnostics toggle that fetches lazily on expand, and stays disabled/greyed-out when skipped or not-run", async () => {
    const user = userEvent.setup();
    const dWithDiagStep = detail({
      verdict: "failed",
      steps: [
        {
          stepId: "P1.1",
          name: "Primary gateway",
          state: "failed",
          startedAt: "2026-01-01T00:30:00Z",
          endedAt: "2026-01-01T00:31:00Z",
          durationMs: 60_000,
        },
        {
          stepId: "diagnostics",
          name: "failure diagnostics",
          state: "passed",
          startedAt: "2026-01-01T00:31:00Z",
          endedAt: "2026-01-01T00:31:03Z",
          durationMs: 3_000,
        },
      ],
    });
    const getE2eDiagnostics = vi.fn().mockResolvedValue(
      plane({
        diagnostics: [
          {
            title: "All pods",
            body: "hypershell-server-0   0/1   CrashLoopBackOff",
            severity: "error" as const,
          },
          {
            title: "Platform events",
            body: "Warning BackOff restarting failed container",
            severity: "warn" as const,
          },
        ],
      }),
    );
    const { api } = makeApi(dWithDiagStep, { getE2eDiagnostics });
    const view = render(
      <IntlProvider locale="en">
        <QueryClientProvider
          client={
            new QueryClient({ defaultOptions: { queries: { retry: false } } })
          }
        >
          <FleetApiProvider value={api}>
            <RouterProvider
              history={createMemoryHistory({ path: "/e2e", search: "" })}
            >
              <E2eRunView
                data={enabled([summary({ verdict: "failed" })])}
                nowMs={NOW}
              />
            </RouterProvider>
          </FleetApiProvider>
        </QueryClientProvider>
      </IntlProvider>,
    );

    await user.click(screen.getByRole("link", { name: /Open run abc1234/ }));
    const diagSection = await screen.findByRole("region", {
      name: "Diagnostics",
    });

    // Collapsed by default: no fetch has happened yet.
    const showBtn = within(diagSection).getByRole("button", {
      name: "Show Diagnostics",
    });
    expect(showBtn.getAttribute("aria-expanded")).toBe("false");
    expect(getE2eDiagnostics).not.toHaveBeenCalled();

    // Expanding triggers the lazy fetch and renders the sections.
    await user.click(showBtn);
    expect(showBtn.getAttribute("aria-expanded")).toBe("true");
    expect(getE2eDiagnostics).toHaveBeenCalledTimes(1);
    expect(await within(diagSection).findByText("All pods")).toBeDefined();
    expect(
      within(diagSection).getByText(
        /hypershell-server-0\s+0\/1\s+CrashLoopBackOff/,
      ),
    ).toBeDefined();
    expect(within(diagSection).getByText("Platform events")).toBeDefined();
    view.unmount();

    // Passing run with skipped diagnostics step -> toggle disabled, Skipped note, no fetch
    const skippedGetDiags = vi.fn();
    const { api: passedApi } = makeApi(
      detail({
        verdict: "passed",
        steps: [
          {
            stepId: "diagnostics",
            name: "failure diagnostics",
            state: "skipped",
            startedAt: null,
            endedAt: null,
            durationMs: null,
          },
        ],
      }),
      { getE2eDiagnostics: skippedGetDiags },
    );
    const passedView = render(
      <IntlProvider locale="en">
        <QueryClientProvider
          client={
            new QueryClient({ defaultOptions: { queries: { retry: false } } })
          }
        >
          <FleetApiProvider value={passedApi}>
            <RouterProvider
              history={createMemoryHistory({ path: "/e2e", search: "" })}
            >
              <E2eRunView
                data={enabled([summary({ verdict: "passed" })])}
                nowMs={NOW}
              />
            </RouterProvider>
          </FleetApiProvider>
        </QueryClientProvider>
      </IntlProvider>,
    );
    await user.click(screen.getByRole("link", { name: /Open run abc1234/ }));
    const passedDiag = await screen.findByRole("region", {
      name: "Diagnostics",
    });
    const passedBtn = within(passedDiag).getByRole("button", {
      name: "Show Diagnostics",
    });
    expect(passedBtn.hasAttribute("disabled")).toBe(true);
    // The skipped state shows a single note, not a duplicate "Skipped" chip.
    expect(within(passedDiag).getByText("Skipped (run passed)")).toBeDefined();
    expect(skippedGetDiags).not.toHaveBeenCalled();
    passedView.unmount();

    // Failed/in-progress run with not-run diagnostics step -> toggle disabled, Not run note, no fetch
    const notRunGetDiags = vi.fn();
    const { api: emptyFailedApi } = makeApi(
      detail({
        verdict: "failed",
        steps: [
          {
            stepId: "diagnostics",
            name: "failure diagnostics",
            state: "not-run",
            startedAt: null,
            endedAt: null,
            durationMs: null,
          },
        ],
      }),
      { getE2eDiagnostics: notRunGetDiags },
    );
    renderView(emptyFailedApi, enabled([summary({ verdict: "failed" })]));
    await user.click(screen.getByRole("link", { name: /Open run abc1234/ }));
    const emptyFailedDiag = await screen.findByRole("region", {
      name: "Diagnostics",
    });
    const notRunBtn = within(emptyFailedDiag).getByRole("button", {
      name: "Show Diagnostics",
    });
    expect(notRunBtn.hasAttribute("disabled")).toBe(true);
    // Only the single note is shown for the not-run state (no duplicate chip).
    expect(
      within(emptyFailedDiag).getByText("No failure diagnostics collected."),
    ).toBeDefined();
    expect(notRunGetDiags).not.toHaveBeenCalled();
  });

  it("renders steps in canonical step order by default and allows sorting by failure, alphabetically, or duration", async () => {
    const user = userEvent.setup();
    const dUnordered = detail({
      verdict: "failed",
      steps: [
        {
          stepId: "cleanup",
          name: "suite-teardown",
          state: "passed",
          startedAt: "2026-01-01T00:05:00Z",
          endedAt: "2026-01-01T00:05:05Z",
          durationMs: 5_000,
        },
        {
          stepId: "P2.7",
          name: "vault-fail-closed",
          state: "failed",
          startedAt: "2026-01-01T00:03:00Z",
          endedAt: "2026-01-01T00:03:30Z",
          durationMs: 30_000,
        },
        {
          stepId: "P0.1",
          name: "authentication",
          state: "passed",
          startedAt: "2026-01-01T00:01:00Z",
          endedAt: "2026-01-01T00:01:10Z",
          durationMs: 10_000,
        },
        {
          stepId: "P3.1",
          name: "audit-log-continuity",
          state: "not-run",
          startedAt: null,
          endedAt: null,
          durationMs: null,
        },
        {
          stepId: "setup",
          name: "suite-setup",
          state: "passed",
          startedAt: "2026-01-01T00:00:00Z",
          endedAt: "2026-01-01T00:00:02Z",
          durationMs: 2_000,
        },
      ],
    });
    const { api } = makeApi(dUnordered);
    renderView(api, enabled([summary({ verdict: "failed" })]));

    await user.click(screen.getByRole("link", { name: /Open run abc1234/ }));
    const stepsTable = await screen.findByRole("grid", { name: "Run steps" });

    const stepButtonNames = () =>
      within(stepsTable)
        .getAllByRole("button")
        .filter((btn) =>
          [
            "suite setup",
            "authentication",
            "vault fail closed",
            "audit log continuity",
            "suite teardown",
          ].includes(btn.textContent || ""),
        )
        .map((btn) => btn.textContent);

    // Default order is canonical E2E execution order: setup -> P0.1 -> P2.7 -> P3.1 -> cleanup
    expect(stepButtonNames()).toEqual([
      "suite setup",
      "authentication",
      "vault fail closed",
      "audit log continuity",
      "suite teardown",
    ]);

    const sortSelect = screen.getByRole("combobox", { name: "Sort steps" });

    // Sort by failure (Failed first)
    await user.selectOptions(sortSelect, "1:asc");
    expect(stepButtonNames()[0]).toBe("vault fail closed");
    expect(stepButtonNames()[4]).toBe("audit log continuity");

    // Sort alphabetically (A-Z)
    await user.selectOptions(sortSelect, "0:asc");
    expect(stepButtonNames()).toEqual([
      "audit log continuity",
      "authentication",
      "suite setup",
      "suite teardown",
      "vault fail closed",
    ]);

    // Sort by duration (longest first)
    await user.selectOptions(sortSelect, "2:desc");
    expect(stepButtonNames()[0]).toBe("vault fail closed");

    // Return to canonical step order
    await user.selectOptions(sortSelect, "order");
    expect(stepButtonNames()).toEqual([
      "suite setup",
      "authentication",
      "vault fail closed",
      "audit log continuity",
      "suite teardown",
    ]);
  });
});
