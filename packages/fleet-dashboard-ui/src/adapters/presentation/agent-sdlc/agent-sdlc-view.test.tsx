import { fireEvent, render, screen } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { AgentSdlcCard, AgentSdlcData } from "../../../domain/agent-sdlc";
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
          timeInPhaseSeconds: 50,
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
    // as a single addressable navigation (GLASS-NAV-08). The bundle reads by its
    // identiname (the stable two-word alias), not the raw version.
    fireEvent.click(screen.getByRole("link", { name: identiName("d2") }));
    const route = parseRoute(history.read());
    expect(route.tab).toBe("map");
    expect(route.bundle).toBe("d2");
  });

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

  function card(partial: Partial<AgentSdlcCard>): AgentSdlcCard {
    return {
      number: 10,
      title: "Fly me",
      url: "u",
      phase: "spec",
      timeInPhaseSeconds: 50,
      scope: null,
      escalation: null,
      rework: null,
      history: [],
      ...partial,
    };
  }

  function flightData(phase: string, timeInPhaseSeconds = 50): AgentSdlcData {
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

  it("flies on a backward reject-loop move too", () => {
    const { rerender } = render(tree(flightData("spec-approved")));
    rerender(tree(flightData("spec")));
    expect(document.querySelectorAll('[data-card-flight="10"]')).toHaveLength(
      1,
    );
  });

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

  it("snaps (no flight) under prefers-reduced-motion", () => {
    stubReducedMotion(true);
    const { rerender } = render(tree(flightData("spec")));
    rerender(tree(flightData("spec-approved")));
    // No flight plays; the card snaps straight into its new column slot.
    expect(document.querySelector('[data-card-flight="10"]')).toBeNull();
    expect(document.querySelector('[data-card-number="10"]')).not.toBeNull();
  });

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
