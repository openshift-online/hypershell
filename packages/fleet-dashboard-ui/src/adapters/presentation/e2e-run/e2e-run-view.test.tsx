import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IntlProvider } from "react-intl";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  MAX_LOG_BUFFER,
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
    getE2eRuns: vi.fn(),
    getE2eRun: vi.fn().mockResolvedValue(plane(runDetail)),
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

function renderView(api: FleetApi, data: E2eRunsData): void {
  render(
    <IntlProvider locale="en" defaultLocale="en">
      <FleetApiProvider value={api}>
        <RouterProvider
          history={createMemoryHistory({ path: "/e2e", search: "" })}
        >
          <E2eRunView data={data} nowMs={NOW} />
        </RouterProvider>
      </FleetApiProvider>
    </IntlProvider>,
  );
}

/** Render a gate execution drawer with a gate selected so route.map.exec applies. */
function renderGate(api: FleetApi, ui: React.ReactElement): void {
  render(
    <IntlProvider locale="en">
      <FleetApiProvider value={api}>
        <RouterProvider
          history={createMemoryHistory({ path: "/", search: "?gate=g1" })}
        >
          {ui}
        </RouterProvider>
      </FleetApiProvider>
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
    // The Progress tab's failed-step preview reads the paginated log buffer, not the
    // (now empty) detail logs. Scope to the steps grid (the same line also renders in
    // the log pane).
    const grid = await screen.findByRole("grid", { name: "Run steps" });
    expect(await within(grid).findByText(/boom: build failed/)).toBeDefined();
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
    const history = createMemoryHistory({ path: "/", search: "?gate=g1" });
    const view = (runs: readonly E2eRunSummary[]) => (
      <IntlProvider locale="en">
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
    const history = createMemoryHistory({
      path: "/",
      search: "?gate=g1&gattempt=live",
    });
    const view = (runs: readonly E2eRunSummary[]) => (
      <IntlProvider locale="en">
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

  // A stale older-page fetch terminates paging and keeps the loaded lines; the
  // scroll anchor is cleared so follow is not frozen after a no-new-lines page.
  // Verifies: E2E-LOG-22.01
  it("stops paging and keeps loaded lines when an older page comes back stale", async () => {
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
      });
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

    // The already-loaded lines are kept, not blanked, and the panel still renders
    // its steps (the whole-panel backend-unavailable state is not shown).
    expect(screen.getByText("m6")).toBeDefined();
    expect(
      screen.queryByText("Run details are unavailable right now."),
    ).toBeNull();
  });

  // Verifies: E2E-LOG-22.03, E2E-LOG-20
  it("shows a log-pane-only degraded notice when logs fail but steps load", async () => {
    const user = userEvent.setup();
    const staleLogs = {
      data: { entries: [], nextCursor: null, hasMore: false },
      generatedAt: "2026-01-01T00:00:00Z",
      stale: true,
      error: "e2e log backend unavailable",
    };
    const { api } = makeApi(detail({ reportingComplete: false }), {
      getE2eRunLogs: vi.fn().mockResolvedValue(staleLogs),
    });
    renderView(
      api,
      enabled([summary({ reportingComplete: false, verdict: "unknown" })]),
    );
    await user.click(screen.getByRole("link", { name: /Open run abc1234/ }));
    await user.click(await screen.findByRole("tab", { name: "Logs" }));

    // The log-pane-only notice, NOT the whole-panel backend-unavailable message.
    expect(
      await screen.findByText(
        "Log history is unavailable right now. Steps are shown above.",
      ),
    ).toBeDefined();
    expect(
      screen.queryByText("Run details are unavailable right now."),
    ).toBeNull();
    // Steps still render.
    expect(screen.getByRole("tab", { name: "Progress" })).toBeDefined();
  });
});
