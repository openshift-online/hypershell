import { act, fireEvent, render, screen } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { AgentSdlcCard, AgentSdlcData } from "../../../domain/agent-sdlc";
import type { SdlcAgentMetrics } from "../../../domain/sdlc-agents";
import { identiName } from "../../../domain/map/identiname";
import { parseRoute } from "../../../domain/navigation/route";
import { createMemoryHistory } from "../../routing/history-port";
import { RouterProvider } from "../../routing/router";
import { AgentSdlcView, type AgentSdlcViewProps } from "./agent-sdlc-view";

const EMPTY: AgentSdlcData = {
  phases: [],
  cards: [],
  lane: [],
  matrix: { hubClusters: [], rows: [] },
};

function renderView(overrides: Partial<AgentSdlcViewProps>): {
  history: ReturnType<typeof createMemoryHistory>;
} {
  const history = createMemoryHistory({ path: "/sdlc", search: "" });
  render(
    <IntlProvider locale="en" defaultLocale="en">
      <RouterProvider history={history}>
        <AgentSdlcView data={EMPTY} selectedBundleSeed={null} {...overrides} />
      </RouterProvider>
    </IntlProvider>,
  );
  return { history };
}

describe("AgentSdlcView board", () => {
  // Verifies: SDLC-VIS-09.02
  it("renders a column per board phase with its card count, and the card's rework meter", () => {
    const data: AgentSdlcData = {
      ...EMPTY,
      phases: [
        { key: "spec", sleSeconds: 100 },
        { key: "shipped", sleSeconds: 0 },
      ],
      cards: [
        {
          number: 10,
          title: "Add tab",
          url: "u",
          phase: "spec",
          // Older than the replay-on-load window so this settled-card render test is not
          // perturbed by a load replay flight (which would hold the card out of its column).
          timeInPhaseSeconds: 5000,
          scope: null,
          escalation: null,
          rework: { count: 2, threshold: 3 },
          history: [],
        },
      ],
    };
    renderView({ data });

    // The Spec column header renders (Shipped is the matrix, not a board column).
    expect(screen.getByText("Spec")).toBeDefined();
    // The rework meter exposes its count/threshold as an accessible meter.
    const meter = screen.getByRole("meter", { name: "2 of 3 rejections" });
    expect(meter.getAttribute("aria-valuenow")).toBe("2");
    expect(meter.getAttribute("aria-valuemax")).toBe("3");
    // At 2 of 3 the remaining-until-block countdown shows (non-color cue).
    expect(
      screen.getByText(/1 more rejection blocks this item\./),
    ).toBeDefined();
  });

  it("shows time-in-phase to minute resolution for a young card", () => {
    const data: AgentSdlcData = {
      ...EMPTY,
      phases: [{ key: "spec", sleSeconds: 100_000 }],
      cards: [
        {
          number: 11,
          title: "eleven minutes in",
          url: "u",
          phase: "spec",
          timeInPhaseSeconds: 11 * 60,
          scope: null,
          escalation: null,
          rework: null,
          history: [],
        },
      ],
    };
    // Freeze the clock so the live-aging delta is zero and the snapshot age renders.
    renderView({ data, nowMs: Date.UTC(2026, 9, 15, 12, 0, 0) });

    // A card 11 minutes old reads "11m", not a misleading "0d 0h".
    expect(screen.getByText("11m")).toBeDefined();
  });
});

describe("AgentSdlcView escalations notification drawer", () => {
  // Verifies: SDLC-VIS-10.04
  it("shows the cleared state when the drawer is opened with an empty lane", () => {
    renderView({ data: EMPTY });
    // The escalations live behind the bell now; open the notification drawer.
    fireEvent.click(screen.getByRole("button", { name: "Escalations" }));
    expect(
      screen.getByText("Escalation lane clear - nothing blocked or stalled."),
    ).toBeDefined();
  });

  it("groups escalations by bucket when the drawer is opened", () => {
    const data: AgentSdlcData = {
      ...EMPTY,
      lane: [
        {
          number: 1,
          title: "blocked one",
          url: "u",
          phase: "code",
          timeInPhaseSeconds: 10,
          scope: null,
          escalation: "blocked",
          rework: null,
          history: [],
        },
        {
          number: 2,
          title: "stale one",
          url: "u",
          phase: "spec",
          timeInPhaseSeconds: 999,
          scope: null,
          escalation: "past-sle",
          rework: null,
          history: [],
        },
      ],
    };
    renderView({ data });
    fireEvent.click(screen.getByRole("button", { name: "Escalations" }));
    expect(screen.getByText("Blocked")).toBeDefined();
    expect(screen.getByText("Past SLE")).toBeDefined();
  });
});

describe("AgentSdlcView matrix", () => {
  // Verifies: FLT-NAV-08.02
  it("shows the Unreleased row and cross-links a bundle to the map", () => {
    const data: AgentSdlcData = {
      ...EMPTY,
      matrix: {
        hubClusters: [
          {
            key: "env-a",
            cluster: "cluster-a",
            env: "prod",
            currentVersion: "v2",
            currentDigest: "d2",
          },
        ],
        rows: [
          {
            version: null,
            digest: null,
            date: null,
            unreleased: true,
            hubClusters: [],
            issues: [],
            otherPRs: [],
          },
          {
            version: "v2",
            digest: "d2",
            date: null,
            unreleased: false,
            hubClusters: ["env-a"],
            issues: [],
            otherPRs: [],
          },
        ],
      },
    };
    const { history } = renderView({ data });

    expect(screen.getByText("Unreleased")).toBeDefined();
    // The current-bundle marker reads with a non-color cue ("current" text).
    expect(screen.getAllByText("current").length).toBeGreaterThan(0);

    // Activating the bundle opens it on the map (shared freight bar + tab jump)
    // as a single addressable navigation (FLT-NAV-08). The bundle reads by its
    // identiname (the stable two-word alias), not the raw version.
    fireEvent.click(screen.getByRole("link", { name: identiName("d2") }));
    const route = parseRoute(history.read());
    expect(route.tab).toBe("map");
    expect(route.bundle).toBe("d2");
  });

  // Verifies: SDLC-VIS-16.02
  it("renders a localized 'merged Nd ago' timestamp using the injected clock", () => {
    const nowMs = Date.UTC(2026, 9, 15, 12, 0, 0);
    const mergedAt = new Date(Date.UTC(2026, 9, 12, 12, 0, 0)).toISOString(); // 3 days earlier
    const data: AgentSdlcData = {
      ...EMPTY,
      matrix: {
        hubClusters: [],
        rows: [
          {
            version: "v9",
            digest: "d9",
            date: null,
            unreleased: false,
            hubClusters: [],
            issues: [
              {
                number: 700,
                title: "shipped thing",
                url: "u",
                mergedAt,
                mergedBy: "octo-dev",
                prs: [
                  {
                    number: 701,
                    title: "deliver it",
                    url: "pr-u",
                    author: "pr-bot",
                    mergedAt,
                  },
                ],
              },
            ],
            otherPRs: [],
          },
        ],
      },
    };
    renderView({ data, nowMs });

    // Expand the single bundle row to reveal its issues.
    fireEvent.click(screen.getByRole("button", { name: "Details" }));
    expect(screen.getByText("merged 3 days ago by octo-dev")).toBeDefined();
  });

  // Verifies: SDLC-VIS-11.02, SDLC-VIS-11.03
  it("splits an expanded bundle into tracked issues and other PRs with a count summary", () => {
    const data: AgentSdlcData = {
      ...EMPTY,
      matrix: {
        hubClusters: [],
        rows: [
          {
            version: "v9",
            digest: "d9",
            date: null,
            unreleased: false,
            hubClusters: [],
            issues: [
              {
                number: 525,
                title: "declarative import",
                url: "i-525",
                mergedAt: null,
                mergedBy: null,
                prs: [
                  {
                    number: 526,
                    title: "import part one",
                    url: "pr-526",
                    author: null,
                    mergedAt: null,
                  },
                ],
              },
            ],
            otherPRs: [
              {
                number: 540,
                title: "bump deps",
                url: "pr-540",
                author: "dependabot",
                mergedAt: null,
              },
              {
                number: 533,
                title: "fix typo",
                url: "pr-533",
                author: null,
                mergedAt: null,
              },
            ],
          },
        ],
      },
    };
    renderView({ data });

    // Row summary shows both counts: 1 tracked issue and 3 total PRs (1 grouped + 2 one-off).
    expect(screen.getByText("1 issue, 3 PRs")).toBeDefined();

    // Expand to reveal both groups.
    fireEvent.click(screen.getByRole("button", { name: "Details" }));
    expect(screen.getByText("Tracked issues")).toBeDefined();
    expect(screen.getByText("Other pull requests")).toBeDefined();
    // The tracked issue links to its side panel; the one-off PRs link out to GitHub.
    expect(screen.getByText("#525 declarative import")).toBeDefined();
    const typoLink = screen.getByRole("link", { name: /#533 fix typo/ });
    expect(typoLink.getAttribute("href")).toBe("pr-533");
    // The PR grouped under the tracked issue is rendered too, so the "3 PRs" count
    // reconciles with what is on screen (grouped PR #526 + one-off #540, #533).
    const groupedLink = screen.getByRole("link", {
      name: /#526 import part one/,
    });
    expect(groupedLink.getAttribute("href")).toBe("pr-526");
  });
});

describe("AgentSdlcView phase-transition flights", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  // Default age is older than the replay-on-load window, so these live-transition tests start
  // from a settled board (no load replay) and isolate the prev -> next flight.
  function card(partial: Partial<AgentSdlcCard>): AgentSdlcCard {
    return {
      number: 10,
      title: "Fly me",
      url: "u",
      phase: "spec",
      timeInPhaseSeconds: 5000,
      scope: null,
      escalation: null,
      rework: null,
      history: [],
      ...partial,
    };
  }

  function flightData(phase: string, timeInPhaseSeconds = 5000): AgentSdlcData {
    return {
      phases: [
        { key: "spec", sleSeconds: 100 },
        { key: "spec-approved", sleSeconds: 100 },
        { key: "shipped", sleSeconds: 0 },
      ],
      cards: [card({ phase, timeInPhaseSeconds })],
      lane: [],
      matrix: { hubClusters: [], rows: [] },
    };
  }

  function tree(data: AgentSdlcData) {
    const history = createMemoryHistory({ path: "/sdlc", search: "" });
    return (
      <IntlProvider locale="en" defaultLocale="en">
        <RouterProvider history={history}>
          <AgentSdlcView
            data={data}
            selectedBundleSeed={null}
            nowMs={1_000_000}
          />
        </RouterProvider>
      </IntlProvider>
    );
  }

  function stubReducedMotion(matches: boolean): void {
    vi.stubGlobal(
      "matchMedia",
      (query: string) =>
        ({
          matches,
          media: query,
          onchange: null,
          addEventListener: () => undefined,
          removeEventListener: () => undefined,
          addListener: () => undefined,
          removeListener: () => undefined,
          dispatchEvent: () => false,
        }) as unknown as MediaQueryList,
    );
  }

  // Verifies: SDLC-VIS-21.03
  it("flies the whole card and holds its destination when a phase advances", () => {
    const { rerender } = render(tree(flightData("spec")));
    // Before the transition the card rests in its column (a queryable anchor).
    expect(document.querySelector('[data-card-number="10"]')).not.toBeNull();
    expect(document.querySelector('[data-card-flight="10"]')).toBeNull();

    rerender(tree(flightData("spec-approved")));

    // A full-card flight is airborne, and the destination holds the resting card (no
    // double render) until it lands: the resting anchor is gone while the flight shows.
    expect(document.querySelectorAll('[data-card-flight="10"]')).toHaveLength(
      1,
    );
    expect(document.querySelector('[data-card-number="10"]')).toBeNull();
  });

  // Verifies: SDLC-VIS-20.02
  it("flies on a backward reject-loop move too", () => {
    const { rerender } = render(tree(flightData("spec-approved")));
    rerender(tree(flightData("spec")));
    expect(document.querySelectorAll('[data-card-flight="10"]')).toHaveLength(
      1,
    );
  });

  // Verifies: SDLC-VIS-21.02
  it("does not restart or cancel an in-flight flight on a continuous-aging re-render", () => {
    const { rerender } = render(tree(flightData("spec")));
    rerender(tree(flightData("spec-approved")));
    expect(document.querySelectorAll('[data-card-flight="10"]')).toHaveLength(
      1,
    );

    // A later snapshot with the SAME phase (only the aged time advanced) must not emit a
    // new flight nor cancel the one still airborne - exactly one flight remains.
    rerender(tree(flightData("spec-approved", 90)));
    expect(document.querySelectorAll('[data-card-flight="10"]')).toHaveLength(
      1,
    );
  });

  // Verifies: SDLC-VIS-20.04
  it("snaps (no flight) under prefers-reduced-motion", () => {
    stubReducedMotion(true);
    const { rerender } = render(tree(flightData("spec")));
    rerender(tree(flightData("spec-approved")));
    // No flight plays; the card snaps straight into its new column slot.
    expect(document.querySelector('[data-card-flight="10"]')).toBeNull();
    expect(document.querySelector('[data-card-number="10"]')).not.toBeNull();
  });

  // Verifies: SDLC-VIS-20.03
  it("flies the card into the release matrix on the Shipped hop", () => {
    const onBoard: AgentSdlcData = {
      ...flightData("spec-approved"),
      cards: [card({ phase: "spec-approved" })],
    };
    const shipped: AgentSdlcData = {
      ...onBoard,
      cards: [],
      matrix: {
        hubClusters: [],
        rows: [
          {
            version: "v1",
            digest: "d1",
            date: null,
            unreleased: false,
            hubClusters: [],
            issues: [
              {
                number: 10,
                title: "Fly me",
                url: "u",
                mergedAt: null,
                mergedBy: null,
                prs: [],
              },
            ],
            otherPRs: [],
          },
        ],
      },
    };
    const { rerender } = render(tree(onBoard));
    rerender(tree(shipped));
    // The departing card flies into the matrix (one flight), and is gone from the board.
    expect(document.querySelectorAll('[data-card-flight="10"]')).toHaveLength(
      1,
    );
    expect(document.querySelector('[data-card-number="10"]')).toBeNull();
  });

  // Verifies: SDLC-VIS-21.04
  it("flies each of several simultaneous phase changes on its own flight", () => {
    const before: AgentSdlcData = {
      ...flightData("spec"),
      cards: [
        card({ number: 10, title: "First", phase: "spec" }),
        card({ number: 11, title: "Second", phase: "spec" }),
      ],
    };
    const after: AgentSdlcData = {
      ...before,
      cards: [
        card({ number: 10, title: "First", phase: "spec-approved" }),
        card({ number: 11, title: "Second", phase: "spec-approved" }),
      ],
    };
    const { rerender } = render(tree(before));
    rerender(tree(after));
    expect(document.querySelectorAll('[data-card-flight="10"]')).toHaveLength(
      1,
    );
    expect(document.querySelectorAll('[data-card-flight="11"]')).toHaveLength(
      1,
    );
  });
});

describe("AgentSdlcView replay-on-load and landing", () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  const PHASES = [
    { key: "intake", sleSeconds: 100 },
    { key: "workable", sleSeconds: 100 },
    { key: "spec", sleSeconds: 100 },
    { key: "spec-approved", sleSeconds: 100 },
    { key: "shipped", sleSeconds: 0 },
  ];
  function replayCard(partial: Partial<AgentSdlcCard>): AgentSdlcCard {
    return {
      number: 10,
      title: "Fly me",
      url: "u",
      phase: "spec",
      timeInPhaseSeconds: 60,
      scope: null,
      escalation: null,
      rework: null,
      history: [],
      ...partial,
    };
  }
  function renderBoard(cards: AgentSdlcCard[]) {
    const data: AgentSdlcData = {
      phases: PHASES,
      cards,
      lane: [],
      matrix: { hubClusters: [], rows: [] },
    };
    const history = createMemoryHistory({ path: "/sdlc", search: "" });
    return render(
      <IntlProvider locale="en" defaultLocale="en">
        <RouterProvider history={history}>
          <AgentSdlcView
            data={data}
            selectedBundleSeed={null}
            nowMs={1_000_000}
          />
        </RouterProvider>
      </IntlProvider>,
    );
  }

  // Verifies: SDLC-VIS-24.01
  it("replays a recently-arrived card on first load, holding it until it lands", () => {
    renderBoard([replayCard({ phase: "spec", timeInPhaseSeconds: 60 })]);
    // On first load (no prior snapshot) a replay flight is airborne and the destination
    // card is held out of its column until the flying copy lands.
    expect(document.querySelectorAll('[data-card-flight="10"]')).toHaveLength(
      1,
    );
    expect(document.querySelector('[data-card-number="10"]')).toBeNull();
  });

  // Verifies: SDLC-VIS-24.02
  it("does not replay a card older than the replay window", () => {
    renderBoard([replayCard({ phase: "spec", timeInPhaseSeconds: 5000 })]);
    expect(document.querySelector('[data-card-flight="10"]')).toBeNull();
    expect(document.querySelector('[data-card-number="10"]')).not.toBeNull();
  });

  // Verifies: SDLC-VIS-20.05
  it("renders no dust or impact burst beside the flying card", () => {
    renderBoard([replayCard({ phase: "spec", timeInPhaseSeconds: 60 })]);
    const flight = document.querySelector('[data-card-flight="10"]');
    expect(flight).not.toBeNull();
    // The flight wrapper carries only the flying clone - no burst/dust sibling.
    expect(flight?.parentElement?.childElementCount).toBe(1);
  });

  // Verifies: SDLC-VIS-20.06
  it("releases the hold when the clone lands, then culls the spent clone", () => {
    vi.useFakeTimers();
    renderBoard([replayCard({ phase: "spec", timeInPhaseSeconds: 60 })]);
    // Airborne: clone present, real card held.
    expect(document.querySelectorAll('[data-card-flight="10"]')).toHaveLength(
      1,
    );
    expect(document.querySelector('[data-card-number="10"]')).toBeNull();
    // At landing (FLIGHT_MS) the hold releases: the real card is back while the clone fades.
    act(() => {
      vi.advanceTimersByTime(900);
    });
    expect(document.querySelector('[data-card-number="10"]')).not.toBeNull();
    // After the settle fade the spent clone is removed.
    act(() => {
      vi.advanceTimersByTime(300);
    });
    expect(document.querySelector('[data-card-flight="10"]')).toBeNull();
  });
});

describe("AgentSdlcView stacked-card popover", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  function card(number: number, timeInPhaseSeconds: number): AgentSdlcCard {
    return {
      number,
      title: `item ${String(number)}`,
      url: "u",
      phase: "spec",
      timeInPhaseSeconds,
      scope: null,
      escalation: null,
      rework: null,
      history: [],
    };
  }

  it("stays open while the live clock ages the cards and moves the stack", () => {
    vi.useFakeTimers();
    // One card sits at its true age; the next two are too close to it to fit, so
    // they collapse into a stack placed just below it. As the clock ticks, the
    // first card slides down and the stack moves with it.
    const data: AgentSdlcData = {
      ...EMPTY,
      phases: [{ key: "spec", sleSeconds: 100 }],
      // All older than the replay-on-load window so no load replay holds a card out of the
      // stack; the ages stay evenly spaced so cards 2 and 3 still collapse under card 1.
      cards: [card(1, 700), card(2, 800), card(3, 900)],
    };
    // No nowMs override: the live clock ticks every second.
    renderView({ data });

    fireEvent.click(screen.getByRole("button", { name: /\+2 more/ }));
    expect(screen.getByText("2 items at a similar age")).toBeDefined();

    // Several ticks move the stack's position; the popover must stay open.
    act(() => {
      vi.advanceTimersByTime(5_000);
    });
    expect(screen.getByText("2 items at a similar age")).toBeDefined();
    expect(screen.getByRole("button", { name: /#2 item 2/ })).toBeDefined();
  });
});

function makeAgentMetrics(
  overrides?: Partial<SdlcAgentMetrics>,
): SdlcAgentMetrics {
  return {
    name: "spec-review",
    env: "integration",
    replicasOn: 2,
    replicasTotal: 3,
    queueDepth: 5,
    itemsProcessed: 10,
    itemsFailed: 1,
    provisionSeconds: 12,
    series: {
      times: [],
      queueDepth: [],
      itemsProcessed: [],
      itemsFailed: [1],
      provisionSeconds: [],
    },
    ...overrides,
  };
}

describe("AgentSdlcView column agent integration", () => {
  const data: AgentSdlcData = {
    ...EMPTY,
    phases: [{ key: "spec", sleSeconds: 100 }],
  };

  // Verifies: SDLC-AGT-07.03
  it("renders the agent name as plain text when there is no runtime telemetry", () => {
    renderView({ data });
    expect(screen.getByText("spec-review")).toBeDefined();
    expect(screen.queryByRole("button", { name: /spec-review/ })).toBeNull();
  });

  // Verifies: SDLC-AGT-07.02, SDLC-AGT-08.01, SDLC-AGT-08.03
  it("renders the agent name as a button with a runtime strip, and opens the detail panel on click", () => {
    const agents = new Map([["spec-review", makeAgentMetrics()]]);
    renderView({ data, agents });

    const button = screen.getByRole("button", {
      name: "Open runtime details for the spec-review agent",
    });
    expect(screen.getByText("on")).toBeDefined();
    expect(screen.getByText("2/3")).toBeDefined();
    expect(screen.getByText("queue")).toBeDefined();
    expect(screen.getByText("5")).toBeDefined();
    expect(screen.getByText("fail")).toBeDefined();
    expect(screen.getByText("1")).toBeDefined();

    fireEvent.click(button);
    expect(screen.getByText("Queue")).toBeDefined();
    expect(screen.getByText("Replicas")).toBeDefined();
  });

  // Verifies: SDLC-AGT-08.02
  it("mutes queue and failure counts when they are zero", () => {
    const agents = new Map([
      [
        "spec-review",
        makeAgentMetrics({
          queueDepth: 0,
          itemsFailed: 0,
          series: {
            times: [],
            queueDepth: [],
            itemsProcessed: [],
            itemsFailed: [0],
            provisionSeconds: [],
          },
        }),
      ],
    ]);
    renderView({ data, agents });

    const subtle = "var(--pf-t--global--text--color--subtle, #9a9da0)";
    const queueLabel = screen.getByText("queue");
    expect(queueLabel.parentElement?.style.color).toBe(subtle);
    const failLabel = screen.getByText("fail");
    expect(failLabel.parentElement?.style.color).toBe(subtle);
  });

  // Regression guard: the fail count must come from the live itemsFailed scalar,
  // not series.itemsFailed's last sample, which lags behind on the 24h/32-sample
  // history grid and can diverge from the current count.
  it("uses the live itemsFailed scalar, not a stale series sample, for the fail count", () => {
    const agents = new Map([
      [
        "spec-review",
        makeAgentMetrics({
          itemsFailed: 4,
          series: {
            times: [100],
            queueDepth: [0],
            itemsProcessed: [0],
            itemsFailed: [1],
            provisionSeconds: [0],
          },
        }),
      ],
    ]);
    renderView({ data, agents });
    expect(screen.getByText("4")).toBeDefined();
    expect(screen.queryByText("1")).toBeNull();
  });

  // Verifies: SDLC-AGT-09
  it("renders capped sparkline tiles for queue, done, and failed history in the detail panel", () => {
    const agents = new Map([
      [
        "spec-review",
        makeAgentMetrics({
          queueDepth: 5,
          itemsProcessed: 10,
          itemsFailed: 1,
          series: {
            times: [100, 200],
            queueDepth: [3, 5],
            itemsProcessed: [8, 10],
            itemsFailed: [0, 1],
            provisionSeconds: [1, 2],
          },
        }),
      ],
    ]);
    renderView({ data, agents });
    fireEvent.click(
      screen.getByRole("button", {
        name: "Open runtime details for the spec-review agent",
      }),
    );
    expect(screen.getByRole("group", { name: "Queue: 5" })).toBeDefined();
    expect(screen.getByRole("group", { name: "Done: 10" })).toBeDefined();
    expect(screen.getByRole("group", { name: "Failed: 1" })).toBeDefined();
  });
});

describe("AgentSdlcView column agent activity (SDLC-AGT-17)", () => {
  const data: AgentSdlcData = {
    ...EMPTY,
    phases: [{ key: "spec", sleSeconds: 100 }],
  };
  const NOW_MS = 1_700_000_000_000;
  const nowS = NOW_MS / 1000;

  // Verifies: SDLC-AGT-17.01, SDLC-AGT-17.02
  it("renders Active with a last-seen line for a running agent with a fresh timestamp", () => {
    const agents = new Map([
      [
        "spec-review",
        makeAgentMetrics({ replicasOn: 2, lastReportedAt: nowS - 30 }),
      ],
    ]);
    renderView({ data, agents, nowMs: NOW_MS, planeStale: false });
    expect(screen.getByText("Active")).toBeDefined();
    expect(screen.getByText(/last seen .* ago/)).toBeDefined();
  });

  // Verifies: SDLC-AGT-17.01
  it("renders Not active for an idle agent (no running replicas)", () => {
    const agents = new Map([
      [
        "spec-review",
        makeAgentMetrics({ replicasOn: 0, lastReportedAt: nowS - 30 }),
      ],
    ]);
    renderView({ data, agents, nowMs: NOW_MS, planeStale: false });
    expect(screen.getByText("Not active")).toBeDefined();
  });

  // Blind state on the board column header.
  // Verifies: SDLC-AGT-17.01
  it("renders Monitoring blind for every agent when the plane is stale", () => {
    const agents = new Map([
      [
        "spec-review",
        makeAgentMetrics({ replicasOn: 2, lastReportedAt: nowS - 30 }),
      ],
    ]);
    renderView({ data, agents, nowMs: NOW_MS, planeStale: true });
    expect(screen.getByText("Monitoring blind")).toBeDefined();
    expect(screen.queryByText("Active")).toBeNull();
  });

  // Verifies: SDLC-AGT-17.03
  it("keeps the agent name a button (history reachable) even when not active", () => {
    const agents = new Map([
      [
        "spec-review",
        makeAgentMetrics({ replicasOn: 0, lastReportedAt: nowS - 30 }),
      ],
    ]);
    renderView({ data, agents, nowMs: NOW_MS, planeStale: false });
    expect(screen.getByText("Not active")).toBeDefined();
    expect(
      screen.getByRole("button", {
        name: "Open runtime details for the spec-review agent",
      }),
    ).toBeDefined();
  });
});
